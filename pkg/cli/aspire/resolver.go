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
	"strings"
)

// AspireExpression represents a parsed reference extracted from an expression.
type AspireExpression struct {
	// ResourceName is the referenced resource name.
	ResourceName string

	// PropertyPath contains path segments after the resource name.
	PropertyPath []string

	// RawText is the original {...} text for error reporting.
	RawText string
}

// valuePart represents a single segment of a composite value — either literal text or an expression.
type valuePart struct {
	literal    string
	expression *AspireExpression
}

// compositeValue represents a string value containing zero or more expression references mixed with literals.
type compositeValue struct {
	parts []valuePart
}

// parseExpressions scans a string for {...} patterns and returns a compositeValue.
// It handles composite values with multiple references mixed with literal text.
func parseExpressions(input string) *compositeValue {
	cv := &compositeValue{}
	remaining := input

	for len(remaining) > 0 {
		// Find the next opening brace.
		openIdx := strings.Index(remaining, "{")
		if openIdx == -1 {
			// No more expressions — rest is literal.
			cv.parts = append(cv.parts, valuePart{literal: remaining})
			break
		}

		// Add literal text before the expression.
		if openIdx > 0 {
			cv.parts = append(cv.parts, valuePart{literal: remaining[:openIdx]})
		}

		// Find the closing brace.
		closeIdx := strings.Index(remaining[openIdx:], "}")
		if closeIdx == -1 {
			// No closing brace — treat rest as literal.
			cv.parts = append(cv.parts, valuePart{literal: remaining[openIdx:]})
			break
		}

		// Extract the expression content (without braces).
		closeIdx += openIdx
		rawText := remaining[openIdx : closeIdx+1]
		exprContent := remaining[openIdx+1 : closeIdx]

		// Parse the expression into resource name and property path.
		expr := parseExpressionContent(exprContent, rawText)
		cv.parts = append(cv.parts, valuePart{expression: expr})

		remaining = remaining[closeIdx+1:]
	}

	return cv
}

// parseExpressionContent parses the content inside {...} into an AspireExpression.
func parseExpressionContent(content, rawText string) *AspireExpression {
	parts := strings.Split(content, ".")
	if len(parts) == 0 {
		return &AspireExpression{RawText: rawText}
	}

	return &AspireExpression{
		ResourceName: parts[0],
		PropertyPath: parts[1:],
		RawText:      rawText,
	}
}

// hasExpressions returns true if the composite value contains any expression references.
func (cv *compositeValue) hasExpressions() bool {
	for _, part := range cv.parts {
		if part.expression != nil {
			return true
		}
	}

	return false
}

// resolveExpressions resolves all expression references in the translation context,
// building connections maps and replacing env var values with resolved text.
func resolveExpressions(ctx *translationContext) {
	for name, resource := range ctx.resources {
		if resource.Container == nil {
			continue
		}

		resolvedEnv := make(map[string]EnvVarSpec, len(resource.Container.Env))
		connections := make(map[string]ConnectionSpec)

		for envKey, envSpec := range resource.Container.Env {
			cv := parseExpressions(envSpec.Value)
			if !cv.hasExpressions() {
				resolvedEnv[envKey] = envSpec
				continue
			}

			resolved, conn, err := resolveCompositeValue(cv, name, ctx)
			if err != nil {
				ctx.addError(err)
				resolvedEnv[envKey] = envSpec
				continue
			}

			// Merge connections.
			for connName, connSpec := range conn {
				connections[connName] = connSpec
			}

			resolvedEnv[envKey] = resolved
		}

		resource.Container.Env = resolvedEnv

		// Merge resolved connections with any existing connections.
		if resource.Connections == nil {
			resource.Connections = make(map[string]ConnectionSpec)
		}

		for k, v := range connections {
			resource.Connections[k] = v
		}
	}
}

// resolveCompositeValue resolves a composite value into a resolved env var and connections.
func resolveCompositeValue(cv *compositeValue, sourceResource string, ctx *translationContext) (EnvVarSpec, map[string]ConnectionSpec, error) {
	connections := make(map[string]ConnectionSpec)
	var resultParts []string
	hasBicepInterpolation := false

	for _, part := range cv.parts {
		if part.expression == nil {
			resultParts = append(resultParts, part.literal)
			continue
		}

		expr := part.expression
		targetName := expr.ResourceName

		// Validate the referenced resource exists.
		targetKind, exists := ctx.kindMap[targetName]
		if !exists {
			return EnvVarSpec{}, nil, &unknownResourceError{
				sourceResource: sourceResource,
				targetResource: targetName,
			}
		}

		resolved, connSpec, err := resolveSingleExpression(expr, targetName, targetKind, ctx)
		if err != nil {
			return EnvVarSpec{}, nil, err
		}

		resultParts = append(resultParts, resolved)

		if connSpec != nil {
			connections[targetName] = *connSpec
			if connSpec.IsBicepReference {
				hasBicepInterpolation = true
			}
		}
	}

	value := strings.Join(resultParts, "")
	return EnvVarSpec{Value: value, IsBicepInterpolation: hasBicepInterpolation}, connections, nil
}

