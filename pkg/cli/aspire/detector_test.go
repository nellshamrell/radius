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

func TestDetectBackingService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		image    string
		expected ResourceKind
	}{
		{
			name:     "official redis",
			image:    "redis:7",
			expected: KindRedisCache,
		},
		{
			name:     "bitnami redis",
			image:    "docker.io/bitnami/redis:latest",
			expected: KindRedisCache,
		},
		{
			name:     "official postgres",
			image:    "postgres:14",
			expected: KindSQLDB,
		},
		{
			name:     "private registry postgres",
			image:    "myregistry.io/library/postgres:14-alpine",
			expected: KindSQLDB,
		},
		{
			name:     "mysql",
			image:    "mysql:8",
			expected: KindSQLDB,
		},
		{
			name:     "mariadb",
			image:    "mariadb:10",
			expected: KindSQLDB,
		},
		{
			name:     "mongo",
			image:    "mongo:6",
			expected: KindMongoDB,
		},
		{
			name:     "mongodb private mirror",
			image:    "ghcr.io/company/mongodb:5.0",
			expected: KindMongoDB,
		},
		{
			name:     "rabbitmq",
			image:    "rabbitmq:3-management",
			expected: KindRabbitMQ,
		},
		{
			name:     "unknown image",
			image:    "nginx:latest",
			expected: KindUnsupported,
		},
		{
			name:     "custom app image",
			image:    "myapp/api:v1.0",
			expected: KindUnsupported,
		},
		{
			name:     "case insensitive redis",
			image:    "Redis:latest",
			expected: KindRedisCache,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := detectBackingService(tt.image)
			if result != tt.expected {
				t.Errorf("detectBackingService(%q) = %q, want %q", tt.image, result, tt.expected)
			}
		})
	}
}

func TestExtractBaseImageName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		image    string
		expected string
	}{
		{
			name:     "simple image",
			image:    "redis",
			expected: "redis",
		},
		{
			name:     "image with tag",
			image:    "redis:7",
			expected: "redis",
		},
		{
			name:     "image with registry",
			image:    "docker.io/bitnami/redis:latest",
			expected: "redis",
		},
		{
			name:     "image with library path",
			image:    "myregistry.io/library/postgres:14",
			expected: "postgres",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := extractBaseImageName(tt.image)
			if result != tt.expected {
				t.Errorf("extractBaseImageName(%q) = %q, want %q", tt.image, result, tt.expected)
			}
		})
	}
}

func TestClassify_BackingServiceDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resName  string
		resource ManifestResource
		expected ResourceKind
	}{
		{
			name:     "redis image detected as cache",
			resName:  "cache",
			resource: ManifestResource{Type: "container.v0", Image: "redis:7"},
			expected: KindRedisCache,
		},
		{
			name:     "postgres image detected as sql",
			resName:  "db",
			resource: ManifestResource{Type: "container.v0", Image: "postgres:14"},
			expected: KindSQLDB,
		},
		{
			name:     "override forces container",
			resName:  "myredis",
			resource: ManifestResource{Type: "container.v0", Image: "redis:latest"},
			expected: KindContainer,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var overrides map[string]ResourceKind
			if tt.name == "override forces container" {
				overrides = map[string]ResourceKind{
					"myredis": KindContainer,
				}
			}

			result := classify(tt.resName, tt.resource, overrides)
			if result != tt.expected {
				t.Errorf("classify(%q) = %q, want %q", tt.resName, result, tt.expected)
			}
		})
	}
}
