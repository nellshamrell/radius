/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package aspire

import (
	"fmt"
	"sort"
)

// Translate is the top-level entry point. It reads the manifest, runs the full
// translation pipeline, and returns the generated Bicep text.
func Translate(opts TranslateOptions) (*TranslateResult, error) {
	// Parse the manifest.
	manifest, err := parseManifest(opts.ManifestPath)
	if err != nil {
		return nil, err
	}

	// Build config with defaults.
	config := &translationConfig{
		appName:           opts.AppName,
		environmentName:   opts.EnvironmentName,
		imageMappings:     opts.ImageMappings,
		resourceOverrides: opts.ResourceOverrides,
	}

	if config.appName == "" {
		config.appName = "app"
	}

	if config.environmentName == "" {
		config.environmentName = "default"
	}

	// Create translation context.
	ctx := newTranslationContext(manifest, config)

	// Check for circular references.
	if err := detectCircularReferences(ctx); err != nil {
		return nil, err
	}

	// Validate expression references.
	if err := validateExpressionReferences(ctx); err != nil {
		return nil, err
	}

	// Phase 1: Classify all resources.
	classifyResources(ctx)

	// Check for empty manifest.
	if isEmptyManifest(ctx) {
		return handleEmptyManifest(), nil
	}

	// Phase 2: Sanitize identifiers.
	if err := sanitizeIdentifiers(ctx); err != nil {
		return nil, err
	}

	// Phase 3: Map resources.
	if err := mapResources(ctx); err != nil {
		return nil, err
	}

	// Phase 4: Resolve expressions.
	resolveExpressions(ctx)

	// Check for accumulated errors.
	if len(ctx.errors) > 0 {
		// Return the first error for now.
		return nil, ctx.errors[0]
	}

	// Phase 5: Synthesize gateway if needed.
	gateway := synthesizeGateway(ctx)
	if gateway != nil {
		ctx.resources["gateway"] = gateway
	}

	// Phase 6: Emit Bicep.
	bicep, err := emit(ctx)
	if err != nil {
		return nil, err
	}

	// Build the result.
	result := buildResult(ctx, bicep, gateway)

	return result, nil
}

// classifyResources classifies all manifest resources and populates kindMap.
func classifyResources(ctx *translationContext) {
	for name, resource := range ctx.manifest.Resources {
		kind := classify(name, resource, ctx.config.resourceOverrides)
		ctx.kindMap[name] = kind

		if kind == KindUnsupported {
			ctx.addWarning(fmt.Sprintf("Skipping unrecognized resource type %q for resource %q", resource.Type, name))
		}
	}
}

// sanitizeIdentifiers generates Bicep identifiers for all translatable resources.
func sanitizeIdentifiers(ctx *translationContext) error {
	var names []string
	for name, kind := range ctx.kindMap {
		if kind != KindUnsupported && kind != KindValueResource && kind != KindParameter {
			names = append(names, name)
		}
	}

	// Sort for deterministic ordering.
	sort.Strings(names)

	nameMap, err := sanitizeAll(names)
	if err != nil {
		return err
	}

	ctx.nameMap = nameMap

	return nil
}

// mapResources converts each classifed manifest resource into a RadiusResource.
func mapResources(ctx *translationContext) error {
	for name, kind := range ctx.kindMap {
		resource := ctx.manifest.Resources[name]

		switch {
		case kind == KindContainer:
			bicepID := ctx.nameMap[name]
			mapped, err := mapContainer(name, resource, bicepID, ctx.config.imageMappings)
			if err != nil {
				return err
			}

			ctx.resources[name] = mapped

		case kind.IsPortableResource():
			bicepID := ctx.nameMap[name]
			mapped := mapPortableResource(name, kind, bicepID)
			ctx.resources[name] = mapped

		case kind == KindValueResource:
			// Value resources are inlined into consumers — no standalone resource.

		case kind == KindParameter:
			// Parameter resources become Bicep parameters.
			mapParameter(name, resource, ctx)

		case kind == KindUnsupported:
			// Already warned during classification.
		}
	}

	return nil
}

