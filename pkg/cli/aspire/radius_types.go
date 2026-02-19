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

// RadiusResource represents a translated Radius resource ready for Bicep emission.
type RadiusResource struct {
	// BicepIdentifier is the sanitized Bicep identifier (e.g., api_service).
	BicepIdentifier string

	// RuntimeName is the original Aspire resource name used as the Radius name property.
	RuntimeName string

	// RadiusType is the fully qualified Radius resource type.
	RadiusType string

	// APIVersion is the API version (hardcoded 2023-10-01-preview).
	APIVersion string

	// Kind is the resource kind discriminator.
	Kind ResourceKind

	// Container holds container details (for Container kind).
	Container *ContainerSpec

	// PortableResource holds portable resource details (for PortableResource kinds).
	PortableResource *PortableResourceSpec

	// Gateway holds gateway details (for Gateway kind).
	Gateway *GatewaySpec

	// Application holds application details (for Application kind).
	Application *ApplicationSpec

	// Connections maps dependency names to connection specs.
	Connections map[string]ConnectionSpec
}

// ContainerSpec holds container resource properties.
type ContainerSpec struct {
	// Image is the container image reference.
	Image string

	// Command is the entrypoint command (from Aspire entrypoint).
	Command []string

	// Args is the container arguments.
	Args []string

	// Env maps environment variable names to their specs.
	Env map[string]EnvVarSpec

	// Ports maps port names to their specs.
	Ports map[string]PortSpec

	// Volumes maps volume names to their specs.
	Volumes map[string]VolumeSpec
}

// PortSpec describes a container port mapping.
type PortSpec struct {
	// ContainerPort is the container-side port number.
	ContainerPort int

	// Protocol is the transport protocol (TCP/UDP).
	Protocol string

	// Scheme is the protocol scheme (http/https/tcp).
	Scheme string
}

// VolumeSpec describes a volume mount.
type VolumeSpec struct {
	// Kind is the volume kind (ephemeral or persistent).
	Kind string

	// MountPath is the path where the volume is mounted in the container.
	MountPath string

	// ReadOnly indicates whether the volume is mounted read-only.
	ReadOnly bool
}

// EnvVarSpec describes a resolved environment variable.
type EnvVarSpec struct {
	// Value is the resolved literal value (may contain Bicep interpolation syntax).
	Value string

	// IsBicepInterpolation indicates whether Value contains Bicep interpolation.
	IsBicepInterpolation bool
}

// ConnectionSpec describes a dependency connection to another resource.
type ConnectionSpec struct {
	// Source is the Radius resource ID reference or URL.
	Source string

	// IsBicepReference indicates whether Source is a Bicep expression vs a literal string.
	IsBicepReference bool
}

// PortableResourceSpec holds portable resource properties.
type PortableResourceSpec struct {
	// RecipeName is the recipe name (defaults to "default").
	RecipeName string
}

// GatewaySpec holds gateway resource properties.
type GatewaySpec struct {
	// Routes is the list of gateway routes.
	Routes []GatewayRouteSpec
}

// GatewayRouteSpec describes a single gateway route.
type GatewayRouteSpec struct {
	// Path is the route path (e.g., /).
	Path string

	// Destination is the destination URL.
	Destination string
}

// ApplicationSpec holds application resource properties.
type ApplicationSpec struct {
	// EnvironmentRef is the reference to the Radius environment.
	EnvironmentRef string
}

// BicepParameter represents a Bicep parameter declaration.
type BicepParameter struct {
	// Name is the parameter name.
	Name string

	// Type is the parameter type (e.g., "string").
	Type string

	// DefaultValue is the default value, if any.
	DefaultValue string

	// Secure indicates whether the parameter should have the @secure() decorator.
	Secure bool

	// Description is the parameter description.
	Description string
}
