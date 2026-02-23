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
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ScanDirectory(t *testing.T) {
	testCases := []struct {
		name          string
		dir           string
		expectedCount int
		expectedFiles []string
		expectError   bool
	}{
		{
			name:          "aspire-starter discovers 4 bicep files in lexicographic order",
			dir:           "./testdata/aspire-starter",
			expectedCount: 4,
			expectedFiles: []string{
				"apiservice.bicep",
				"cache.bicep",
				"main.bicep",
				"webfrontend.bicep",
			},
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
			files, err := ScanDirectory(tc.dir)

			if tc.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, files, tc.expectedCount)

			// Verify lexicographic file order by base name
			for i, expectedName := range tc.expectedFiles {
				assert.Equal(t, expectedName, filepath.Base(files[i]),
					"file at index %d should be %s", i, expectedName)
			}
		})
	}
}

func Test_ParseFile_MainBicep(t *testing.T) {
	bf, err := ParseFile("./testdata/aspire-starter/main.bicep")
	require.NoError(t, err)

	// Should extract 3 module declarations
	require.Len(t, bf.Modules, 3, "main.bicep should have 3 modules")

	// Verify module names and source paths
	modulesByName := make(map[string]BicepModule)
	for _, m := range bf.Modules {
		modulesByName[m.Name] = m
	}

	t.Run("apiservice module", func(t *testing.T) {
		m, ok := modulesByName["apiservice"]
		require.True(t, ok, "should have apiservice module")
		assert.Equal(t, "./apiservice/apiservice.bicep", m.Source)
		assert.Empty(t, m.DependsOn, "apiservice has no dependsOn")
	})

	t.Run("webfrontend module", func(t *testing.T) {
		m, ok := modulesByName["webfrontend"]
		require.True(t, ok, "should have webfrontend module")
		assert.Equal(t, "./webfrontend/webfrontend.bicep", m.Source)
		assert.Contains(t, m.DependsOn, "apiservice", "webfrontend depends on apiservice")
	})

	t.Run("cache module", func(t *testing.T) {
		m, ok := modulesByName["cache"]
		require.True(t, ok, "should have cache module")
		assert.Equal(t, "./cache/cache.bicep", m.Source)
		assert.Contains(t, m.DependsOn, "apiservice", "cache depends on apiservice")
	})

	// Should extract 2 parameters
	t.Run("parameters", func(t *testing.T) {
		require.Len(t, bf.Parameters, 2, "main.bicep should have 2 parameters")

		paramsByName := make(map[string]BicepParameter)
		for _, p := range bf.Parameters {
			paramsByName[p.Name] = p
		}

		envParam, ok := paramsByName["environmentName"]
		require.True(t, ok, "should have environmentName param")
		assert.Equal(t, "string", envParam.Type)
		assert.Equal(t, "The environment name", envParam.Description)

		locParam, ok := paramsByName["location"]
		require.True(t, ok, "should have location param")
		assert.Equal(t, "string", locParam.Type)
	})
}

func Test_ParseFile_ApiserviceBicep(t *testing.T) {
	bf, err := ParseFile("./testdata/aspire-starter/apiservice/apiservice.bicep")
	require.NoError(t, err)

	// Should extract 1 containerApps resource
	require.Len(t, bf.Resources, 1, "apiservice.bicep should have 1 resource")
	res := bf.Resources[0]

	t.Run("resource type and name", func(t *testing.T) {
		assert.Equal(t, "apiservice", res.SymbolicName)
		assert.Equal(t, "Microsoft.App/containerApps@2024-03-01", res.Type)
	})

	t.Run("ingress properties", func(t *testing.T) {
		ingress, ok := res.Properties["ingress"].(map[string]any)
		require.True(t, ok, "should have ingress properties")
		assert.Equal(t, 8080, ingress["targetPort"], "targetPort should be 8080")
		assert.Equal(t, false, ingress["external"], "external should be false")
	})

	t.Run("containers with env including ConnectionStrings__cache", func(t *testing.T) {
		containers, ok := res.Properties["containers"].([]map[string]any)
		require.True(t, ok, "should have containers")
		require.Len(t, containers, 1, "should have 1 container")

		envVars, ok := containers[0]["env"].([]map[string]string)
		require.True(t, ok, "should have env vars")

		// Find the ConnectionStrings__cache env var
		found := false
		for _, env := range envVars {
			if env["name"] == "ConnectionStrings__cache" {
				found = true
				break
			}
		}
		assert.True(t, found, "should have ConnectionStrings__cache env var")
	})

	// Should extract 2 parameters
	t.Run("parameters", func(t *testing.T) {
		require.Len(t, bf.Parameters, 2, "apiservice.bicep should have 2 parameters")
	})
}

func Test_ParseFile_CacheBicep(t *testing.T) {
	bf, err := ParseFile("./testdata/aspire-starter/cache/cache.bicep")
	require.NoError(t, err)

	// Should extract 1 Redis resource
	require.Len(t, bf.Resources, 1, "cache.bicep should have 1 resource")
	res := bf.Resources[0]

	t.Run("Redis resource type", func(t *testing.T) {
		assert.Equal(t, "cache", res.SymbolicName)
		assert.Equal(t, "Microsoft.Cache/redis@2023-08-01", res.Type)
	})
}

func Test_ParseFile_WebfrontendBicep(t *testing.T) {
	bf, err := ParseFile("./testdata/aspire-starter/webfrontend/webfrontend.bicep")
	require.NoError(t, err)

	require.Len(t, bf.Resources, 1, "webfrontend.bicep should have 1 resource")
	res := bf.Resources[0]

	t.Run("resource type and name", func(t *testing.T) {
		assert.Equal(t, "webfrontend", res.SymbolicName)
		assert.Equal(t, "Microsoft.App/containerApps@2024-03-01", res.Type)
	})

	t.Run("ingress external true", func(t *testing.T) {
		ingress, ok := res.Properties["ingress"].(map[string]any)
		require.True(t, ok, "should have ingress properties")
		assert.Equal(t, true, ingress["external"], "external should be true")
		assert.Equal(t, 8080, ingress["targetPort"], "targetPort should be 8080")
	})

	t.Run("ConnectionStrings__apiservice env var", func(t *testing.T) {
		containers, ok := res.Properties["containers"].([]map[string]any)
		require.True(t, ok, "should have containers")
		require.Len(t, containers, 1)

		envVars, ok := containers[0]["env"].([]map[string]string)
		require.True(t, ok, "should have env vars")

		found := false
		for _, env := range envVars {
			if env["name"] == "ConnectionStrings__apiservice" {
				found = true
				break
			}
		}
		assert.True(t, found, "should have ConnectionStrings__apiservice env var")
	})
}

func Test_ParseFile_NonexistentFile(t *testing.T) {
	_, err := ParseFile("./testdata/nonexistent.bicep")
	require.Error(t, err)
}
