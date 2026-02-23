/*
Copyright 2026 The Radius Authors.

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

package generate

import "time"

// --- Parsed Model (Input) ---

// BicepFile represents a single parsed Bicep file.
type BicepFile struct {
	// Path is the absolute file path of the source Bicep file.
	Path string `json:"path"`
	// Resources contains the resource declarations found in the file.
	Resources []BicepResource `json:"resources"`
	// Parameters contains the parameter declarations found in the file.
	Parameters []BicepParameter `json:"parameters"`
	// Modules contains the module declarations found in the file (main.bicep only).
	Modules []BicepModule `json:"modules"`
	// Variables contains the variable declarations found in the file.
	Variables []BicepVariable `json:"variables"`
}

// BicepResource represents a resource declaration in Bicep.
type BicepResource struct {
	// SymbolicName is the Bicep symbolic name (e.g., "apiservice").
	SymbolicName string `json:"symbolicName"`
	// Type is the resource type (e.g., "Microsoft.App/containerApps@2024-03-01").
	Type string `json:"type"`
	// Name is the resource name expression.
	Name string `json:"name"`
	// Properties is the nested property tree extracted from the resource body.
	Properties map[string]any `json:"properties"`
	// SourceFile is the file this resource was extracted from.
	SourceFile string `json:"sourceFile"`
	// StartLine is the line number where the resource declaration begins.
	StartLine int `json:"startLine"`
}

// BicepParameter represents a param declaration in Bicep.
type BicepParameter struct {
	// Name is the parameter name.
	Name string `json:"name"`
	// Type is the parameter type (e.g., "string", "int").
	Type string `json:"type"`
	// DefaultValue is the default value expression, if any.
	DefaultValue string `json:"defaultValue,omitempty"`
	// IsSecure indicates whether the parameter has the @secure() decorator.
	IsSecure bool `json:"isSecure,omitempty"`
	// Description is the @description() value, if any.
	Description string `json:"description,omitempty"`
	// SourceFile is the file this parameter was extracted from.
	SourceFile string `json:"sourceFile"`
}

// BicepModule represents a module declaration in main.bicep.
type BicepModule struct {
	// Name is the module symbolic name.
	Name string `json:"name"`
	// Source is the module source path (relative to main.bicep).
	Source string `json:"source"`
	// Parameters contains the parameter expressions passed to the module.
	Parameters map[string]string `json:"parameters,omitempty"`
	// DependsOn contains the symbolic names this module depends on.
	DependsOn []string `json:"dependsOn,omitempty"`
}

// BicepVariable represents a var declaration in Bicep.
type BicepVariable struct {
	// Name is the variable name.
	Name string `json:"name"`
	// Value is the variable value expression.
	Value string `json:"value"`
}

// --- Radius Model (Output) ---

// RadiusApplication is the top-level entity representing the entire application conversion.
type RadiusApplication struct {
	// Name is the application name derived from the Aspire project name.
	Name string `json:"name"`
	// Containers contains the service containers in the application.
	Containers []RadiusContainer `json:"containers"`
	// Dependencies contains the infrastructure dependencies (Redis, etc.).
	Dependencies []RadiusDependency `json:"dependencies"`
	// Parameters contains the Bicep parameters for the output file.
	Parameters []RadiusParameter `json:"parameters"`
	// Variables contains the Bicep variables for the output file.
	Variables []RadiusVariable `json:"variables,omitempty"`
}

// RadiusContainer represents a Radius.Compute/containers resource.
type RadiusContainer struct {
	// Name is the container resource name.
	Name string `json:"name"`
	// ImageParam is the Bicep parameter name for the image.
	ImageParam string `json:"imageParam"`
	// ImageDefault is the default image value.
	ImageDefault string `json:"imageDefault"`
	// Ports contains the port definitions.
	Ports []RadiusPort `json:"ports"`
	// EnvVars contains the environment variables.
	EnvVars map[string]string `json:"envVars,omitempty"`
	// Connections contains the connections to other resources.
	Connections []RadiusConnection `json:"connections,omitempty"`
	// IsExternal indicates whether this service has external ingress.
	IsExternal bool `json:"isExternal,omitempty"`
	// Command is the container command override.
	Command []string `json:"command,omitempty"`
}

// RadiusPort represents a port definition on a container.
type RadiusPort struct {
	// Name is the port name (e.g., "http", "tcp").
	Name string `json:"name"`
	// ContainerPort is the port number.
	ContainerPort int `json:"containerPort"`
	// Protocol is the protocol ("TCP", "UDP").
	Protocol string `json:"protocol"`
	// IsPlaceholder indicates whether this port is a placeholder (not found in source).
	IsPlaceholder bool `json:"isPlaceholder,omitempty"`
}

// RadiusConnection represents a connection from one resource to another.
type RadiusConnection struct {
	// Name is the connection name (e.g., "cache", "api").
	Name string `json:"name"`
	// TargetResourceName is the symbolic name of the target resource.
	TargetResourceName string `json:"targetResourceName"`
	// Source is the Bicep expression for the connection source (e.g., "cache.id").
	Source string `json:"source"`
}

// RadiusDependency represents a portable resource (e.g., Redis).
type RadiusDependency struct {
	// Name is the resource name.
	Name string `json:"name"`
	// Type is the Radius resource type (e.g., "Applications.Datastores/redisCaches").
	Type string `json:"type"`
	// IsRecipeBacked indicates whether the dependency is provisioned by Recipe.
	IsRecipeBacked bool `json:"isRecipeBacked"`
	// IsPlaceholder indicates whether this is a placeholder (unsupported type).
	IsPlaceholder bool `json:"isPlaceholder,omitempty"`
	// PlaceholderComment is a comment explaining the placeholder.
	PlaceholderComment string `json:"placeholderComment,omitempty"`
}

// RadiusParameter represents a param declaration in the output app.bicep.
type RadiusParameter struct {
	// Name is the parameter name.
	Name string `json:"name"`
	// Type is the Bicep type ("string", "int").
	Type string `json:"type"`
	// DefaultValue is the default value.
	DefaultValue string `json:"defaultValue,omitempty"`
	// IsSecure indicates whether to add @secure().
	IsSecure bool `json:"isSecure,omitempty"`
	// Description is the @description() text.
	Description string `json:"description,omitempty"`
}

// RadiusVariable represents a var declaration in the output app.bicep.
type RadiusVariable struct {
	// Name is the variable name.
	Name string `json:"name"`
	// Value is the variable expression.
	Value string `json:"value"`
}

// --- Mapping Report Model ---

// MappingEntry tracks one field's lineage from source to output.
type MappingEntry struct {
	// TargetResource is the name of the Radius resource in app.bicep.
	TargetResource string `json:"targetResource"`
	// TargetField is the dotted path of the field (e.g., "container.ports.http.containerPort").
	TargetField string `json:"targetField"`
	// SourceFile is the source Bicep file path.
	SourceFile string `json:"sourceFile"`
	// SourceField is the source field path (e.g., "configuration.ingress.targetPort").
	SourceField string `json:"sourceField"`
	// Value is the mapped value.
	Value string `json:"value"`
	// IsGap indicates whether this is a gap (field could not be populated).
	IsGap bool `json:"isGap,omitempty"`
	// IsAssumption indicates whether a default/assumption was used.
	IsAssumption bool `json:"isAssumption,omitempty"`
	// GapMessage is the explanation of the gap or assumption.
	GapMessage string `json:"gapMessage,omitempty"`
}

// MappingReport aggregates all mapping entries for output.
type MappingReport struct {
	// SourceDirectory is the input directory path.
	SourceDirectory string `json:"sourceDirectory"`
	// OutputFile is the output file path.
	OutputFile string `json:"outputFile"`
	// GeneratedAt is the timestamp of conversion.
	GeneratedAt time.Time `json:"generatedAt"`
	// Entries contains all mapping entries.
	Entries []MappingEntry `json:"entries"`
	// Gaps contains only gap entries (filtered from Entries).
	Gaps []MappingEntry `json:"gaps,omitempty"`
	// Assumptions contains only assumption entries (filtered from Entries).
	Assumptions []MappingEntry `json:"assumptions,omitempty"`
}