// resolveSingleExpression resolves a single expression reference to a value string and optional connection.
func resolveSingleExpression(expr *AspireExpression, targetName string, targetKind ResourceKind, ctx *translationContext) (string, *ConnectionSpec, error) {
	path := expr.PropertyPath

	// Handle parameter references — resolve to Bicep parameter interpolation.
	if targetKind == KindParameter {
		paramName := sanitize(targetName)
		return "${" + paramName + "}", nil, nil
	}

	// Handle connectionString references.
	if len(path) == 1 && path[0] == "connectionString" {
		return resolveConnectionString(targetName, targetKind, ctx)
	}

	// Handle bindings references: bindings.<name>.<property>
	if len(path) >= 3 && path[0] == "bindings" {
		return resolveBindingReference(expr, targetName, targetKind, ctx)
	}

	// Handle value resource references.
	if targetKind == KindValueResource {
		return resolveValueReference(targetName, ctx)
	}

	return "", nil, &unsupportedExpressionError{
		resourceName: targetName,
		expression:   expr.RawText,
	}
}

// resolveConnectionString resolves a {resource.connectionString} expression.
func resolveConnectionString(targetName string, targetKind ResourceKind, ctx *translationContext) (string, *ConnectionSpec, error) {
	if targetKind.IsPortableResource() {
		// Portable resource — reference by .id.
		targetID := ctx.nameMap[targetName]
		conn := &ConnectionSpec{
			Source:           targetID + ".id",
			IsBicepReference: true,
		}

		return "${" + targetID + ".id}", conn, nil
	}

	if targetKind == KindValueResource {
		return resolveValueReference(targetName, ctx)
	}

	// Container with connectionString — resolve the target's connectionString expression recursively.
	targetResource, ok := ctx.manifest.Resources[targetName]
	if !ok {
		return "", nil, &unknownResourceError{sourceResource: targetName, targetResource: targetName}
	}

	if targetResource.ConnectionString != "" {
		cv := parseExpressions(targetResource.ConnectionString)
		if cv.hasExpressions() {
			// Resolve the target's connectionString in the target's context.
			resolved, connections, err := resolveCompositeValue(cv, targetName, ctx)
			if err != nil {
				return "", nil, err
			}

			// Return the first connection found (if any).
			for _, conn := range connections {
				return resolved.Value, &conn, nil
			}

			return resolved.Value, nil, nil
		}

		return targetResource.ConnectionString, nil, nil
	}

	// Fall back to URL-style connection.
	return resolveContainerURL(targetName, "", ctx)
}

// resolveBindingReference resolves a {resource.bindings.<name>.<property>} expression.
func resolveBindingReference(expr *AspireExpression, targetName string, targetKind ResourceKind, ctx *translationContext) (string, *ConnectionSpec, error) {
	path := expr.PropertyPath
	bindingName := path[1]
	property := ""
	if len(path) >= 3 {
		property = path[2]
	}

	// For portable resources, reference by ID.
	if targetKind.IsPortableResource() {
		targetID := ctx.nameMap[targetName]
		conn := &ConnectionSpec{
			Source:           targetID + ".id",
			IsBicepReference: true,
		}

		return "${" + targetID + ".id}", conn, nil
	}

	// For containers, resolve to URL based on binding properties.
	targetResource, ok := ctx.manifest.Resources[targetName]
	if !ok {
		return "", nil, &unknownResourceError{sourceResource: targetName, targetResource: targetName}
	}

	binding, ok := targetResource.Bindings[bindingName]
	if !ok {
		return "", nil, fmt.Errorf("expression %s references unknown binding %q on resource %q", expr.RawText, bindingName, targetName)
	}

	switch property {
	case "url":
		url := buildBindingURL(targetName, binding)
		conn := &ConnectionSpec{
			Source:           url,
			IsBicepReference: false,
		}

		return url, conn, nil
	case "host":
		return targetName, nil, nil
	case "port":
		port := binding.TargetPort
		if port == 0 {
			port = binding.Port
		}

		return fmt.Sprintf("%d", port), nil, nil
	default:
		return "", nil, &unsupportedExpressionError{
			resourceName: targetName,
			expression:   expr.RawText,
		}
	}
}

