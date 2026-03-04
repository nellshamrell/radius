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

package graph

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func Test_stripAPIVersion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "type with API version",
			input:    "Applications.Core/containers@2023-10-01-preview",
			expected: "Applications.Core/containers",
		},
		{
			name:     "type without API version",
			input:    "Applications.Core/containers",
			expected: "Applications.Core/containers",
		},
		{
			name:     "non-Radius type with API version",
			input:    "Microsoft.Storage/storageAccounts@2021-02-01",
			expected: "Microsoft.Storage/storageAccounts",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := stripAPIVersion(tc.input)
			require.Equal(t, tc.expected, result)
		})
	}
}

func Test_synthesizeResourceID(t *testing.T) {
	result := synthesizeResourceID("Applications.Core/containers", "frontend")
	require.Equal(t, "/planes/radius/local/resourceGroups/default/providers/Applications.Core/containers/frontend", result)
}

func Test_isRadiusResource(t *testing.T) {
	tests := []struct {
		name         string
		importField  string
		resourceType string
		expected     bool
	}{
		{
			name:         "Radius import",
			importField:  "Radius",
			resourceType: "Applications.Core/containers",
			expected:     true,
		},
		{
			name:         "Radius import case-insensitive",
			importField:  "radius",
			resourceType: "anything",
			expected:     true,
		},
		{
			name:         "Applications prefix without import",
			importField:  "",
			resourceType: "Applications.Core/containers",
			expected:     true,
		},
		{
			name:         "Radius namespace prefix",
			importField:  "",
			resourceType: "Radius.Core/applications",
			expected:     true,
		},
		{
			name:         "non-Radius resource",
			importField:  "",
			resourceType: "Microsoft.Storage/storageAccounts",
			expected:     false,
		},
		{
			name:         "non-Radius with az import",
			importField:  "az",
			resourceType: "Microsoft.Storage/storageAccounts",
			expected:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isRadiusResource(tc.importField, tc.resourceType)
			require.Equal(t, tc.expected, result)
		})
	}
}

func Test_resolveExpression(t *testing.T) {
	resources := map[string]resourceEntry{
		"app": {
			SymbolicName:  "app",
			Type:          "Applications.Core/applications",
			Name:          "myapp",
			SynthesizedID: "/planes/radius/local/resourceGroups/default/providers/Applications.Core/applications/myapp",
		},
		"backend": {
			SymbolicName:  "backend",
			Type:          "Applications.Core/containers",
			Name:          "backend",
			SynthesizedID: "/planes/radius/local/resourceGroups/default/providers/Applications.Core/containers/backend",
		},
	}

	tests := []struct {
		name       string
		value      string
		expectedID string
		expectedOk bool
	}{
		{
			name:       "literal pass-through",
			value:      "http://backend:3000",
			expectedID: "http://backend:3000",
			expectedOk: true,
		},
		{
			name:       "literal resource ID",
			value:      "/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default",
			expectedID: "/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default",
			expectedOk: true,
		},
		{
			name:       "reference expression found",
			value:      "[reference('app').id]",
			expectedID: "/planes/radius/local/resourceGroups/default/providers/Applications.Core/applications/myapp",
			expectedOk: true,
		},
		{
			name:       "reference expression not found",
			value:      "[reference('nonexistent').id]",
			expectedID: "",
			expectedOk: false,
		},
		{
			name:       "parameters expression unsupported",
			value:      "[parameters('location')]",
			expectedID: "",
			expectedOk: false,
		},
		{
			name:       "format expression unsupported",
			value:      "[format('{0}-env', parameters('name'))]",
			expectedID: "",
			expectedOk: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolved, ok := resolveExpression(tc.value, resources)
			require.Equal(t, tc.expectedOk, ok)
			require.Equal(t, tc.expectedID, resolved)
		})
	}
}

func loadTestFixture(t *testing.T, name string) map[string]any {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	var template map[string]any
	err = json.Unmarshal(data, &template)
	require.NoError(t, err)
	return template
}

