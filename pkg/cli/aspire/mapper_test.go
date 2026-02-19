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

import "testing"

func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resource ManifestResource
		expected ResourceKind
	}{
		{
			name:     "container.v0",
			resource: ManifestResource{Type: "container.v0", Image: "myapp:latest"},
			expected: KindContainer,
		},
		{
			name:     "container.v1",
			resource: ManifestResource{Type: "container.v1", Image: "myapp:latest"},
			expected: KindContainer,
		},
		{
			name:     "unknown type",
			resource: ManifestResource{Type: "executable.v0"},
			expected: KindUnsupported,
		},
		{
			name:     "value.v0",
			resource: ManifestResource{Type: "value.v0"},
			expected: KindValueResource,
		},
		{
			name:     "parameter.v0",
			resource: ManifestResource{Type: "parameter.v0"},
			expected: KindParameter,
		},
		{
			name:     "project.v1",
			resource: ManifestResource{Type: "project.v1", Path: "test.csproj"},
			expected: KindContainer,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := classify(tt.name, tt.resource, nil)
			if result != tt.expected {
				t.Errorf("classify(%q) = %q, want %q", tt.resource.Type, result, tt.expected)
			}
		})
	}
}

func TestClassify_WithOverride(t *testing.T) {
	t.Parallel()

	overrides := map[string]ResourceKind{
		"myredis": KindContainer,
	}

	// Even though the image contains "redis", the override forces it to KindContainer.
	resource := ManifestResource{Type: "container.v0", Image: "redis:latest"}
	result := classify("myredis", resource, overrides)
	if result != KindContainer {
		t.Errorf("expected KindContainer with override, got %q", result)
	}
}

func TestMapContainer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		resource    ManifestResource
		expectImage string
		expectPorts int
		expectEnv   int
	}{
		{
			name: "full container",
			resource: ManifestResource{
				Type:  "container.v0",
				Image: "myapp/api:latest",
				Env:   map[string]string{"PORT": "8080"},
				Bindings: map[string]ManifestBinding{
					"http": {Scheme: "http", Protocol: "tcp", Port: 8080, TargetPort: 8080},
				},
			},
			expectImage: "myapp/api:latest",
			expectPorts: 1,
			expectEnv:   1,
		},
		{
			name: "minimal container",
			resource: ManifestResource{
				Type:  "container.v0",
				Image: "nginx:latest",
			},
			expectImage: "nginx:latest",
			expectPorts: 0,
			expectEnv:   0,
		},
		{
			name: "container with entrypoint and args",
			resource: ManifestResource{
				Type:       "container.v0",
				Image:      "myapp:latest",
				Entrypoint: "/app/start.sh",
				Args:       []string{"--config", "/etc/config.yaml"},
			},
			expectImage: "myapp:latest",
		},
		{
			name: "container with volumes",
			resource: ManifestResource{
				Type:  "container.v0",
				Image: "myapp:latest",
				Volumes: []ManifestVolumeMount{
					{Name: "data", Target: "/data", ReadOnly: false},
				},
			},
			expectImage: "myapp:latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := mapContainer(tt.name, tt.resource, "test", nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Container.Image != tt.expectImage {
				t.Errorf("expected image %q, got %q", tt.expectImage, result.Container.Image)
			}

			if tt.expectPorts > 0 && len(result.Container.Ports) != tt.expectPorts {
				t.Errorf("expected %d ports, got %d", tt.expectPorts, len(result.Container.Ports))
			}

			if tt.expectEnv > 0 && len(result.Container.Env) != tt.expectEnv {
				t.Errorf("expected %d env vars, got %d", tt.expectEnv, len(result.Container.Env))
			}
		})
	}
}

func TestMapContainer_WithEntrypointAndArgs(t *testing.T) {
	t.Parallel()

	resource := ManifestResource{
		Type:       "container.v0",
		Image:      "myapp:latest",
		Entrypoint: "/app/start.sh",
		Args:       []string{"--config", "/etc/config.yaml"},
	}

	result, err := mapContainer("test", resource, "test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Container.Command) != 1 || result.Container.Command[0] != "/app/start.sh" {
		t.Errorf("unexpected command: %v", result.Container.Command)
	}

	if len(result.Container.Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(result.Container.Args))
	}
}

func TestMapContainer_Volumes(t *testing.T) {
	t.Parallel()

	resource := ManifestResource{
		Type:  "container.v0",
		Image: "myapp:latest",
		Volumes: []ManifestVolumeMount{
			{Name: "data", Target: "/data", ReadOnly: true},
		},
	}

	result, err := mapContainer("test", resource, "test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Container.Volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(result.Container.Volumes))
	}

	vol, ok := result.Container.Volumes["data"]
	if !ok {
		t.Fatal("expected volume 'data'")
	}

	if vol.MountPath != "/data" {
		t.Errorf("expected mount path '/data', got %q", vol.MountPath)
	}

	if !vol.ReadOnly {
		t.Error("expected read-only volume")
	}
}

func TestSynthesizeGateway(t *testing.T) {
	t.Parallel()

	t.Run("with external bindings", func(t *testing.T) {
		t.Parallel()

		ctx := &translationContext{
			manifest: &AspireManifest{
				Resources: map[string]ManifestResource{
					"frontend": {
						Type:  "container.v0",
						Image: "frontend:latest",
						Bindings: map[string]ManifestBinding{
							"http": {Scheme: "http", Port: 3000, TargetPort: 3000, External: true},
						},
					},
				},
			},
		}

		gw := synthesizeGateway(ctx)
		if gw == nil {
			t.Fatal("expected gateway to be synthesized")
		}

		if len(gw.Gateway.Routes) != 1 {
			t.Fatalf("expected 1 route, got %d", len(gw.Gateway.Routes))
		}
	})

	t.Run("no external bindings", func(t *testing.T) {
		t.Parallel()

		ctx := &translationContext{
			manifest: &AspireManifest{
				Resources: map[string]ManifestResource{
					"api": {
						Type:  "container.v0",
						Image: "api:latest",
						Bindings: map[string]ManifestBinding{
							"http": {Scheme: "http", Port: 8080, TargetPort: 8080, External: false},
						},
					},
				},
			},
		}

		gw := synthesizeGateway(ctx)
		if gw != nil {
			t.Error("expected no gateway when no external bindings")
		}
	})
}