// resolveContainerURL resolves a container reference to a URL string.
func resolveContainerURL(targetName, bindingName string, ctx *translationContext) (string, *ConnectionSpec, error) {
	targetResource, ok := ctx.manifest.Resources[targetName]
	if !ok {
		return "", nil, &unknownResourceError{sourceResource: targetName, targetResource: targetName}
	}

	// Find the best binding.
	var binding ManifestBinding
	if bindingName != "" {
		b, ok := targetResource.Bindings[bindingName]
		if !ok {
			return "", nil, fmt.Errorf("unknown binding %q on resource %q", bindingName, targetName)
		}

		binding = b
	} else {
		// Use the first binding.
		for _, b := range targetResource.Bindings {
			binding = b
			break
		}
	}

	url := buildBindingURL(targetName, binding)
	conn := &ConnectionSpec{
		Source:           url,
		IsBicepReference: false,
	}

	return url, conn, nil
}

// buildBindingURL constructs a URL from a binding.
func buildBindingURL(resourceName string, binding ManifestBinding) string {
	scheme := binding.Scheme
	if scheme == "" {
		scheme = "http"
	}

	port := binding.TargetPort
	if port == 0 {
		port = binding.Port
	}

	return fmt.Sprintf("%s://%s:%d", scheme, resourceName, port)
}

// resolveValueReference resolves a reference to a value.v0 resource.
func resolveValueReference(targetName string, ctx *translationContext) (string, *ConnectionSpec, error) {
	targetResource, ok := ctx.manifest.Resources[targetName]
	if !ok {
		return "", nil, &unknownResourceError{sourceResource: targetName, targetResource: targetName}
	}

	// Value resources expose their connectionString as the resolved value.
	if targetResource.ConnectionString != "" {
		cv := parseExpressions(targetResource.ConnectionString)
		if cv.hasExpressions() {
			resolved, connections, err := resolveCompositeValue(cv, targetName, ctx)
			if err != nil {
				return "", nil, err
			}

			for _, conn := range connections {
				return resolved.Value, &conn, nil
			}

			return resolved.Value, nil, nil
		}

		return targetResource.ConnectionString, nil, nil
	}

	return targetResource.Value, nil, nil
}

// detectCircularReferences checks for circular dependencies in the resource graph.
// Only connectionString chains and value resource references are considered for cycles,
// because binding URL/host/port references resolve to static values without recursion.
func detectCircularReferences(ctx *translationContext) error {
	// Build a dependency graph from connectionString expressions only.
	// Binding references (bindings.xxx.url/host/port) resolve to static values
	// and don't create actual data dependency cycles.
	deps := make(map[string]map[string]bool)

	for name, resource := range ctx.manifest.Resources {
		deps[name] = make(map[string]bool)

		// Only track connectionString-based dependencies for cycle detection.
		if resource.ConnectionString != "" {
			cv := parseExpressions(resource.ConnectionString)
			for _, part := range cv.parts {
				if part.expression != nil {
					deps[name][part.expression.ResourceName] = true
				}
			}
		}

		// Track env vars that reference connectionStrings (not bindings).
		for _, value := range resource.Env {
			cv := parseExpressions(value)
			for _, part := range cv.parts {
				if part.expression == nil {
					continue
				}

				// Only count connectionString references as potential cycles.
				if len(part.expression.PropertyPath) == 1 && part.expression.PropertyPath[0] == "connectionString" {
					deps[name][part.expression.ResourceName] = true
				}
			}
		}
	}

	// DFS cycle detection.
	visited := make(map[string]int) // 0=unvisited, 1=in-progress, 2=done
	var chain []string

	var visit func(name string) error
	visit = func(name string) error {
		if visited[name] == 2 {
			return nil
		}

		if visited[name] == 1 {
			// Found a cycle — build the chain.
			cycle := append(chain, name)
			return &circularReferenceError{chain: cycle}
		}

		visited[name] = 1
		chain = append(chain, name)

		for dep := range deps[name] {
			if err := visit(dep); err != nil {
				return err
			}
		}

		chain = chain[:len(chain)-1]
		visited[name] = 2

		return nil
	}

	for name := range deps {
		if err := visit(name); err != nil {
			return err
		}
	}

	return nil
}
