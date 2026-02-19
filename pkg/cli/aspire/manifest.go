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
	"encoding/json"
	"fmt"
	"os"
)

// parseManifest reads and validates the Aspire manifest JSON file at the given path.
func parseManifest(path string) (*AspireManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("manifest file not found: %s", path)
		}

		return nil, fmt.Errorf("failed to read manifest file: %w", err)
	}

	var manifest AspireManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	if manifest.Resources == nil {
		return nil, fmt.Errorf("failed to parse manifest: missing required field \"resources\"")
	}

	// Validate that all resources have a type field.
	for name, resource := range manifest.Resources {
		if resource.Type == "" {
			return nil, fmt.Errorf("failed to parse manifest: resource %q missing required field \"type\"", name)
		}
	}

	return &manifest, nil
}

// AspireManifest represents the top-level Aspire manifest structure.
type AspireManifest struct {
	Schema    string                      `json:"$schema"`
	Resources map[string]ManifestResource `json:"resources"`
}

// ManifestResource represents a single resource in the Aspire manifest.
type ManifestResource struct {
	Type             string                        `json:"type"`
	Image            string                        `json:"image,omitempty"`
	Entrypoint       string                        `json:"entrypoint,omitempty"`
	Path             string                        `json:"path,omitempty"`
	ConnectionString string                        `json:"connectionString,omitempty"`
	Env              map[string]string             `json:"env,omitempty"`
	Bindings         map[string]ManifestBinding    `json:"bindings,omitempty"`
	Args             []string                      `json:"args,omitempty"`
	Volumes          []ManifestVolumeMount         `json:"volumes,omitempty"`
	BindMounts       []ManifestBindMount           `json:"bindMounts,omitempty"`
	Value            string                        `json:"value,omitempty"`
	Inputs           map[string]ManifestParamInput `json:"inputs,omitempty"`
}

// ManifestBinding represents a network binding on an Aspire resource.
type ManifestBinding struct {
	Scheme     string `json:"scheme,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
	Transport  string `json:"transport,omitempty"`
	Port       int    `json:"port,omitempty"`
	TargetPort int    `json:"targetPort,omitempty"`
	External   bool   `json:"external,omitempty"`
}

// ManifestVolumeMount represents a named volume mount.
type ManifestVolumeMount struct {
	Name     string `json:"name"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"readOnly,omitempty"`
}

// ManifestBindMount represents a host bind mount.
type ManifestBindMount struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"readOnly,omitempty"`
}

// ManifestParamInput defines input configuration for parameter resources.
type ManifestParamInput struct {
	Type    string                `json:"type,omitempty"`
	Secret  bool                  `json:"secret,omitempty"`
	Default *ManifestParamDefault `json:"default,omitempty"`
}

// ManifestParamDefault defines the default value for a parameter input.
type ManifestParamDefault struct {
	Generate *ManifestParamGenerate `json:"generate,omitempty"`
	Value    string                 `json:"value,omitempty"`
}

// ManifestParamGenerate defines auto-generation config for parameter defaults.
type ManifestParamGenerate struct {
	MinLength int `json:"minLength,omitempty"`
}
