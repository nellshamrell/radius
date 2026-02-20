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
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Parse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		wantErr     string
		validate    func(t *testing.T, m *AspireManifest)
	}{
		{
			name: "valid container resource",
			input: `{
				"resources": {
					"myapp": {
						"type": "container.v0",
						"image": "myregistry/myapp:latest",
						"bindings": {
							"http": {
								"scheme": "http",
								"protocol": "tcp",
								"targetPort": 8080
							}
						},
						"env": {
							"PORT": "8080"
						}
					}
				}
			}`,
			validate: func(t *testing.T, m *AspireManifest) {
				t.Helper()
				require.Len(t, m.Resources, 1)

				r := m.Resources["myapp"]
				assert.Equal(t, "myapp", r.Name)
				assert.Equal(t, "container.v0", r.Type)
				assert.Equal(t, "myregistry/myapp:latest", r.Image)

				require.Len(t, r.Bindings, 1)
				b := r.Bindings["http"]
				assert.Equal(t, "http", b.Name)
				assert.Equal(t, "http", b.Scheme)
				assert.Equal(t, "tcp", b.Protocol)
				assert.Equal(t, 8080, b.TargetPort)

				require.Len(t, r.Env, 1)
				assert.Equal(t, "8080", r.Env["PORT"])
			},
		},
		{
			name: "valid backing service",
			input: `{
				"resources": {
					"redis": {
						"type": "redis.server.v0",
						"connectionString": "{redis.bindings.tcp.host}:{redis.bindings.tcp.port}",
						"bindings": {
							"tcp": {
								"scheme": "tcp",
								"protocol": "tcp",
								"targetPort": 6379
							}
						}
					}
				}
			}`,
			validate: func(t *testing.T, m *AspireManifest) {
				t.Helper()
				require.Len(t, m.Resources, 1)

				r := m.Resources["redis"]
				assert.Equal(t, "redis", r.Name)
				assert.Equal(t, "redis.server.v0", r.Type)
				assert.Contains(t, r.ConnectionString, "{redis.bindings.tcp.host}")
			},
		},
		{
			name: "valid parameter with secret",
			input: `{
				"resources": {
					"db-password": {
						"type": "parameter.v0",
						"value": "{db-password.inputs.value}",
						"inputs": {
							"value": {
								"type": "string",
								"secret": true,
								"default": {
									"generate": {
										"minLength": 16,
										"special": false
									}
								}
							}
						}
					}
				}
			}`,
			validate: func(t *testing.T, m *AspireManifest) {
				t.Helper()
				require.Len(t, m.Resources, 1)

				r := m.Resources["db-password"]
				assert.Equal(t, "parameter.v0", r.Type)
				assert.Equal(t, "{db-password.inputs.value}", r.Value)

				require.Len(t, r.Inputs, 1)
				input := r.Inputs["value"]
				assert.True(t, input.Secret)
				assert.Equal(t, "string", input.Type)
				require.NotNil(t, input.Default)
				require.NotNil(t, input.Default.Generate)
				assert.Equal(t, 16, input.Default.Generate.MinLength)
				assert.False(t, input.Default.Generate.Special)
			},
		},
		{
			name: "container.v1 with build config",
			input: `{
				"resources": {
					"webapp": {
						"type": "container.v1",
						"build": {
							"context": "src/webapp",
							"dockerfile": "Dockerfile"
						},
						"bindings": {
							"http": {
								"scheme": "http",
								"protocol": "tcp",
								"targetPort": 3000
							}
						}
					}
				}
			}`,
			validate: func(t *testing.T, m *AspireManifest) {
				t.Helper()
				r := m.Resources["webapp"]
				assert.Equal(t, "container.v1", r.Type)
				require.NotNil(t, r.Build)
				assert.Equal(t, "src/webapp", r.Build.Context)
				assert.Equal(t, "Dockerfile", r.Build.Dockerfile)
			},
		},
		{
			name: "multiple resources",
			input: `{
				"resources": {
					"api": {
						"type": "container.v0",
						"image": "api:latest"
					},
					"db": {
						"type": "postgres.server.v0",
						"connectionString": "{db.bindings.tcp.host}"
					}
				}
			}`,
			validate: func(t *testing.T, m *AspireManifest) {
				t.Helper()
				require.Len(t, m.Resources, 2)
				assert.Equal(t, "api", m.Resources["api"].Name)
				assert.Equal(t, "db", m.Resources["db"].Name)
			},
		},
		{
			name: "schema field is parsed",
			input: `{
				"$schema": "https://json.schemastore.org/aspire-8.0.json",
				"resources": {
					"svc": {
						"type": "container.v0",
						"image": "svc:latest"
					}
				}
			}`,
			validate: func(t *testing.T, m *AspireManifest) {
				t.Helper()
				assert.Equal(t, "https://json.schemastore.org/aspire-8.0.json", m.Schema)
			},
		},
		{
			name:    "missing type field",
			input:   `{"resources": {"bad": {}}}`,
			wantErr: `resource "bad" is missing required field "type"`,
		},
		{
			name:    "empty type field",
			input:   `{"resources": {"bad": {"type": "  "}}}`,
			wantErr: `resource "bad" is missing required field "type"`,
		},
		{
			name:    "empty resources",
			input:   `{"resources": {}}`,
			wantErr: "manifest contains no resources",
		},
		{
			name:    "null resources",
			input:   `{"resources": null}`,
			wantErr: "manifest contains no resources",
		},
		{
			name:    "missing resources key",
			input:   `{}`,
			wantErr: "manifest contains no resources",
		},
		{
			name:    "malformed JSON",
			input:   `{not valid json`,
			wantErr: "invalid JSON",
		},
		{
			name:    "empty input",
			input:   ``,
			wantErr: "invalid JSON",
		},
		{
			name: "unknown resource type is still parsed",
			input: `{
				"resources": {
					"mystery": {
						"type": "some.unknown.v99"
					}
				}
			}`,
			validate: func(t *testing.T, m *AspireManifest) {
				t.Helper()
				require.Len(t, m.Resources, 1)
				assert.Equal(t, "some.unknown.v99", m.Resources["mystery"].Type)
			},
		},
		{
			name: "container with entrypoint and args",
			input: `{
				"resources": {
					"worker": {
						"type": "container.v0",
						"image": "worker:latest",
						"entrypoint": "/bin/sh",
						"args": ["-c", "echo hello"]
					}
				}
			}`,
			validate: func(t *testing.T, m *AspireManifest) {
				t.Helper()
				r := m.Resources["worker"]
				assert.Equal(t, "/bin/sh", r.Entrypoint)
				assert.Equal(t, []string{"-c", "echo hello"}, r.Args)
			},
		},
		{
			name: "binding with external flag",
			input: `{
				"resources": {
					"web": {
						"type": "container.v0",
						"image": "web:latest",
						"bindings": {
							"http": {
								"scheme": "http",
								"protocol": "tcp",
								"targetPort": 80,
								"external": true
							}
						}
					}
				}
			}`,
			validate: func(t *testing.T, m *AspireManifest) {
				t.Helper()
				b := m.Resources["web"].Bindings["http"]
				assert.True(t, b.External)
				assert.Equal(t, 80, b.TargetPort)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			manifest, err := Parse([]byte(tt.input))
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, manifest)
			if tt.validate != nil {
				tt.validate(t, manifest)
			}
		})
	}
}

func Test_Parse_SampleManifest(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/aspire-manifest.json")
	require.NoError(t, err)

	manifest, err := Parse(data)
	require.NoError(t, err)
	require.NotNil(t, manifest)

	assert.Equal(t, "https://json.schemastore.org/aspire-8.0.json", manifest.Schema)
	assert.Len(t, manifest.Resources, 5)

	// Verify specific resources.
	cache := manifest.Resources["cache"]
	assert.Equal(t, "container.v0", cache.Type)
	assert.Equal(t, "docker.io/library/redis:8.2", cache.Image)
	assert.Equal(t, "/bin/sh", cache.Entrypoint)

	app := manifest.Resources["app"]
	assert.Equal(t, "container.v1", app.Type)
	require.NotNil(t, app.Build)
	assert.Equal(t, "app", app.Build.Context)

	cachePassword := manifest.Resources["cache-password"]
	assert.Equal(t, "parameter.v0", cachePassword.Type)
	assert.True(t, cachePassword.Inputs["value"].Secret)

	unsupported := manifest.Resources["cache-password-uri-encoded"]
	assert.Equal(t, "annotated.string", unsupported.Type)
}
