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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_LookupResourceType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		aspireType   string
		wantCategory resourceCategory
		wantRadius   string
	}{
		{
			name:         "container.v0",
			aspireType:   "container.v0",
			wantCategory: categoryContainer,
			wantRadius:   "Radius.Compute/containers@2025-08-01-preview",
		},
		{
			name:         "container.v1",
			aspireType:   "container.v1",
			wantCategory: categoryContainer,
			wantRadius:   "Radius.Compute/containers@2025-08-01-preview",
		},
		{
			name:         "redis.server.v0",
			aspireType:   "redis.server.v0",
			wantCategory: categoryDataStore,
			wantRadius:   "Applications.Datastores/redisCaches@2023-10-01-preview",
		},
		{
			name:         "postgres.server.v0",
			aspireType:   "postgres.server.v0",
			wantCategory: categoryDataStore,
			wantRadius:   "Radius.Data/postgreSqlDatabases@2025-08-01-preview",
		},
		{
			name:         "mysql.server.v0",
			aspireType:   "mysql.server.v0",
			wantCategory: categoryDataStore,
			wantRadius:   "Radius.Data/mySqlDatabases@2025-08-01-preview",
		},
		{
			name:         "parameter.v0",
			aspireType:   "parameter.v0",
			wantCategory: categoryParameter,
		},
		{
			name:         "unknown type returns unsupported",
			aspireType:   "some.unknown.v99",
			wantCategory: categoryUnsupported,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := LookupResourceType(tt.aspireType)
			assert.Equal(t, tt.wantCategory, result.Category)
			if tt.wantRadius != "" {
				assert.Equal(t, tt.wantRadius, result.RadiusType)
			}
		})
	}
}

func Test_ParseExpressionRefs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []ExpressionRef
	}{
		{
			name:  "no references",
			input: "just a plain string",
			want:  []ExpressionRef{},
		},
		{
			name:  "single simple reference",
			input: "{cache.connectionString}",
			want: []ExpressionRef{
				{ResourceName: "cache", PropertyPath: "connectionString", FullMatch: "{cache.connectionString}"},
			},
		},
		{
			name:  "deep property path",
			input: "{cache.bindings.tcp.host}",
			want: []ExpressionRef{
				{ResourceName: "cache", PropertyPath: "bindings.tcp.host", FullMatch: "{cache.bindings.tcp.host}"},
			},
		},
		{
			name:  "multiple references",
			input: "{cache.bindings.tcp.host}:{cache.bindings.tcp.port}",
			want: []ExpressionRef{
				{ResourceName: "cache", PropertyPath: "bindings.tcp.host", FullMatch: "{cache.bindings.tcp.host}"},
				{ResourceName: "cache", PropertyPath: "bindings.tcp.port", FullMatch: "{cache.bindings.tcp.port}"},
			},
		},
		{
			name:  "mixed text and references",
			input: "redis://:{password.value}@{cache.bindings.tcp.host}:6379",
			want: []ExpressionRef{
				{ResourceName: "password", PropertyPath: "value", FullMatch: "{password.value}"},
				{ResourceName: "cache", PropertyPath: "bindings.tcp.host", FullMatch: "{cache.bindings.tcp.host}"},
			},
		},
		{
			name:  "resource name only (no property)",
			input: "{myresource}",
			want: []ExpressionRef{
				{ResourceName: "myresource", PropertyPath: "", FullMatch: "{myresource}"},
			},
		},
		{
			name:  "name with hyphens",
			input: "{cache-password.value}",
			want: []ExpressionRef{
				{ResourceName: "cache-password", PropertyPath: "value", FullMatch: "{cache-password.value}"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ParseExpressionRefs(tt.input)
			require.Len(t, got, len(tt.want))
			for i, ref := range got {
				assert.Equal(t, tt.want[i].ResourceName, ref.ResourceName)
				assert.Equal(t, tt.want[i].PropertyPath, ref.PropertyPath)
				assert.Equal(t, tt.want[i].FullMatch, ref.FullMatch)
			}
		})
	}
}

