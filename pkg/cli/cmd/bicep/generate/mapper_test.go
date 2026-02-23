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

func Test_MapToRadius_AspireStarter(t *testing.T) {
	// Parse the aspire-starter test fixtures
	filePaths, err := ScanDirectory("./testdata/aspire-starter")
	require.NoError(t, err)

	var parsedFiles []BicepFile
	for _, fp := range filePaths {
		bf, err := ParseFile(fp)
		require.NoError(t, err)
		parsedFiles = append(parsedFiles, bf)
	}

	app, err := MapToRadius(parsedFiles, "", "./testdata/aspire-starter")
	require.NoError(t, err)

	t.Run("application name derived from directory", func(t *testing.T) {
		assert.Equal(t, "aspire-starter", app.Name)
	})

	t.Run("2 containers mapped", func(t *testing.T) {
		require.Len(t, app.Containers, 2, "should have 2 containers")

		containersByName := make(map[string]RadiusContainer)
		for _, c := range app.Containers {
			containersByName[c.Name] = c
		}

		// apiservice container
		api, ok := containersByName["apiservice"]
		require.True(t, ok, "should have apiservice container")
		assert.Equal(t, "apiserviceImage", api.ImageParam)
		assert.Equal(t, "apiservice:latest", api.ImageDefault)
		require.Len(t, api.Ports, 1)
		assert.Equal(t, 8080, api.Ports[0].ContainerPort)
		assert.Equal(t, "TCP", api.Ports[0].Protocol)

		// apiservice → cache connection
		require.Len(t, api.Connections, 1, "apiservice should have 1 connection")
		assert.Equal(t, "cache", api.Connections[0].Name)
		assert.Equal(t, "cache.id", api.Connections[0].Source)

		// webfrontend container
		web, ok := containersByName["webfrontend"]
		require.True(t, ok, "should have webfrontend container")
		assert.Equal(t, "webfrontendImage", web.ImageParam)
		assert.Equal(t, "webfrontend:latest", web.ImageDefault)
		require.Len(t, web.Ports, 1)
		assert.Equal(t, 8080, web.Ports[0].ContainerPort)

		// webfrontend → apiservice connection
		require.Len(t, web.Connections, 1, "webfrontend should have 1 connection")
		assert.Equal(t, "apiservice", web.Connections[0].Name)
		assert.Equal(t, "apiservice.id", web.Connections[0].Source)
	})

	t.Run("1 Redis dependency mapped", func(t *testing.T) {
		require.Len(t, app.Dependencies, 1, "should have 1 dependency")
		dep := app.Dependencies[0]
		assert.Equal(t, "cache", dep.Name)
		assert.Equal(t, "Applications.Datastores/redisCaches", dep.Type)
		assert.True(t, dep.IsRecipeBacked)
		assert.False(t, dep.IsPlaceholder)
	})

	t.Run("2 image parameters generated", func(t *testing.T) {
		require.Len(t, app.Parameters, 2, "should have 2 image parameters")

		paramsByName := make(map[string]RadiusParameter)
		for _, p := range app.Parameters {
			paramsByName[p.Name] = p
		}

		apiParam, ok := paramsByName["apiserviceImage"]
		require.True(t, ok, "should have apiserviceImage param")
		assert.Equal(t, "string", apiParam.Type)
		assert.Equal(t, "apiservice:latest", apiParam.DefaultValue)

		webParam, ok := paramsByName["webfrontendImage"]
		require.True(t, ok, "should have webfrontendImage param")
		assert.Equal(t, "string", webParam.Type)
		assert.Equal(t, "webfrontend:latest", webParam.DefaultValue)
	})

	t.Run("deterministic ordering", func(t *testing.T) {
		// Containers sorted by name
		assert.Equal(t, "apiservice", app.Containers[0].Name)
		assert.Equal(t, "webfrontend", app.Containers[1].Name)

		// Dependencies sorted by name
		assert.Equal(t, "cache", app.Dependencies[0].Name)

		// Parameters sorted by name
		assert.Equal(t, "apiserviceImage", app.Parameters[0].Name)
		assert.Equal(t, "webfrontendImage", app.Parameters[1].Name)
	})
}

func Test_MapToRadius_AppNameOverride(t *testing.T) {
	filePaths, err := ScanDirectory("./testdata/aspire-starter")
	require.NoError(t, err)

	var parsedFiles []BicepFile
	for _, fp := range filePaths {
		bf, err := ParseFile(fp)
		require.NoError(t, err)
		parsedFiles = append(parsedFiles, bf)
	}

	app, err := MapToRadius(parsedFiles, "my-custom-app", "./testdata/aspire-starter")
	require.NoError(t, err)

	assert.Equal(t, "my-custom-app", app.Name, "app name should use override")
}

func Test_deriveAppName(t *testing.T) {
	testCases := []struct {
		name            string
		mainFile        *BicepFile
		appNameOverride string
		sourceDir       string
		expected        string
	}{
		{
			name:            "override takes precedence",
			mainFile:        nil,
			appNameOverride: "my-app",
			sourceDir:       "/tmp/test",
			expected:        "my-app",
		},
		{
			name: "environmentName param from main.bicep",
			mainFile: &BicepFile{
				Parameters: []BicepParameter{
					{Name: "environmentName", Type: "string", DefaultValue: "'aspire-demo'"},
				},
			},
			appNameOverride: "",
			sourceDir:       "/tmp/test",
			expected:        "aspire-demo",
		},
		{
			name:            "falls back to directory name",
			mainFile:        &BicepFile{},
			appNameOverride: "",
			sourceDir:       "/tmp/my-project",
			expected:        "my-project",
		},
		{
			name:            "infra directory goes up one level",
			mainFile:        &BicepFile{},
			appNameOverride: "",
			sourceDir:       "/tmp/my-project/infra",
			expected:        "my-project",
		},
		{
			name:            "empty fallback",
			mainFile:        nil,
			appNameOverride: "",
			sourceDir:       ".",
			expected:        "aspire-app",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := deriveAppName(tc.mainFile, tc.appNameOverride, tc.sourceDir)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_isContainerApp(t *testing.T) {
	assert.True(t, isContainerApp("Microsoft.App/containerApps@2024-03-01"))
	assert.True(t, isContainerApp("Microsoft.App/containerApps@2023-05-01"))
	assert.False(t, isContainerApp("Microsoft.Cache/redis@2023-08-01"))
	assert.False(t, isContainerApp("Microsoft.Sql/servers@2023-08-01"))
}

func Test_isDependencyResource(t *testing.T) {
	assert.True(t, isDependencyResource("Microsoft.Cache/redis@2023-08-01"))
	assert.True(t, isDependencyResource("Microsoft.DocumentDB/databaseAccounts@2024-02-01"))
	assert.True(t, isDependencyResource("Microsoft.Sql/servers@2023-08-01"))
	assert.False(t, isDependencyResource("Microsoft.App/containerApps@2024-03-01"))
}

func Test_extractBaseResourceType(t *testing.T) {
	assert.Equal(t, "Microsoft.Cache/redis", extractBaseResourceType("Microsoft.Cache/redis@2023-08-01"))
	assert.Equal(t, "Microsoft.App/containerApps", extractBaseResourceType("Microsoft.App/containerApps@2024-03-01"))
	assert.Equal(t, "NoVersion", extractBaseResourceType("NoVersion"))
}