// mapParameter converts a parameter.v0 resource to a BicepParameter.
func mapParameter(name string, resource ManifestResource, ctx *translationContext) {
	sanitizedName := sanitize(name)

	param := BicepParameter{
		Name:        sanitizedName,
		Type:        "string",
		Description: fmt.Sprintf("Parameter: %s", name),
	}

	if resource.Value != "" {
		param.DefaultValue = resource.Value
	}

	// Check inputs for secret flag.
	if resource.Inputs != nil {
		for _, input := range resource.Inputs {
			if input.Secret {
				param.Secure = true
				break
			}
		}
	}

	ctx.parameters = append(ctx.parameters, param)
}

// buildResult constructs the TranslateResult from the translation context.
func buildResult(ctx *translationContext, bicep string, gateway *RadiusResource) *TranslateResult {
	var resources []TranslatedResource

	// Collect resource summaries in sorted order.
	var names []string
	for name := range ctx.resources {
		if name == "gateway" {
			continue
		}

		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		res := ctx.resources[name]
		resources = append(resources, TranslatedResource{
			OriginalName:    name,
			BicepIdentifier: res.BicepIdentifier,
			Kind:            res.Kind,
		})
	}

	// Add gateway if present.
	if gateway != nil {
		resources = append(resources, TranslatedResource{
			OriginalName:    "gateway",
			BicepIdentifier: "gateway",
			Kind:            KindGateway,
			Synthesized:     true,
		})
	}

	// Add application as synthesized.
	resources = append(resources, TranslatedResource{
		OriginalName:    ctx.config.appName,
		BicepIdentifier: "app",
		Kind:            KindApplication,
		Synthesized:     true,
	})

	return &TranslateResult{
		Bicep:     bicep,
		Resources: resources,
		Warnings:  ctx.warnings,
	}
}

// isEmptyManifest checks if the manifest has no translatable resources.
func isEmptyManifest(ctx *translationContext) bool {
	for _, kind := range ctx.kindMap {
		if kind != KindUnsupported {
			return false
		}
	}

	return true
}

// handleEmptyManifest returns an empty translation result with a message.
func handleEmptyManifest() *TranslateResult {
	return &TranslateResult{
		Bicep:    "",
		Warnings: []string{"No translatable resources found in manifest"},
	}
}

// validateExpressionReferences checks that all expression references point to existing resources.
func validateExpressionReferences(ctx *translationContext) error {
	for name, resource := range ctx.manifest.Resources {
		allValues := collectAllExpressionValues(resource)

		for _, value := range allValues {
			cv := parseExpressions(value)
			for _, part := range cv.parts {
				if part.expression == nil {
					continue
				}

				targetName := part.expression.ResourceName
				if _, exists := ctx.manifest.Resources[targetName]; !exists {
					return &unknownResourceError{
						sourceResource: name,
						targetResource: targetName,
					}
				}
			}
		}
	}

	return nil
}

// collectAllExpressionValues gathers all string values that may contain expressions.
func collectAllExpressionValues(resource ManifestResource) []string {
	var values []string

	for _, v := range resource.Env {
		values = append(values, v)
	}

	if resource.ConnectionString != "" {
		values = append(values, resource.ConnectionString)
	}

	return values
}

// addEmptyManifestCheck extends Translate to handle empty manifests gracefully.
func init() {
	// Validation is integrated directly into Translate().
	// This placeholder documents the design decision.
}

// The imports for "sort" and "strings" are already declared.
// These functions extend the translation pipeline with additional validation.

// validateTranslation performs final validation checks on the translation context.
func validateTranslation(ctx *translationContext) error {
	// Validate all expression references.
	if err := validateExpressionReferences(ctx); err != nil {
		return err
	}

	return nil
}