func Test_ResolveExpression(t *testing.T) {
	t.Parallel()

	manifest := &AspireManifest{
		Resources: map[string]AspireResource{
			"cache": {
				Name: "cache",
				Type: "container.v0",
				Bindings: map[string]AspireBinding{
					"tcp": {
						Name:       "tcp",
						Scheme:     "tcp",
						Protocol:   "tcp",
						TargetPort: 6379,
					},
				},
			},
			"db-password": {
				Name:  "db-password",
				Type:  "parameter.v0",
				Value: "{db-password.inputs.value}",
			},
		},
	}

	tests := []struct {
		name             string
		expr             string
		wantValue        string
		wantBicepExpr    string
	}{
		{
			name:      "static string",
			expr:      "plain value",
			wantValue: "plain value",
		},
		{
			name:          "parameter value reference",
			expr:          "{db-password.value}",
			wantBicepExpr: "db_password",
		},
		{
			name:          "parameter inputs reference",
			expr:          "{db-password.inputs.value}",
			wantBicepExpr: "db_password",
		},
		{
			name:          "binding targetPort reference",
			expr:          "{cache.bindings.tcp.targetPort}",
			wantBicepExpr: "'6379'",
		},
		{
			name:          "binding host reference",
			expr:          "{cache.bindings.tcp.host}",
			wantBicepExpr: "cache.id",
		},
		{
			name:          "connectionString reference",
			expr:          "{cache.connectionString}",
			wantBicepExpr: "cache.id",
		},
	}

	symNameMap := computeSymbolicNames(manifest)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := resolveExpression(tt.expr, manifest, symNameMap)
			if tt.wantValue != "" {
				assert.Equal(t, tt.wantValue, got.Value)
				assert.Empty(t, got.BicepExpression)
			}
			if tt.wantBicepExpr != "" {
				assert.Equal(t, tt.wantBicepExpr, got.BicepExpression)
				assert.Empty(t, got.Value)
			}
		})
	}
}

func Test_mapContainer(t *testing.T) {
	t.Parallel()

	manifest := &AspireManifest{
		Resources: map[string]AspireResource{
			"api": {
				Name:  "api",
				Type:  "container.v0",
				Image: "myregistry/api:latest",
				Bindings: map[string]AspireBinding{
					"http": {
						Name:       "http",
						Scheme:     "http",
						Protocol:   "tcp",
						TargetPort: 8080,
					},
				},
				Env: map[string]string{
					"PORT": "8080",
				},
			},
			"cache": {
				Name: "cache",
				Type: "redis.server.v0",
			},
		},
	}

	symNameMap := computeSymbolicNames(manifest)

	t.Run("basic container mapping", func(t *testing.T) {
		t.Parallel()

		container := mapContainer("api", manifest.Resources["api"], manifest, symNameMap)

		assert.Equal(t, "api", container.SymbolicName)
		assert.Equal(t, "Radius.Compute/containers@2025-08-01-preview", container.TypeName)
		assert.Equal(t, "api", container.Name)
		assert.Equal(t, "myregistry/api:latest", container.Image)
		assert.Equal(t, "app.id", container.ApplicationRef)
		assert.Equal(t, "environment", container.EnvironmentRef)

		// Check ports.
		require.Contains(t, container.Ports, "http")
		assert.Equal(t, 8080, container.Ports["http"].ContainerPort)
		assert.Equal(t, "TCP", container.Ports["http"].Protocol)

		// Check env vars.
		require.Contains(t, container.Env, "PORT")
		assert.Equal(t, "8080", container.Env["PORT"].Value)
	})

	t.Run("container with entrypoint and args", func(t *testing.T) {
		t.Parallel()

		resource := AspireResource{
			Name:       "worker",
			Type:       "container.v0",
			Image:      "worker:latest",
			Entrypoint: "/bin/sh",
			Args:       []string{"-c", "echo hello"},
		}

		container := mapContainer("worker", resource, manifest, symNameMap)
		assert.Equal(t, []string{"/bin/sh", "-c", "echo hello"}, container.Command)
	})

	t.Run("container with build config sets warning", func(t *testing.T) {
		t.Parallel()

		resource := AspireResource{
			Name: "webapp",
			Type: "container.v1",
			Build: &AspireBuild{
				Context:    "src/webapp",
				Dockerfile: "Dockerfile",
			},
		}

		container := mapContainer("webapp", resource, manifest, symNameMap)
		assert.True(t, container.NeedsBuildWarning)
		assert.Equal(t, "src/webapp", container.BuildContext)
		assert.Equal(t, "<YOUR_REGISTRY>/webapp:latest", container.Image)
	})

	t.Run("container with env var referencing another resource", func(t *testing.T) {
		t.Parallel()

		resource := AspireResource{
			Name:  "frontend",
			Type:  "container.v0",
			Image: "frontend:latest",
			Env: map[string]string{
				"CACHE_HOST": "{cache.bindings.tcp.host}",
			},
		}

		manifest := &AspireManifest{
			Resources: map[string]AspireResource{
				"frontend": resource,
				"cache": {
					Name: "cache",
					Type: "redis.server.v0",
					Bindings: map[string]AspireBinding{
						"tcp": {
							Name:       "tcp",
							TargetPort: 6379,
						},
					},
				},
			},
		}

		localSymNameMap := computeSymbolicNames(manifest)
		container := mapContainer("frontend", resource, manifest, localSymNameMap)

		// Should have a connection to cache.
		require.Contains(t, container.Connections, "cache")
		assert.Equal(t, "cache.id", container.Connections["cache"].Source)
	})
}