func Test_extractResourcesFromTemplate_simpleApp(t *testing.T) {
	template := loadTestFixture(t, "simple-app.json")

	resources, warnings, err := extractResourcesFromTemplate(template)
	require.NoError(t, err)
	require.Empty(t, warnings)

	// simple-app.json has 3 resources: app, frontend, backend
	require.Len(t, resources, 3)

	// Verify sorted order (by type, then name)
	// Applications.Core/applications/myapp, Applications.Core/containers/backend, Applications.Core/containers/frontend
	require.Equal(t, "Applications.Core/applications", to.String(resources[0].Type))
	require.Equal(t, "myapp", to.String(resources[0].Name))

	require.Equal(t, "Applications.Core/containers", to.String(resources[1].Type))
	require.Equal(t, "backend", to.String(resources[1].Name))

	require.Equal(t, "Applications.Core/containers", to.String(resources[2].Type))
	require.Equal(t, "frontend", to.String(resources[2].Name))

	// Verify synthesized IDs
	require.Equal(t, "/planes/radius/local/resourceGroups/default/providers/Applications.Core/applications/myapp", to.String(resources[0].ID))
	require.Equal(t, "/planes/radius/local/resourceGroups/default/providers/Applications.Core/containers/backend", to.String(resources[1].ID))
	require.Equal(t, "/planes/radius/local/resourceGroups/default/providers/Applications.Core/containers/frontend", to.String(resources[2].ID))

	// Verify provisioning state
	for _, r := range resources {
		require.Equal(t, "NotDeployed", r.Properties["provisioningState"])
		status, ok := r.Properties["status"].(map[string]any)
		require.True(t, ok)
		outputResources, ok := status["outputResources"].([]any)
		require.True(t, ok)
		require.Empty(t, outputResources)
	}

	// Verify resolved connections on frontend
	frontendConns, ok := resources[2].Properties["connections"].(map[string]any)
	require.True(t, ok)
	backendConn, ok := frontendConns["backend"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "/planes/radius/local/resourceGroups/default/providers/Applications.Core/containers/backend", backendConn["source"])

	// Verify resolved application reference on frontend
	require.Equal(t,
		"/planes/radius/local/resourceGroups/default/providers/Applications.Core/applications/myapp",
		resources[2].Properties["application"])
}

func Test_extractResourcesFromTemplate_emptyResources(t *testing.T) {
	template := loadTestFixture(t, "empty.json")

	resources, warnings, err := extractResourcesFromTemplate(template)
	require.NoError(t, err)
	require.Empty(t, warnings)
	require.Empty(t, resources)
	require.NotNil(t, resources) // should be empty slice, not nil
}

func Test_extractResourcesFromTemplate_nonRadiusResources(t *testing.T) {
	template := loadTestFixture(t, "non-radius.json")

	resources, warnings, err := extractResourcesFromTemplate(template)
	require.NoError(t, err)
	require.Empty(t, warnings)

	// non-radius.json has 3 resources: 2 Radius + 1 Azure
	require.Len(t, resources, 3)

	// Find the non-Radius resource
	var nonRadius *struct {
		name       string
		resType    string
		properties map[string]any
	}
	for _, r := range resources {
		if !isRadiusResource("", to.String(r.Type)) {
			nonRadius = &struct {
				name       string
				resType    string
				properties map[string]any
			}{
				name:       to.String(r.Name),
				resType:    to.String(r.Type),
				properties: r.Properties,
			}
			break
		}
	}
	require.NotNil(t, nonRadius)
	require.Equal(t, "NotDeployed", nonRadius.properties["provisioningState"])
	// Non-Radius resources should NOT have application or connections
	_, hasApp := nonRadius.properties["application"]
	require.False(t, hasApp)
}

func Test_extractResourcesFromTemplate_missingResourcesKey(t *testing.T) {
	template := map[string]any{
		"$schema": "...",
	}

	_, _, err := extractResourcesFromTemplate(template)
	require.Error(t, err)
	require.Contains(t, err.Error(), "resources")
}

