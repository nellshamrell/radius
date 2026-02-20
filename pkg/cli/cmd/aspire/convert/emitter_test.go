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
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// normalizeTimestamp replaces the timestamp line in emitter output with a fixed value
// so golden file comparisons are deterministic.
func normalizeTimestamp(s string) string {
	re := regexp.MustCompile(`// Date: \S+`)
	return re.ReplaceAllString(s, "// Date: 2026-02-19T00:00:00Z")
}

func Test_Emit_BasicGoldenFile(t *testing.T) {
	t.Parallel()

	// Step 1: Read the sample manifest.
	data, err := os.ReadFile("testdata/aspire-manifest.json")
	require.NoError(t, err)

	// Step 2: Parse the manifest.
	manifest, err := Parse(data)
	require.NoError(t, err)

	// Step 3: Map to Bicep IR.
	bicepFile := MapManifest(manifest, "")

	// Step 4: Emit Bicep text.
	got, err := Emit(bicepFile, "./aspire-manifest.json")
	require.NoError(t, err)

	// Step 5: Read the golden file.
	expected, err := os.ReadFile("testdata/expected-basic.bicep")
	require.NoError(t, err)

	// Normalize timestamps and line endings for comparison.
	gotNormalized := normalizeTimestamp(strings.ReplaceAll(got, "\r\n", "\n"))
	expectedNormalized := normalizeTimestamp(strings.ReplaceAll(string(expected), "\r\n", "\n"))

	assert.Equal(t, expectedNormalized, gotNormalized,
		"Emitted Bicep output does not match the golden file expected-basic.bicep.\n"+
			"Update the golden file if the change is intentional.")
}

func Test_Emit_FullGoldenFile_InvalidManifestField(t *testing.T) {
	t.Parallel()

	// Step 1: Read the invalid manifest (contains an errored resource).
	data, err := os.ReadFile("testdata/aspire-manifest-invalid-manifest-field.json")
	require.NoError(t, err)

	// Step 2: Parse the manifest.
	manifest, err := Parse(data)
	require.NoError(t, err)

	// Step 3: Map to Bicep IR.
	bicepFile := MapManifest(manifest, "")

	// Step 4: Emit Bicep text.
	got, err := Emit(bicepFile, "./aspire-manifest-invalid-manifest-field.json")
	require.NoError(t, err)

	// Step 5: Read the golden file.
	expected, err := os.ReadFile("testdata/expected-full.bicep")
	require.NoError(t, err)

	// Normalize timestamps and line endings for comparison.
	gotNormalized := normalizeTimestamp(strings.ReplaceAll(got, "\r\n", "\n"))
	expectedNormalized := normalizeTimestamp(strings.ReplaceAll(string(expected), "\r\n", "\n"))

	assert.Equal(t, expectedNormalized, gotNormalized,
		"Emitted Bicep output does not match the golden file expected-full.bicep.\n"+
			"Update the golden file if the change is intentional.")
}

func Test_Emit_EmptyContainers(t *testing.T) {
	t.Parallel()

	// Verify that emitting a BicepFile with only an application resource works.
	bicepFile := &BicepFile{
		Extensions: []string{"radius"},
		Parameters: []BicepParameter{
			{
				Name:        "environment",
				Type:        "string",
				Description: "The ID of your Radius Environment. Set automatically by the rad CLI.",
			},
			{
				Name:         "applicationName",
				Type:         "string",
				Description:  "The name of the Radius Application.",
				DefaultValue: "test-app",
			},
		},
		Application: BicepResource{
			SymbolicName: "app",
			TypeName:     "Radius.Core/applications@2025-08-01-preview",
			Name:         "applicationName",
			Properties: map[string]any{
				"environment": BicepExpr{Expression: "environment"},
			},
		},
	}

	got, err := Emit(bicepFile, "test.json")
	require.NoError(t, err)

	assert.Contains(t, got, "extension radius")
	assert.Contains(t, got, "param environment string")
	assert.Contains(t, got, "param applicationName string = 'test-app'")
	assert.Contains(t, got, "resource app 'Radius.Core/applications@2025-08-01-preview'")
	assert.NotContains(t, got, "container:")
}