func Test_mapBindingToPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		binding AspireBinding
		want    BicepPort
	}{
		{
			name: "tcp binding",
			binding: AspireBinding{
				Protocol:   "tcp",
				TargetPort: 6379,
			},
			want: BicepPort{ContainerPort: 6379, Protocol: "TCP"},
		},
		{
			name: "empty protocol defaults to TCP",
			binding: AspireBinding{
				TargetPort: 8080,
			},
			want: BicepPort{ContainerPort: 8080, Protocol: "TCP"},
		},
		{
			name: "http protocol uppercased",
			binding: AspireBinding{
				Protocol:   "http",
				TargetPort: 80,
			},
			want: BicepPort{ContainerPort: 80, Protocol: "HTTP"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := mapBindingToPort(tt.binding)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_generateConnections(t *testing.T) {
	t.Parallel()

	manifest := &AspireManifest{
		Resources: map[string]AspireResource{
			"api": {
				Name:  "api",
				Type:  "container.v0",
				Image: "api:latest",
				Env: map[string]string{
					"CACHE_HOST":           "{cache.bindings.tcp.host}",
					"DB_CONNECTION_STRING": "{db.connectionString}",
					"STATIC_VAR":          "no-reference",
				},
			},
			"cache": {
				Name: "cache",
				Type: "redis.server.v0",
			},
			"db": {
				Name: "db",
				Type: "postgres.server.v0",
			},
			"secret": {
				Name: "secret",
				Type: "parameter.v0",
			},
		},
	}

	symNameMap := computeSymbolicNames(manifest)

	t.Run("generates connections to containers and data stores", func(t *testing.T) {
		t.Parallel()

		connections := generateConnections("api", manifest.Resources["api"], manifest, symNameMap)
		require.Len(t, connections, 2)
		assert.Equal(t, "cache.id", connections["cache"].Source)
		assert.Equal(t, "db.id", connections["db"].Source)
	})

	t.Run("does not generate self-connections", func(t *testing.T) {
		t.Parallel()

		resource := AspireResource{
			Name: "api",
			Type: "container.v0",
			Env: map[string]string{
				"SELF_REF": "{api.bindings.http.targetPort}",
			},
		}

		connections := generateConnections("api", resource, manifest, symNameMap)
		assert.NotContains(t, connections, "api")
	})

	t.Run("does not generate connections to parameters", func(t *testing.T) {
		t.Parallel()

		resource := AspireResource{
			Name: "api",
			Type: "container.v0",
			Env: map[string]string{
				"SECRET_VAL": "{secret.value}",
			},
		}

		connections := generateConnections("api", resource, manifest, symNameMap)
		assert.NotContains(t, connections, "secret")
	})

	t.Run("connections from args references", func(t *testing.T) {
		t.Parallel()

		resource := AspireResource{
			Name: "worker",
			Type: "container.v0",
			Args: []string{"--cache-host", "{cache.bindings.tcp.host}"},
		}

		connections := generateConnections("worker", resource, manifest, symNameMap)
		require.Contains(t, connections, "cache")
		assert.Equal(t, "cache.id", connections["cache"].Source)
	})
}

func Test_mapBackingService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		resourceName string
		resource     AspireResource
		mapping      resourceTypeMapping
		wantSymName  string
		wantType     string
	}{
		{
			name:         "redis mapping",
			resourceName: "cache",
			resource:     AspireResource{Name: "cache", Type: "redis.server.v0"},
			mapping:      LookupResourceType("redis.server.v0"),
			wantSymName:  "cache",
			wantType:     "Applications.Datastores/redisCaches@2023-10-01-preview",
		},
		{
			name:         "postgres mapping",
			resourceName: "my-db",
			resource:     AspireResource{Name: "my-db", Type: "postgres.server.v0"},
			mapping:      LookupResourceType("postgres.server.v0"),
			wantSymName:  "my_db",
			wantType:     "Radius.Data/postgreSqlDatabases@2025-08-01-preview",
		},
		{
			name:         "mysql mapping",
			resourceName: "mysql-db",
			resource:     AspireResource{Name: "mysql-db", Type: "mysql.server.v0"},
			mapping:      LookupResourceType("mysql.server.v0"),
			wantSymName:  "mysql_db",
			wantType:     "Radius.Data/mySqlDatabases@2025-08-01-preview",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			symNameMap := map[string]string{tt.resourceName: sanitizeBicepIdentifier(tt.resourceName)}
			result := mapBackingService(tt.resourceName, tt.mapping, symNameMap)
			assert.Equal(t, tt.wantSymName, result.SymbolicName)
			assert.Equal(t, tt.wantType, result.TypeName)
			assert.Equal(t, tt.resourceName, result.Name)

			// Verify application and environment properties use expressions.
			appProp := result.Properties["application"]
			assert.IsType(t, BicepExpr{}, appProp)
			assert.Equal(t, "app.id", appProp.(BicepExpr).Expression)

			envProp := result.Properties["environment"]
			assert.IsType(t, BicepExpr{}, envProp)
			assert.Equal(t, "environment", envProp.(BicepExpr).Expression)
		})
	}
}

