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

// translationContext holds the complete state during translation.
type translationContext struct {
	// manifest is the parsed input manifest.
	manifest *AspireManifest

	// config is the user-provided configuration.
	config *translationConfig

	// resources holds translated Radius resources keyed by original Aspire name.
	resources map[string]*RadiusResource

	// parameters holds Bicep parameter declarations from parameter.v0 resources.
	parameters []BicepParameter

	// warnings accumulates non-fatal warnings.
	warnings []string

	// errors accumulates fatal errors during translation.
	errors []error

	// nameMap maps original Aspire resource names to sanitized Bicep identifiers.
	nameMap map[string]string

	// kindMap maps original Aspire resource names to their classified ResourceKind.
	kindMap map[string]ResourceKind
}

// translationConfig holds user-provided configuration that controls translation behavior.
type translationConfig struct {
	// appName is the application name override. Defaults to "app".
	appName string

	// environmentName is the environment name. Defaults to "default".
	environmentName string

	// imageMappings maps project resource names to container image references.
	imageMappings map[string]string

	// resourceOverrides maps resource names to explicit Radius resource types.
	resourceOverrides map[string]ResourceKind

	// outputDir is the directory to write app.bicep. Defaults to current directory.
	outputDir string
}

// newTranslationContext creates a new translationContext with the given manifest and config.
func newTranslationContext(manifest *AspireManifest, config *translationConfig) *translationContext {
	return &translationContext{
		manifest:  manifest,
		config:    config,
		resources: make(map[string]*RadiusResource),
		nameMap:   make(map[string]string),
		kindMap:   make(map[string]ResourceKind),
	}
}

// addWarning appends a non-fatal warning message.
func (ctx *translationContext) addWarning(msg string) {
	ctx.warnings = append(ctx.warnings, msg)
}

// addError appends a fatal error.
func (ctx *translationContext) addError(err error) {
	ctx.errors = append(ctx.errors, err)
}