func Test_Emit_ContainerWithConnections(t *testing.T) {
	t.Parallel()

	bicepFile := &BicepFile{
		Extensions: []string{"containers", "radius"},
		Parameters: []BicepParameter{
			{
				Name:        "environment",
				Type:        "string",
				Description: "The ID of your Radius Environment. Set automatically by the rad CLI.",
			},
			{
				Name:         "applicationName",
				Type:         "string",
				Description:  "The name of the Radius Application.",
				DefaultValue: "test-app",
			},
		},
		Application: BicepResource{
			SymbolicName: "app",
			TypeName:     "Radius.Core/applications@2025-08-01-preview",
			Name:         "applicationName",
			Properties: map[string]any{
				"environment": BicepExpr{Expression: "environment"},
			},
		},
		Containers: []BicepContainer{
			{
				SymbolicName:   "api",
				TypeName:       "Radius.Compute/containers@2025-08-01-preview",
				Name:           "api",
				Image:          "api:latest",
				ApplicationRef: "app.id",
				EnvironmentRef: "environment",
				Ports: map[string]BicepPort{
					"http": {ContainerPort: 8080, Protocol: "TCP"},
				},
				Env: map[string]BicepEnvVar{
					"PORT": {Value: "8080"},
				},
				Connections: map[string]BicepConnection{
					"cache": {Source: "cache.id"},
				},
			},
		},
	}

	got, err := Emit(bicepFile, "test.json")
	require.NoError(t, err)

	assert.Contains(t, got, "resource api 'Radius.Compute/containers@2025-08-01-preview'")
	assert.Contains(t, got, "image: 'api:latest'")
	assert.Contains(t, got, "containerPort: 8080")
	assert.Contains(t, got, "PORT: '8080'")
	assert.Contains(t, got, "connections:")
	assert.Contains(t, got, "source: cache.id")
}

func Test_Emit_DataStoresAndGateways(t *testing.T) {
	t.Parallel()

	bicepFile := &BicepFile{
		Extensions: []string{"containers", "radius"},
		Parameters: []BicepParameter{
			{
				Name:        "environment",
				Type:        "string",
				Description: "The ID of your Radius Environment. Set automatically by the rad CLI.",
			},
			{
				Name:         "applicationName",
				Type:         "string",
				Description:  "The name of the Radius Application.",
				DefaultValue: "test-app",
			},
		},
		Application: BicepResource{
			SymbolicName: "app",
			TypeName:     "Radius.Core/applications@2025-08-01-preview",
			Name:         "applicationName",
			Properties: map[string]any{
				"environment": BicepExpr{Expression: "environment"},
			},
		},
		DataStores: []BicepResource{
			{
				SymbolicName: "cache",
				TypeName:     "Applications.Datastores/redisCaches@2023-10-01-preview",
				Name:         "cache",
				Properties: map[string]any{
					"application": BicepExpr{Expression: "app.id"},
					"environment": BicepExpr{Expression: "environment"},
				},
			},
		},
		Gateways: []BicepGateway{
			{
				SymbolicName:   "webGateway",
				TypeName:       "Radius.Compute/routes@2025-08-01-preview",
				Name:           "web-gateway",
				ContainerRef:   "web.id",
				ApplicationRef: "app.id",
				EnvironmentRef: "environment",
				Routes: []BicepGatewayRoute{
					{Path: "/", Port: 80},
				},
			},
		},
	}

	got, err := Emit(bicepFile, "test.json")
	require.NoError(t, err)

	assert.Contains(t, got, "resource cache 'Applications.Datastores/redisCaches@2023-10-01-preview'")
	assert.Contains(t, got, "resource webGateway 'Radius.Compute/routes@2025-08-01-preview'")
	assert.Contains(t, got, "path: '/'")
	assert.Contains(t, got, "port: 80")
}

func Test_Emit_UnsupportedResourceComments(t *testing.T) {
	t.Parallel()

	bicepFile := &BicepFile{
		Extensions: []string{"radius"},
		Parameters: []BicepParameter{
			{
				Name:        "environment",
				Type:        "string",
				Description: "Environment.",
			},
			{
				Name:         "applicationName",
				Type:         "string",
				Description:  "App name.",
				DefaultValue: "test",
			},
		},
		Application: BicepResource{
			SymbolicName: "app",
			TypeName:     "Radius.Core/applications@2025-08-01-preview",
			Name:         "applicationName",
			Properties: map[string]any{
				"environment": BicepExpr{Expression: "environment"},
			},
		},
		Comments: []BicepComment{
			{
				ResourceName: "mystery",
				ResourceType: "some.unknown.v99",
				Message:      "manual configuration required",
			},
		},
	}

	got, err := Emit(bicepFile, "test.json")
	require.NoError(t, err)

	assert.Contains(t, got, "// Unsupported: mystery (some.unknown.v99)")
	assert.Contains(t, got, "manual configuration required")
}

