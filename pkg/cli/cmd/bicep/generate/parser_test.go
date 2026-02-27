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

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ScanDirectory(t *testing.T) {
	testCases := []struct {
		name        string
		dir         string
		expectError bool
	}{
		{
			name:        "example-aspire-app discovers AppHost infra with 4 tmpl.yaml files and main.bicep",
			dir:         "./testdata/example-aspire-app",
			expectError: false,
		},
		{
			name:        "nonexistent directory returns error",
			dir:         "./testdata/nonexistent",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			descriptor, err := ScanDirectory(tc.dir)

			if tc.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, descriptor)

			// Should discover 4 service templates
			assert.Len(t, descriptor.ServiceTemplates, 4, "should have 4 service templates")

			// Should discover main.bicep
			require.NotNil(t, descriptor.MainBicep, "should have main.bicep")

			// Verify all expected services discovered
			serviceNames := make(map[string]bool)
			for _, st := range descriptor.ServiceTemplates {
				serviceNames[st.ServiceName] = true
			}
			assert.True(t, serviceNames["apiservice"], "should have apiservice")
			assert.True(t, serviceNames["webfrontend"], "should have webfrontend")
			assert.True(t, serviceNames["cache"], "should have cache")
			assert.True(t, serviceNames["sqlserver"], "should have sqlserver")
		})
	}
}

func Test_ParseYAMLTemplate_Apiservice(t *testing.T) {
	st, err := ParseYAMLTemplate("./testdata/example-aspire-app/AspireApp.AppHost/infra/apiservice.tmpl.yaml")
	require.NoError(t, err)

	t.Run("service name from tags", func(t *testing.T) {
		assert.Equal(t, "apiservice", st.ServiceName)
	})

	t.Run("ingress configuration", func(t *testing.T) {
		require.NotNil(t, st.Ingress)
		assert.Equal(t, 8080, st.Ingress.TargetPort)
		assert.Equal(t, "http", st.Ingress.Transport)
		assert.False(t, st.Ingress.External)
	})

	t.Run("containers with ConnectionStrings__weatherdb env var", func(t *testing.T) {
		require.Len(t, st.Containers, 1)

		found := false
		for _, env := range st.Containers[0].Env {
			if env.Name == "ConnectionStrings__weatherdb" {
				found = true
				assert.Equal(t, "connectionstrings--weatherdb", env.SecretRef)
				break
			}
		}
		assert.True(t, found, "should have ConnectionStrings__weatherdb env var")
	})

	t.Run("secrets include connectionstrings--weatherdb", func(t *testing.T) {
		found := false
		for _, s := range st.Secrets {
			if s.Name == "connectionstrings--weatherdb" {
				found = true
				break
			}
		}
		assert.True(t, found, "should have connectionstrings--weatherdb secret")
	})
}

func Test_ParseYAMLTemplate_Cache(t *testing.T) {
	st, err := ParseYAMLTemplate("./testdata/example-aspire-app/AspireApp.AppHost/infra/cache.tmpl.yaml")
	require.NoError(t, err)

	t.Run("service name from tags", func(t *testing.T) {
		assert.Equal(t, "cache", st.ServiceName)
	})

	t.Run("ingress configuration", func(t *testing.T) {
		require.NotNil(t, st.Ingress)
		assert.Equal(t, 6379, st.Ingress.TargetPort)
		assert.Equal(t, "tcp", st.Ingress.Transport)
		assert.False(t, st.Ingress.External)
	})
}

func Test_ParseYAMLTemplate_Webfrontend(t *testing.T) {
	st, err := ParseYAMLTemplate("./testdata/example-aspire-app/AspireApp.AppHost/infra/webfrontend.tmpl.yaml")
	require.NoError(t, err)

	t.Run("external ingress", func(t *testing.T) {
		require.NotNil(t, st.Ingress)
		assert.True(t, st.Ingress.External)
		assert.Equal(t, 8080, st.Ingress.TargetPort)
		assert.Equal(t, "http", st.Ingress.Transport)
	})

	t.Run("has services__apiservice__http__0 env var", func(t *testing.T) {
		require.Len(t, st.Containers, 1)

		found := false
		for _, env := range st.Containers[0].Env {
			if env.Name == "services__apiservice__http__0" {
				found = true
				break
			}
		}
		assert.True(t, found, "should have services__apiservice__http__0 env var")
	})

	t.Run("has ConnectionStrings__cache env var", func(t *testing.T) {
		require.Len(t, st.Containers, 1)

		found := false
		for _, env := range st.Containers[0].Env {
			if env.Name == "ConnectionStrings__cache" {
				found = true
				assert.Equal(t, "connectionstrings--cache", env.SecretRef)
				break
			}
		}
		assert.True(t, found, "should have ConnectionStrings__cache env var")
	})
}

func Test_ParseYAMLTemplate_Sqlserver(t *testing.T) {
	st, err := ParseYAMLTemplate("./testdata/example-aspire-app/AspireApp.AppHost/infra/sqlserver.tmpl.yaml")
	require.NoError(t, err)

	t.Run("service name", func(t *testing.T) {
		assert.Equal(t, "sqlserver", st.ServiceName)
	})

	t.Run("ingress configuration", func(t *testing.T) {
		require.NotNil(t, st.Ingress)
		assert.Equal(t, 1433, st.Ingress.TargetPort)
		assert.Equal(t, "tcp", st.Ingress.Transport)
	})
}

func Test_stripGoTemplateExpressions(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "strips {{ .Image }}",
			input:    "image: {{ .Image }}",
			expected: "image: IMAGE_PLACEHOLDER",
		},
		{
			name:     "strips {{ securedParameter \"name\" }}",
			input:    "value: '{{ securedParameter \"cache_password\" }}'",
			expected: "value: 'SECURED_PARAM_cache_password'",
		},
		{
			name:     "replaces {{ targetPortOrDefault N }} with default value",
			input:    "targetPort: {{ targetPortOrDefault 8080 }}",
			expected: "targetPort: 8080",
		},
		{
			name:     "strips {{ .Env.* }} expressions",
			input:    "value: {{ .Env.AZURE_LOCATION }}",
			expected: "value: ",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := stripGoTemplateExpressions(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_ParseBicepFile_MainBicep(t *testing.T) {
	bf, err := ParseBicepFile("./testdata/example-aspire-app/infra/main.bicep")
	require.NoError(t, err)

	t.Run("extracts parameters", func(t *testing.T) {
		paramsByName := make(map[string]BicepParameter)
		for _, p := range bf.Parameters {
			paramsByName[p.Name] = p
		}

		_, hasEnvName := paramsByName["environmentName"]
		assert.True(t, hasEnvName, "should have environmentName param")
	})
}