func Test_mapGateway(t *testing.T) {
	t.Parallel()

	t.Run("generates gateway for external binding", func(t *testing.T) {
		t.Parallel()

		resource := AspireResource{
			Name: "web",
			Type: "container.v0",
			Bindings: map[string]AspireBinding{
				"http": {
					External:   true,
					TargetPort: 8080,
				},
			},
		}

		symNameMap := map[string]string{"web": "web"}
		gw := mapGateway("web", resource, symNameMap)
		require.NotNil(t, gw)
		assert.Equal(t, "webGateway", gw.SymbolicName)
		assert.Equal(t, "Radius.Compute/routes@2025-08-01-preview", gw.TypeName)
		assert.Equal(t, "web-gateway", gw.Name)
		assert.Equal(t, "web.id", gw.ContainerRef)
		assert.Equal(t, "app.id", gw.ApplicationRef)
		assert.Equal(t, "environment", gw.EnvironmentRef)
		require.Len(t, gw.Routes, 1)
		assert.Equal(t, "/", gw.Routes[0].Path)
		assert.Equal(t, 8080, gw.Routes[0].Port)
	})

	t.Run("returns nil when no external bindings", func(t *testing.T) {
		t.Parallel()

		resource := AspireResource{
			Name: "api",
			Type: "container.v0",
			Bindings: map[string]AspireBinding{
				"http": {
					External:   false,
					TargetPort: 8080,
				},
			},
		}

		symNameMap := map[string]string{"api": "api"}
		gw := mapGateway("api", resource, symNameMap)
		assert.Nil(t, gw)
	})

	t.Run("returns nil when no bindings", func(t *testing.T) {
		t.Parallel()

		resource := AspireResource{
			Name: "worker",
			Type: "container.v0",
		}

		symNameMap := map[string]string{"worker": "worker"}
		gw := mapGateway("worker", resource, symNameMap)
		assert.Nil(t, gw)
	})
}

