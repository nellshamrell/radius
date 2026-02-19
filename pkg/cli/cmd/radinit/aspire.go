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

package radinit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/radius-project/radius/pkg/cli/aspire"
)

// runAspireTranslation runs the manifest translation pipeline when --from-aspire-manifest is set.
// It skips the interactive init prompts and performs only the manifest translation.
func (r *Runner) runAspireTranslation(ctx context.Context) error {
	opts := aspire.TranslateOptions{
		ManifestPath:      r.AspireManifestPath,
		AppName:           r.AspireAppName,
		EnvironmentName:   r.AspireEnvironment,
		ImageMappings:     r.aspireImageMappings(),
		ResourceOverrides: r.aspireResourceOverrides(),
	}

	result, err := aspire.Translate(opts)
	if err != nil {
		return err
	}

	// Print warnings to stderr.
	for _, warning := range result.Warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", warning)
	}

	if result.Bicep == "" {
		r.Output.LogInfo("No translatable resources found in manifest.")
		return nil
	}

	// Write app.bicep to the output directory.
	outputPath := filepath.Join(r.AspireOutputDir, "app.bicep")
	if err := os.MkdirAll(r.AspireOutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	if err := os.WriteFile(outputPath, []byte(result.Bicep), 0644); err != nil {
		return fmt.Errorf("failed to write app.bicep: %w", err)
	}

	// Print summary to stdout.
	r.Output.LogInfo("Translated %d resources from Aspire manifest:", len(result.Resources))
	for _, res := range result.Resources {
		label := string(res.Kind)
		if res.Synthesized {
			label += " (synthesized)"
		}

		if res.Kind.IsPortableResource() {
			label += " (recipe)"
		}

		r.Output.LogInfo("  - %s → %s", res.OriginalName, label)
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("Generated: %s", outputPath)
	r.Output.LogInfo("")
	r.Output.LogInfo("Deploy with: rad deploy %s -p environment=<your-env-id> -p application=<your-app-id>", outputPath)

	return nil
}

// aspireImageMappings parses --image-mapping flags into a map.
func (r *Runner) aspireImageMappings() map[string]string {
	if len(r.AspireImageMappings) == 0 {
		return nil
	}

	result := make(map[string]string, len(r.AspireImageMappings))
	for _, mapping := range r.AspireImageMappings {
		parts := strings.SplitN(mapping, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}

	return result
}

// aspireResourceOverrides parses --resource-override flags into a map.
func (r *Runner) aspireResourceOverrides() map[string]aspire.ResourceKind {
	if len(r.AspireResourceOverrides) == 0 {
		return nil
	}

	result := make(map[string]aspire.ResourceKind, len(r.AspireResourceOverrides))
	for _, override := range r.AspireResourceOverrides {
		parts := strings.SplitN(override, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = aspire.ResourceKind(parts[1])
		}
	}

	return result
}

// isAspireMode returns true if the user specified --from-aspire-manifest.
func (r *Runner) isAspireMode() bool {
	return r.AspireManifestPath != ""
}
