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
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_GenerateAppBicep_GoldenFile(t *testing.T) {
	// Parse → Map → Generate pipeline using example-aspire-app fixtures
	descriptor, err := ScanDirectory("./testdata/example-aspire-app")
	require.NoError(t, err)

	app, err := MapToRadius(descriptor, "")
	require.NoError(t, err)

	output, err := GenerateAppBicep(app, "./testdata/example-aspire-app", "DETERMINISTIC")
	require.NoError(t, err)

	// Compare with golden file
	goldenBytes, err := os.ReadFile("./testdata/golden/app.bicep")
	require.NoError(t, err)

	expected := string(goldenBytes)
	assert.Equal(t, expected, output, "generated app.bicep should match golden file")
}

func Test_GenerateAppBicep_ValidStructure(t *testing.T) {
	app := RadiusApplication{
		Name: "test-app",
		Containers: []RadiusContainer{
			{
				Name:         "web",
				ImageParam:   "webImage",
				ImageDefault: "IMAGE_PLACEHOLDER",
				Ports: []RadiusPort{
					{Name: "http", ContainerPort: 8080, Protocol: "TCP"},
				},
			},
		},
		Dependencies: []RadiusDependency{
			{
				Name:           "redis",
				Type:           "Applications.Datastores/redisCaches",
				IsRecipeBacked: true,
			},
			{
				Name:           "sqlserver",
				Type:           "Applications.Datastores/sqlDatabases",
				IsRecipeBacked: true,
			},
		},
		Parameters: []RadiusParameter{
			{
				Name:         "webImage",
				Type:         "string",
				DefaultValue: "IMAGE_PLACEHOLDER",
				Description:  "Container image for web.",
			},
		},
	}

	output, err := GenerateAppBicep(app, "./infra", "2026-01-01T00:00:00Z")
	require.NoError(t, err)

	// Verify structural elements
	assert.Contains(t, output, "extension radius", "should contain extension radius")
	assert.Contains(t, output, "Applications.Core/applications@2023-10-01-preview", "should contain application resource type")
	assert.Contains(t, output, "Applications.Core/containers@2023-10-01-preview", "should contain container resource type")
	assert.Contains(t, output, "Applications.Datastores/redisCaches@2023-10-01-preview", "should contain Redis resource type")
	assert.Contains(t, output, "Applications.Datastores/sqlDatabases@2023-10-01-preview", "should contain SQL Database resource type")
	assert.Contains(t, output, "param environment string", "should contain environment param")
	assert.Contains(t, output, "param applicationName string = 'test-app'", "should contain applicationName param")
	assert.Contains(t, output, "containerPort: 8080", "should contain port")
	assert.Contains(t, output, "image: webImage", "should reference image param")
}

func Test_GenerateAppBicep_Idempotent(t *testing.T) {
	app := RadiusApplication{
		Name: "idempotent-app",
		Containers: []RadiusContainer{
			{
				Name:         "svc",
				ImageParam:   "svcImage",
				ImageDefault: "IMAGE_PLACEHOLDER",
				Ports: []RadiusPort{
					{Name: "http", ContainerPort: 3000, Protocol: "TCP"},
				},
			},
		},
		Parameters: []RadiusParameter{
			{Name: "svcImage", Type: "string", DefaultValue: "IMAGE_PLACEHOLDER", Description: "Container image for svc."},
		},
	}

	output1, err := GenerateAppBicep(app, "./infra", "DETERMINISTIC")
	require.NoError(t, err)

	output2, err := GenerateAppBicep(app, "./infra", "DETERMINISTIC")
	require.NoError(t, err)

	assert.Equal(t, output1, output2, "successive calls with same input should produce identical output")
}

func Test_GenerateAppBicep_PlaceholderDependency(t *testing.T) {
	app := RadiusApplication{
		Name: "placeholder-app",
		Dependencies: []RadiusDependency{
			{
				Name:               "unsupported",
				IsPlaceholder:      true,
				PlaceholderComment: "PLACEHOLDER: Azure resource type has no Portable Resource equivalent — manual configuration required",
			},
		},
	}

	output, err := GenerateAppBicep(app, "./infra", "DETERMINISTIC")
	require.NoError(t, err)

	assert.Contains(t, output, "PLACEHOLDER", "should contain placeholder comment")
	assert.Contains(t, output, "no Portable Resource equivalent", "should explain the placeholder")
}

func Test_sortedEnvVars(t *testing.T) {
	envVars := map[string]RadiusEnvVar{
		"ZEBRA":  {Value: "z"},
		"ALPHA":  {Value: "a"},
		"MIDDLE": {Value: "m"},
	}

	sorted := sortedEnvVars(envVars)
	require.Len(t, sorted, 3)
	assert.Equal(t, "ALPHA", sorted[0].Key)
	assert.Equal(t, "a", sorted[0].Value.Value)
	assert.Equal(t, "MIDDLE", sorted[1].Key)
	assert.Equal(t, "m", sorted[1].Value.Value)
	assert.Equal(t, "ZEBRA", sorted[2].Key)
	assert.Equal(t, "z", sorted[2].Value.Value)
}
