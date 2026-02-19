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
	"strings"
	"testing"
)

func TestEmit_MinimalApplication(t *testing.T) {
	t.Parallel()

	ctx := &translationContext{
		config: &translationConfig{
			appName:         "testapp",
			environmentName: "default",
		},
		resources: map[string]*RadiusResource{
			"api": {
				BicepIdentifier: "api",
				RuntimeName:     "api",
				RadiusType:      "Applications.Core/containers",
				Kind:            KindContainer,
				Container: &ContainerSpec{
					Image: "myapp/api:latest",
					Ports: map[string]PortSpec{
						"http": {ContainerPort: 8080},
					},
					Env: map[string]EnvVarSpec{
						"PORT": {Value: "8080"},
					},
				},
			},
		},
	}

	result, err := emit(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify basic structure.
	if !strings.Contains(result, "extension radius") {
		t.Error("expected 'extension radius' in output")
	}

	if !strings.Contains(result, "param environment string = 'default'") {
		t.Error("expected environment parameter in output")
	}

	if !strings.Contains(result, "param application string = 'testapp'") {
		t.Error("expected application parameter in output")
	}

	if !strings.Contains(result, "resource app 'Applications.Core/applications@2023-10-01-preview'") {
		t.Error("expected application resource in output")
	}

	if !strings.Contains(result, "resource api 'Applications.Core/containers@2023-10-01-preview'") {
		t.Error("expected container resource in output")
	}

	if !strings.Contains(result, "image: 'myapp/api:latest'") {
		t.Error("expected image in output")
	}

	// Verify ordering: extension → params → app → containers.
	extIdx := strings.Index(result, "extension radius")
	paramIdx := strings.Index(result, "param environment")
	appIdx := strings.Index(result, "resource app")
	containerIdx := strings.Index(result, "resource api")

	if extIdx >= paramIdx || paramIdx >= appIdx || appIdx >= containerIdx {
		t.Error("incorrect ordering in generated Bicep")
	}
}

func TestEmit_WithConnections(t *testing.T) {
	t.Parallel()

	ctx := &translationContext{
		config: &translationConfig{
			appName:         "app",
			environmentName: "default",
		},
		resources: map[string]*RadiusResource{
			"api": {
				BicepIdentifier: "api",
				RuntimeName:     "api",
				RadiusType:      "Applications.Core/containers",
				Kind:            KindContainer,
				Container: &ContainerSpec{
					Image: "myapp/api:latest",
					Ports: map[string]PortSpec{
						"http": {ContainerPort: 8080},
					},
					Env: map[string]EnvVarSpec{},
				},
				Connections: map[string]ConnectionSpec{
					"cache": {Source: "cache.id", IsBicepReference: true},
				},
			},
		},
	}

	result, err := emit(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "connections:") {
		t.Error("expected connections block")
	}

	if !strings.Contains(result, "source: cache.id") {
		t.Error("expected Bicep reference source")
	}
}