func Test_sanitizeBicepIdentifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple name unchanged",
			input: "cache",
			want:  "cache",
		},
		{
			name:  "hyphens become underscores",
			input: "my-resource",
			want:  "my_resource",
		},
		{
			name:  "multiple hyphens",
			input: "cache-password-uri-encoded",
			want:  "cache_password_uri_encoded",
		},
		{
			name:  "starts with digit gets underscore prefix",
			input: "123abc",
			want:  "_123abc",
		},
		{
			name:  "already valid identifier",
			input: "myApp",
			want:  "myApp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := sanitizeBicepIdentifier(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_uniqueSymbolicName(t *testing.T) {
	t.Parallel()

	t.Run("returns name when no collision", func(t *testing.T) {
		t.Parallel()

		used := map[string]bool{"app": true}
		got := uniqueSymbolicName("cache", used)
		assert.Equal(t, "cache", got)
	})

	t.Run("appends Container suffix on collision", func(t *testing.T) {
		t.Parallel()

		used := map[string]bool{"app": true}
		got := uniqueSymbolicName("app", used)
		assert.Equal(t, "appContainer", got)
	})

	t.Run("uses numeric suffix when Container suffix also collides", func(t *testing.T) {
		t.Parallel()

		used := map[string]bool{"app": true, "appContainer": true}
		got := uniqueSymbolicName("app", used)
		assert.Equal(t, "app_2", got)
	})
}

