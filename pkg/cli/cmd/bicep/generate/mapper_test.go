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

func Test_MapToRadius_ExampleAspireApp(t *testing.T) {
	descriptor, err := ScanDirectory("./testdata/example-aspire-app")
	require.NoError(t, err)

	app, err := MapToRadius(descriptor, map[string]string{})
	require.NoError(t, err)

	t.Run("application name derived from directory", func(t *testing.T) {
		assert.Equal(t, "example-aspire-app", app.Name)
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

		// webfrontend container
		web, ok := containersByName["webfrontend"]
		require.True(t, ok, "should have webfrontend container")
		assert.Equal(t, "webfrontendImage", web.ImageParam)
		assert.Equal(t, "webfrontend:latest", web.ImageDefault)
		require.Len(t, web.Ports, 1)
		assert.Equal(t, 8080, web.Ports[0].ContainerPort)
		assert.True(t, web.IsExternal, "webfrontend should be external")
	})

	t.Run("2 dependencies mapped", func(t *testing.T) {
		require.Len(t, app.Dependencies, 2, "should have 2 dependencies")

		depsByName := make(map[string]RadiusDependency)
		for _, d := range app.Dependencies {
			depsByName[d.Name] = d
		}

		cache, ok := depsByName["cache"]
		require.True(t, ok, "should have cache dependency")
		assert.Equal(t, "Applications.Datastores/redisCaches", cache.Type)
		assert.True(t, cache.IsRecipeBacked)
		assert.False(t, cache.IsPlaceholder)

		sqlserver, ok := depsByName["sqlserver"]
		require.True(t, ok, "should have sqlserver dependency")
		assert.Equal(t, "Applications.Datastores/sqlDatabases", sqlserver.Type)
		assert.True(t, sqlserver.IsRecipeBacked)
		assert.False(t, sqlserver.IsPlaceholder)
	})

	t.Run("connections derived correctly", func(t *testing.T) {
		containersByName := make(map[string]RadiusContainer)
		for _, c := range app.Containers {
			containersByName[c.Name] = c
		}

		// apiservice → sqlserver (from ConnectionStrings__weatherdb → sqlserver via db heuristic)
		api := containersByName["apiservice"]
		apiConnsByName := make(map[string]RadiusConnection)
		for _, conn := range api.Connections {
			apiConnsByName[conn.Name] = conn
		}
		_, hasSql := apiConnsByName["sqlserver"]
		assert.True(t, hasSql, "apiservice should have sqlserver connection")

		// webfrontend → apiservice (from services__apiservice__http__0)
		// webfrontend → cache (from ConnectionStrings__cache)
		web := containersByName["webfrontend"]
		webConnsByName := make(map[string]RadiusConnection)
		for _, conn := range web.Connections {
			webConnsByName[conn.Name] = conn
		}
		_, hasApi := webConnsByName["apiservice"]
		assert.True(t, hasApi, "webfrontend should have apiservice connection")
		_, hasCache := webConnsByName["cache"]
		assert.True(t, hasCache, "webfrontend should have cache connection")
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
		assert.Equal(t, "sqlserver", app.Dependencies[1].Name)

		// Parameters sorted by name
		assert.Equal(t, "apiserviceImage", app.Parameters[0].Name)
		assert.Equal(t, "webfrontendImage", app.Parameters[1].Name)
	})
}

func Test_MapToRadius_AppNameOverride(t *testing.T) {
	descriptor, err := ScanDirectory("./testdata/example-aspire-app")
	require.NoError(t, err)

	app, err := MapToRadius(descriptor, map[string]string{"app-name": "my-custom-app"})
	require.NoError(t, err)

	assert.Equal(t, "my-custom-app", app.Name, "app name should use override")
}

func Test_MapToRadius_ImageNamespace(t *testing.T) {
	descriptor, err := ScanDirectory("./testdata/example-aspire-app")
	require.NoError(t, err)

	app, err := MapToRadius(descriptor, map[string]string{"image-namespace": "my-namespace"})
	require.NoError(t, err)

	containersByName := make(map[string]RadiusContainer)
	for _, c := range app.Containers {
		containersByName[c.Name] = c
	}

	// Image defaults should be prefixed with namespace
	api := containersByName["apiservice"]
	assert.Equal(t, "my-namespace/apiservice:latest", api.ImageDefault, "apiservice image should have namespace prefix")

	web := containersByName["webfrontend"]
	assert.Equal(t, "my-namespace/webfrontend:latest", web.ImageDefault, "webfrontend image should have namespace prefix")

	// Image parameters should also reflect the namespace in their default values
	paramsByName := make(map[string]RadiusParameter)
	for _, p := range app.Parameters {
		paramsByName[p.Name] = p
	}

	apiParam := paramsByName["apiserviceImage"]
	assert.Equal(t, "my-namespace/apiservice:latest", apiParam.DefaultValue)

	webParam := paramsByName["webfrontendImage"]
	assert.Equal(t, "my-namespace/webfrontend:latest", webParam.DefaultValue)
}

func Test_deriveAppName(t *testing.T) {
	testCases := []struct {
		name       string
		descriptor *AspireAppDescriptor
		override   string
		expected   string
	}{
		{
			name:       "override takes precedence",
			descriptor: &AspireAppDescriptor{RootDir: "/tmp/test"},
			override:   "my-app",
			expected:   "my-app",
		},
		{
			name: "environmentName param from main.bicep",
			descriptor: &AspireAppDescriptor{
				RootDir: "/tmp/test",
				MainBicep: &BicepFile{
					Parameters: []BicepParameter{
						{Name: "environmentName", Type: "string", DefaultValue: "'aspire-demo'"},
					},
				},
			},
			override: "",
			expected: "aspire-demo",
		},
		{
			name:       "falls back to directory name",
			descriptor: &AspireAppDescriptor{RootDir: "/tmp/my-project"},
			override:   "",
			expected:   "my-project",
		},
		{
			name:       "infra directory goes up one level",
			descriptor: &AspireAppDescriptor{RootDir: "/tmp/my-project/infra"},
			override:   "",
			expected:   "my-project",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := deriveAppName(tc.descriptor, tc.override)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_classifyAsDependency(t *testing.T) {
	testCases := []struct {
		name     string
		st       ServiceTemplate
		expected bool
	}{
		{
			name: "cache with tcp transport and port 6379",
			st: ServiceTemplate{
				ServiceName: "cache",
				Ingress:     &IngressConfig{Transport: "tcp", TargetPort: 6379},
			},
			expected: true,
		},
		{
			name: "sqlserver by name",
			st: ServiceTemplate{
				ServiceName: "sqlserver",
				Ingress:     &IngressConfig{Transport: "tcp", TargetPort: 1433},
			},
			expected: true,
		},
		{
			name: "apiservice with http transport",
			st: ServiceTemplate{
				ServiceName: "apiservice",
				Ingress:     &IngressConfig{Transport: "http", TargetPort: 8080},
			},
			expected: false,
		},
		{
			name: "webfrontend with http transport",
			st: ServiceTemplate{
				ServiceName: "webfrontend",
				Ingress:     &IngressConfig{Transport: "http", TargetPort: 8080},
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := classifyAsDependency(tc.st)
			assert.Equal(t, tc.expected, result)
		})
	}
}