func Test_extractResourcesFromTemplate_withModules(t *testing.T) {
	template := loadTestFixture(t, "with-modules.json")

	resources, warnings, err := extractResourcesFromTemplate(template)
	require.NoError(t, err)

	// Should warn about module reference
	hasModuleWarning := false
	for _, w := range warnings {
		if contains(w, "module reference") && contains(w, "shared-infra") {
			hasModuleWarning = true
			break
		}
	}
	require.True(t, hasModuleWarning, "expected module warning, got: %v", warnings)

	// Module resource should still be included in the output
	require.Len(t, resources, 3)
}

func Test_extractResourcesFromTemplate_unresolvable(t *testing.T) {
	template := loadTestFixture(t, "unresolvable.json")

	resources, warnings, err := extractResourcesFromTemplate(template)
	require.NoError(t, err)

	// Should have warnings for unresolvable expressions
	require.NotEmpty(t, warnings)

	// Should still extract the resources that are resolvable
	require.NotEmpty(t, resources)

	// Check that some connections were skipped (warnings about parameter/format expressions)
	hasParamWarning := false
	hasFormatWarning := false
	for _, w := range warnings {
		if contains(w, "parameters(") {
			hasParamWarning = true
		}
		if contains(w, "format(") {
			hasFormatWarning = true
		}
	}
	require.True(t, hasParamWarning, "expected parameter expression warning, got: %v", warnings)
	require.True(t, hasFormatWarning, "expected format expression warning, got: %v", warnings)

	// The resolvable connection (frontend->backend via reference) should still be present
	for _, r := range resources {
		if to.String(r.Name) == "frontend" {
			conns, ok := r.Properties["connections"].(map[string]any)
			require.True(t, ok, "frontend should have connections")
			_, hasBackend := conns["backend"]
			require.True(t, hasBackend, "frontend should have resolvable backend connection")
		}
	}
}

func Test_extractResourcesFromTemplate_conditionalResource(t *testing.T) {
	// Create a template with a conditional resource inline
	template := map[string]any{
		"resources": map[string]any{
			"conditionalCache": map[string]any{
				"import":    "Radius",
				"type":      "Applications.Core/extenders@2023-10-01-preview",
				"condition": "[parameters('enableCache')]",
				"properties": map[string]any{
					"name":       "my-cache",
					"properties": map[string]any{},
				},
			},
		},
	}

	resources, warnings, err := extractResourcesFromTemplate(template)
	require.NoError(t, err)

	// Resource should be included
	require.Len(t, resources, 1)
	require.Equal(t, "my-cache", to.String(resources[0].Name))

	// But should have a warning about the condition
	hasConditionWarning := false
	for _, w := range warnings {
		if contains(w, "condition") && contains(w, "my-cache") {
			hasConditionWarning = true
			break
		}
	}
	require.True(t, hasConditionWarning, "expected condition warning, got: %v", warnings)
}

func Test_scopeToApplication_noApps(t *testing.T) {
	template := loadTestFixture(t, "no-app.json")

	resources, _, err := extractResourcesFromTemplate(template)
	require.NoError(t, err)

	appName, filtered, err := scopeToApplication(resources)
	require.NoError(t, err)
	require.Equal(t, "", appName)
	require.Equal(t, resources, filtered) // all resources returned
}

func Test_scopeToApplication_oneApp(t *testing.T) {
	template := loadTestFixture(t, "simple-app.json")

	resources, _, err := extractResourcesFromTemplate(template)
	require.NoError(t, err)

	appName, filtered, err := scopeToApplication(resources)
	require.NoError(t, err)
	require.Equal(t, "myapp", appName)

	// Should include app + resources referencing that app (frontend, backend)
	require.Len(t, filtered, 3)
}

func Test_scopeToApplication_multipleApps(t *testing.T) {
	template := loadTestFixture(t, "multi-app.json")

	resources, _, err := extractResourcesFromTemplate(template)
	require.NoError(t, err)

	_, _, err = scopeToApplication(resources)
	require.Error(t, err)
	require.Contains(t, err.Error(), "multiple applications")
}

// contains is a test helper for substring matching.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