func Test_MapManifest(t *testing.T) {
	t.Parallel()

	t.Run("basic manifest with containers and data stores", func(t *testing.T) {
		t.Parallel()

		manifest := &AspireManifest{
			Resources: map[string]AspireResource{
				"api": {
					Name:  "api",
					Type:  "container.v0",
					Image: "api:latest",
					Bindings: map[string]AspireBinding{
						"http": {
							Scheme:     "http",
							Protocol:   "tcp",
							TargetPort: 8080,
						},
					},
				},
				"cache": {
					Name: "cache",
					Type: "redis.server.v0",
				},
			},
		}

		bicepFile := MapManifest(manifest, "")
		require.NotNil(t, bicepFile)

		// Application resource.
		assert.Equal(t, "app", bicepFile.Application.SymbolicName)
		assert.Equal(t, "Radius.Core/applications@2025-08-01-preview", bicepFile.Application.TypeName)

		// Parameters: environment and applicationName.
		require.Len(t, bicepFile.Parameters, 2)
		assert.Equal(t, "environment", bicepFile.Parameters[0].Name)
		assert.Equal(t, "applicationName", bicepFile.Parameters[1].Name)
		assert.Equal(t, "aspire-app", bicepFile.Parameters[1].DefaultValue)

		// Containers: should have api.
		require.Len(t, bicepFile.Containers, 1)
		assert.Equal(t, "api", bicepFile.Containers[0].SymbolicName)

		// DataStores: should have cache.
		require.Len(t, bicepFile.DataStores, 1)
		assert.Equal(t, "cache", bicepFile.DataStores[0].SymbolicName)

		// Extensions: should include containers and radius.
		assert.Contains(t, bicepFile.Extensions, "containers")
		assert.Contains(t, bicepFile.Extensions, "radius")
	})

	t.Run("custom application name", func(t *testing.T) {
		t.Parallel()

		manifest := &AspireManifest{
			Resources: map[string]AspireResource{
				"svc": {
					Name:  "svc",
					Type:  "container.v0",
					Image: "svc:latest",
				},
			},
		}

		bicepFile := MapManifest(manifest, "my-custom-app")
		assert.Equal(t, "my-custom-app", bicepFile.Parameters[1].DefaultValue)
	})

	t.Run("container named app gets deduplicated symbolic name", func(t *testing.T) {
		t.Parallel()

		manifest := &AspireManifest{
			Resources: map[string]AspireResource{
				"app": {
					Name:  "app",
					Type:  "container.v0",
					Image: "app:latest",
				},
			},
		}

		bicepFile := MapManifest(manifest, "")
		require.Len(t, bicepFile.Containers, 1)
		// The container named "app" should get deduplicated since "app" is reserved for the application resource.
		assert.Equal(t, "appContainer", bicepFile.Containers[0].SymbolicName)
	})

	t.Run("external binding generates gateway", func(t *testing.T) {
		t.Parallel()

		manifest := &AspireManifest{
			Resources: map[string]AspireResource{
				"web": {
					Name:  "web",
					Type:  "container.v0",
					Image: "web:latest",
					Bindings: map[string]AspireBinding{
						"http": {
							External:   true,
							TargetPort: 80,
						},
					},
				},
			},
		}

		bicepFile := MapManifest(manifest, "")
		require.Len(t, bicepFile.Gateways, 1)
		assert.Equal(t, "webGateway", bicepFile.Gateways[0].SymbolicName)
		assert.Equal(t, "web-gateway", bicepFile.Gateways[0].Name)
	})

	t.Run("unsupported resource type adds comment and warning", func(t *testing.T) {
		t.Parallel()

		manifest := &AspireManifest{
			Resources: map[string]AspireResource{
				"mystery": {
					Name: "mystery",
					Type: "some.unknown.v99",
				},
			},
		}

		bicepFile := MapManifest(manifest, "")
		require.Len(t, bicepFile.Comments, 1)
		assert.Equal(t, "mystery", bicepFile.Comments[0].ResourceName)
		assert.Equal(t, "some.unknown.v99", bicepFile.Comments[0].ResourceType)
		require.Len(t, bicepFile.Warnings, 1)
		assert.Contains(t, bicepFile.Warnings[0], "unsupported resource type")
	})

	t.Run("parameter.v0 resources are skipped (not in containers or data stores)", func(t *testing.T) {
		t.Parallel()

		manifest := &AspireManifest{
			Resources: map[string]AspireResource{
				"api": {
					Name:  "api",
					Type:  "container.v0",
					Image: "api:latest",
				},
				"db-password": {
					Name:  "db-password",
					Type:  "parameter.v0",
					Value: "{db-password.inputs.value}",
				},
			},
		}

		bicepFile := MapManifest(manifest, "")
		assert.Len(t, bicepFile.Containers, 1)
		assert.Len(t, bicepFile.DataStores, 0)
		assert.Len(t, bicepFile.Comments, 0)
	})

	t.Run("extensions are sorted and deduplicated", func(t *testing.T) {
		t.Parallel()

		manifest := &AspireManifest{
			Resources: map[string]AspireResource{
				"api": {
					Name:  "api",
					Type:  "container.v0",
					Image: "api:latest",
				},
				"db": {
					Name: "db",
					Type: "postgres.server.v0",
				},
			},
		}

		bicepFile := MapManifest(manifest, "")
		// Should have containers, radius, radiusResources — sorted.
		for i := 0; i < len(bicepFile.Extensions)-1; i++ {
			assert.True(t, bicepFile.Extensions[i] < bicepFile.Extensions[i+1],
				"extensions should be sorted: %v", bicepFile.Extensions)
		}
	})

	t.Run("errored resource adds skipped comment and warning", func(t *testing.T) {
		t.Parallel()

		manifest := &AspireManifest{
			Resources: map[string]AspireResource{
				"docker-hub": {
					Name:  "docker-hub",
					Error: "This resource does not support generation in the manifest.",
				},
			},
		}

		bicepFile := MapManifest(manifest, "")
		require.Len(t, bicepFile.Comments, 1)
		assert.Equal(t, "docker-hub", bicepFile.Comments[0].ResourceName)
		assert.Equal(t, "", bicepFile.Comments[0].ResourceType)
		assert.Contains(t, bicepFile.Comments[0].Message, "manifest error")
		assert.Contains(t, bicepFile.Comments[0].Message, "This resource does not support generation in the manifest.")
		require.Len(t, bicepFile.Warnings, 1)
		assert.Contains(t, bicepFile.Warnings[0], "manifest error")
		assert.Contains(t, bicepFile.Warnings[0], "docker-hub")
		// Should have no containers or data stores.
		assert.Empty(t, bicepFile.Containers)
		assert.Empty(t, bicepFile.DataStores)
	})

	t.Run("errored resource alongside valid resources", func(t *testing.T) {
		t.Parallel()

		manifest := &AspireManifest{
			Resources: map[string]AspireResource{
				"api": {
					Name:  "api",
					Type:  "container.v0",
					Image: "api:latest",
				},
				"broken": {
					Name:  "broken",
					Error: "Something went wrong.",
				},
				"db": {
					Name: "db",
					Type: "redis.server.v0",
				},
			},
		}

		bicepFile := MapManifest(manifest, "")
		// Valid resources should still be mapped.
		assert.Len(t, bicepFile.Containers, 1)
		assert.Len(t, bicepFile.DataStores, 1)
		// Errored resource should produce a comment and warning.
		require.Len(t, bicepFile.Comments, 1)
		assert.Equal(t, "broken", bicepFile.Comments[0].ResourceName)
		require.Len(t, bicepFile.Warnings, 1)
		assert.Contains(t, bicepFile.Warnings[0], "broken")
	})

	t.Run("errored and unsupported resources produce separate comments", func(t *testing.T) {
		t.Parallel()

		manifest := &AspireManifest{
			Resources: map[string]AspireResource{
				"api": {
					Name:  "api",
					Type:  "container.v0",
					Image: "api:latest",
				},
				"errored": {
					Name:  "errored",
					Error: "Publisher blew up.",
				},
				"unknown": {
					Name: "unknown",
					Type: "weird.thing.v42",
				},
			},
		}

		bicepFile := MapManifest(manifest, "")
		assert.Len(t, bicepFile.Containers, 1)
		require.Len(t, bicepFile.Comments, 2)

		// Comments should be in sorted order (errored < unknown).
		assert.Equal(t, "errored", bicepFile.Comments[0].ResourceName)
		assert.Equal(t, "", bicepFile.Comments[0].ResourceType)
		assert.Contains(t, bicepFile.Comments[0].Message, "manifest error")

		assert.Equal(t, "unknown", bicepFile.Comments[1].ResourceName)
		assert.Equal(t, "weird.thing.v42", bicepFile.Comments[1].ResourceType)
		assert.Contains(t, bicepFile.Comments[1].Message, "manual configuration required")
	})

	t.Run("buildOnly resource is excluded from conversion", func(t *testing.T) {
		t.Parallel()

		manifest := &AspireManifest{
			Resources: map[string]AspireResource{
				"api": {
					Name:  "api",
					Type:  "container.v1",
					Build: &AspireBuild{Context: "api", Dockerfile: "Dockerfile"},
				},
				"frontend": {
					Name: "frontend",
					Type: "container.v1",
					Build: &AspireBuild{
						Context:    "frontend",
						Dockerfile: "frontend.Dockerfile",
						BuildOnly:  true,
					},
				},
			},
		}

		bicepFile := MapManifest(manifest, "")
		// Only api should be converted — frontend is buildOnly.
		require.Len(t, bicepFile.Containers, 1)
		assert.Equal(t, "api", bicepFile.Containers[0].Name)

		// frontend should produce a skipped comment and warning.
		require.Len(t, bicepFile.Comments, 1)
		assert.Equal(t, "frontend", bicepFile.Comments[0].ResourceName)
		assert.Equal(t, "", bicepFile.Comments[0].ResourceType)
		assert.Contains(t, bicepFile.Comments[0].Message, "build-only artifact")
		assert.Contains(t, bicepFile.Comments[0].Message, "build.buildOnly: true")

		require.Len(t, bicepFile.Warnings, 1)
		assert.Contains(t, bicepFile.Warnings[0], "frontend")
		assert.Contains(t, bicepFile.Warnings[0], "build-only artifact")
	})

	t.Run("buildOnly resource does not get gateway or connections", func(t *testing.T) {
		t.Parallel()

		manifest := &AspireManifest{
			Resources: map[string]AspireResource{
				"frontend": {
					Name: "frontend",
					Type: "container.v1",
					Build: &AspireBuild{
						Context:    "frontend",
						Dockerfile: "Dockerfile",
						BuildOnly:  true,
					},
					Bindings: map[string]AspireBinding{
						"http": {
							External:   true,
							TargetPort: 3000,
						},
					},
				},
			},
		}

		bicepFile := MapManifest(manifest, "")
		assert.Empty(t, bicepFile.Containers)
		assert.Empty(t, bicepFile.Gateways)
		require.Len(t, bicepFile.Comments, 1)
		assert.Contains(t, bicepFile.Comments[0].Message, "build-only artifact")
	})

	t.Run("errored buildOnly and unsupported resources all produce separate comments", func(t *testing.T) {
		t.Parallel()

		manifest := &AspireManifest{
			Resources: map[string]AspireResource{
				"api": {
					Name:  "api",
					Type:  "container.v0",
					Image: "api:latest",
				},
				"broken": {
					Name:  "broken",
					Error: "Cannot generate.",
				},
				"builder": {
					Name: "builder",
					Type: "container.v1",
					Build: &AspireBuild{
						Context:   "build",
						BuildOnly: true,
					},
				},
				"mystery": {
					Name: "mystery",
					Type: "alien.v99",
				},
			},
		}

		bicepFile := MapManifest(manifest, "")
		assert.Len(t, bicepFile.Containers, 1)
		require.Len(t, bicepFile.Comments, 3)
		require.Len(t, bicepFile.Warnings, 3)

		// In sorted resource name order: broken, builder, mystery.
		assert.Equal(t, "broken", bicepFile.Comments[0].ResourceName)
		assert.Contains(t, bicepFile.Comments[0].Message, "manifest error")

		assert.Equal(t, "builder", bicepFile.Comments[1].ResourceName)
		assert.Contains(t, bicepFile.Comments[1].Message, "build-only artifact")

		assert.Equal(t, "mystery", bicepFile.Comments[2].ResourceName)
		assert.Contains(t, bicepFile.Comments[2].Message, "manual configuration required")
	})

	t.Run("non-buildOnly build container still gets build warning", func(t *testing.T) {
		t.Parallel()

		manifest := &AspireManifest{
			Resources: map[string]AspireResource{
				"webapp": {
					Name: "webapp",
					Type: "container.v1",
					Build: &AspireBuild{
						Context:    "src/webapp",
						Dockerfile: "Dockerfile",
						BuildOnly:  false,
					},
				},
			},
		}

		bicepFile := MapManifest(manifest, "")
		require.Len(t, bicepFile.Containers, 1)
		assert.True(t, bicepFile.Containers[0].NeedsBuildWarning)
		assert.Equal(t, "src/webapp", bicepFile.Containers[0].BuildContext)
		assert.Empty(t, bicepFile.Comments)
	})
}
