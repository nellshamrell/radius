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

package convert

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AspireManifest is the top-level parsed representation of an Aspire manifest JSON file.
type AspireManifest struct {
	// Schema is the $schema URL from the manifest (e.g., aspire-8.0.json).
	Schema string `json:"$schema"`

	// Resources is a map of resource name to resource definition.
	Resources map[string]AspireResource `json:"resources"`
}

// AspireResource is a single resource entry within the Aspire manifest.
type AspireResource struct {
	// Name is the resource's logical name (populated from map key, not from JSON).
	Name string `json:"-"`

	// Type is the resource type identifier (e.g., container.v0, parameter.v0, redis.server.v0).
	Type string `json:"type"`

	// Image is the container image reference (for container types).
	Image string `json:"image,omitempty"`

	// Entrypoint is the container entrypoint override.
	Entrypoint string `json:"entrypoint,omitempty"`

	// Args is a list of container command arguments.
	Args []string `json:"args,omitempty"`

	// Env is a map of environment variable names to values.
	// Values may contain expression references like {cache.bindings.tcp.host}.
	Env map[string]string `json:"env,omitempty"`

	// Bindings is a map of named network bindings (ports/endpoints).
	Bindings map[string]AspireBinding `json:"bindings,omitempty"`

	// ConnectionString is a connection string template that may contain expression references.
	ConnectionString string `json:"connectionString,omitempty"`

	// Build is the build configuration for container.v1 resources with Dockerfiles.
	Build *AspireBuild `json:"build,omitempty"`

	// Value is the parameter value for parameter.v0 resources.
	Value string `json:"value,omitempty"`

	// Inputs is a map of parameter inputs for parameter.v0 resources.
	Inputs map[string]AspireInput `json:"inputs,omitempty"`

	// Error is an error message from the Aspire manifest publisher.
	// When non-empty, the resource could not be generated and has no Type field.
	// Such resources should be skipped during conversion.
	Error string `json:"error,omitempty"`

	// Filter is the filter type for annotated.string resources (e.g., "uri").
	// When present with filter: "uri", the resource's value should be wrapped
	// in a uriComponent() Bicep function call.
	Filter string `json:"filter,omitempty"`
}

// AspireBinding is a network binding/endpoint on a container resource.
type AspireBinding struct {
	// Name is the binding name (populated from map key, not from JSON).
	Name string `json:"-"`

	// Scheme is the protocol scheme (e.g., tcp, http, https).
	Scheme string `json:"scheme,omitempty"`

	// Protocol is the network protocol (e.g., tcp).
	Protocol string `json:"protocol,omitempty"`

	// Transport is the transport protocol (e.g., tcp, http).
	Transport string `json:"transport,omitempty"`

	// TargetPort is the container port number.
	TargetPort int `json:"targetPort,omitempty"`

	// External indicates whether this binding is externally accessible.
	External bool `json:"external,omitempty"`
}

// AspireBuild is the build configuration for container.v1 resources.
type AspireBuild struct {
	// Context is the build context directory path.
	Context string `json:"context,omitempty"`

	// Dockerfile is the Dockerfile path relative to context.
	Dockerfile string `json:"dockerfile,omitempty"`

	// BuildOnly indicates the container is a build artifact only (no runtime).
	BuildOnly bool `json:"buildOnly,omitempty"`
}

// AspireInput is a parameter input definition.
type AspireInput struct {
	// Type is the input type (e.g., string).
	Type string `json:"type,omitempty"`

	// Secret indicates whether this input is a secret value.
	Secret bool `json:"secret,omitempty"`

	// Description is an optional human-readable description of the input.
	Description string `json:"description,omitempty"`

	// Default is the default value configuration.
	Default *AspireInputDefault `json:"default,omitempty"`
}

// AspireInputDefault is the default value generation configuration for parameter inputs.
type AspireInputDefault struct {
	// Generate is the auto-generation configuration.
	Generate *AspireGenerate `json:"generate,omitempty"`
}

// AspireGenerate is the auto-generation configuration for default values.
type AspireGenerate struct {
	// MinLength is the minimum length for generated value.
	MinLength int `json:"minLength,omitempty"`

	// Special indicates whether to include special characters.
	Special bool `json:"special,omitempty"`
}

// Parse deserializes the given JSON data into an AspireManifest, populates resource
// Name fields from map keys and binding Name fields from their map keys, and validates
// required fields.
func Parse(data []byte) (*AspireManifest, error) {
	var manifest AspireManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	if manifest.Resources == nil || len(manifest.Resources) == 0 {
		return nil, fmt.Errorf("manifest contains no resources")
	}

	// Populate Name fields from map keys and validate required fields.
	for name, resource := range manifest.Resources {
		resource.Name = name

		// Resources with an error field (no type) are manifest-publisher errors;
		// skip type validation for these — they will be handled during mapping.
		if resource.Error == "" && strings.TrimSpace(resource.Type) == "" {
			return nil, fmt.Errorf("resource %q is missing required field \"type\"", name)
		}

		// Populate binding Name fields from map keys.
		for bindingName, binding := range resource.Bindings {
			binding.Name = bindingName
			resource.Bindings[bindingName] = binding
		}

		manifest.Resources[name] = resource
	}

	return &manifest, nil
}