func Test_Emit_ErroredResourceComments(t *testing.T) {
	t.Parallel()

	bicepFile := &BicepFile{
		Extensions: []string{"radius"},
		Parameters: []BicepParameter{
			{
				Name:        "environment",
				Type:        "string",
				Description: "Environment.",
			},
			{
				Name:         "applicationName",
				Type:         "string",
				Description:  "App name.",
				DefaultValue: "test",
			},
		},
		Application: BicepResource{
			SymbolicName: "app",
			TypeName:     "Radius.Core/applications@2025-08-01-preview",
			Name:         "applicationName",
			Properties: map[string]any{
				"environment": BicepExpr{Expression: "environment"},
			},
		},
		Comments: []BicepComment{
			{
				ResourceName: "docker-hub",
				ResourceType: "",
				Message:      "manifest error: This resource does not support generation in the manifest.",
			},
		},
	}

	got, err := Emit(bicepFile, "test.json")
	require.NoError(t, err)

	assert.Contains(t, got, "// Skipped: docker-hub")
	assert.Contains(t, got, "manifest error: This resource does not support generation in the manifest.")
	assert.NotContains(t, got, "// Unsupported:")
}

func Test_Emit_MixedErroredAndUnsupportedComments(t *testing.T) {
	t.Parallel()

	bicepFile := &BicepFile{
		Extensions: []string{"radius"},
		Parameters: []BicepParameter{
			{
				Name:        "environment",
				Type:        "string",
				Description: "Environment.",
			},
			{
				Name:         "applicationName",
				Type:         "string",
				Description:  "App name.",
				DefaultValue: "test",
			},
		},
		Application: BicepResource{
			SymbolicName: "app",
			TypeName:     "Radius.Core/applications@2025-08-01-preview",
			Name:         "applicationName",
			Properties: map[string]any{
				"environment": BicepExpr{Expression: "environment"},
			},
		},
		Comments: []BicepComment{
			{
				ResourceName: "cache-password-uri-encoded",
				ResourceType: "annotated.string",
				Message:      "manual configuration required",
			},
			{
				ResourceName: "docker-hub",
				ResourceType: "",
				Message:      "manifest error: This resource does not support generation in the manifest.",
			},
		},
	}

	got, err := Emit(bicepFile, "test.json")
	require.NoError(t, err)

	assert.Contains(t, got, "// Unsupported: cache-password-uri-encoded (annotated.string)")
	assert.Contains(t, got, "// Skipped: docker-hub")
}

func Test_Emit_SecureParameter(t *testing.T) {
	t.Parallel()

	bicepFile := &BicepFile{
		Extensions: []string{"radius"},
		Parameters: []BicepParameter{
			{
				Name:        "environment",
				Type:        "string",
				Description: "Environment.",
			},
			{
				Name:         "applicationName",
				Type:         "string",
				Description:  "App name.",
				DefaultValue: "test",
			},
			{
				Name:        "cachePassword",
				Type:        "string",
				Secure:      true,
				Description: "Password for the cache resource.",
			},
		},
		Application: BicepResource{
			SymbolicName: "app",
			TypeName:     "Radius.Core/applications@2025-08-01-preview",
			Name:         "applicationName",
			Properties: map[string]any{
				"environment": BicepExpr{Expression: "environment"},
			},
		},
	}

	got, err := Emit(bicepFile, "test.json")
	require.NoError(t, err)

	assert.Contains(t, got, "@secure()")
	assert.Contains(t, got, "param cachePassword string")
	assert.Contains(t, got, "@description('Password for the cache resource.')")
}

func Test_Emit_BuildWarningComment(t *testing.T) {
	t.Parallel()

	bicepFile := &BicepFile{
		Extensions: []string{"containers", "radius"},
		Parameters: []BicepParameter{
			{
				Name:        "environment",
				Type:        "string",
				Description: "Environment.",
			},
			{
				Name:         "applicationName",
				Type:         "string",
				Description:  "App name.",
				DefaultValue: "test",
			},
		},
		Application: BicepResource{
			SymbolicName: "app",
			TypeName:     "Radius.Core/applications@2025-08-01-preview",
			Name:         "applicationName",
			Properties: map[string]any{
				"environment": BicepExpr{Expression: "environment"},
			},
		},
		Containers: []BicepContainer{
			{
				SymbolicName:      "webapp",
				TypeName:          "Radius.Compute/containers@2025-08-01-preview",
				Name:              "webapp",
				Image:             "<YOUR_REGISTRY>/webapp:latest",
				ApplicationRef:    "app.id",
				EnvironmentRef:    "environment",
				NeedsBuildWarning: true,
				BuildContext:      "src/webapp",
			},
		},
	}

	got, err := Emit(bicepFile, "test.json")
	require.NoError(t, err)

	assert.Contains(t, got, "// WARNING: webapp (container.v1) has a build configuration")
	assert.Contains(t, got, "src/webapp")
}
