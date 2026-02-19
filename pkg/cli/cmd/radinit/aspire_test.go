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

package radinit

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_isAspireMode(t *testing.T) {
	t.Parallel()

	t.Run("enabled", func(t *testing.T) {
		t.Parallel()

		r := &Runner{AspireManifestPath: "/path/to/manifest.json"}
		assert.True(t, r.isAspireMode())
	})

	t.Run("disabled", func(t *testing.T) {
		t.Parallel()

		r := &Runner{}
		assert.False(t, r.isAspireMode())
	})
}

func Test_aspireImageMappings(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		r := &Runner{}
		assert.Nil(t, r.aspireImageMappings())
	})

	t.Run("with mappings", func(t *testing.T) {
		t.Parallel()

		r := &Runner{
			AspireImageMappings: []string{
				"api=myregistry.io/api:v1",
				"worker=myregistry.io/worker:v1",
			},
		}

		result := r.aspireImageMappings()
		assert.Equal(t, "myregistry.io/api:v1", result["api"])
		assert.Equal(t, "myregistry.io/worker:v1", result["worker"])
	})
}

func Test_aspireResourceOverrides(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		r := &Runner{}
		assert.Nil(t, r.aspireResourceOverrides())
	})

	t.Run("with overrides", func(t *testing.T) {
		t.Parallel()

		r := &Runner{
			AspireResourceOverrides: []string{
				"cache=Applications.Core/containers",
			},
		}

		result := r.aspireResourceOverrides()
		assert.Equal(t, "Applications.Core/containers", string(result["cache"]))
	})
}

func Test_runAspireTranslation(t *testing.T) {
	t.Parallel()

	// Create a temp directory with a simple manifest.
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.json")
	content := `{
		"resources": {
			"api": {
				"type": "container.v0",
				"image": "myapp/api:latest",
				"bindings": {
					"http": {
						"scheme": "http",
						"protocol": "tcp",
						"port": 8080,
						"targetPort": 8080
					}
				}
			}
		}
	}`
	require.NoError(t, os.WriteFile(manifestPath, []byte(content), 0644))

	outputDir := filepath.Join(tmpDir, "output")

	r := &Runner{
		Output:             &output.MockOutput{},
		AspireManifestPath: manifestPath,
		AspireAppName:      "testapp",
		AspireEnvironment:  "testenv",
		AspireOutputDir:    outputDir,
	}

	err := r.runAspireTranslation(context.Background())
	require.NoError(t, err)

	// Verify app.bicep was created.
	bicepPath := filepath.Join(outputDir, "app.bicep")
	data, err := os.ReadFile(bicepPath)
	require.NoError(t, err)

	bicep := string(data)
	assert.Contains(t, bicep, "extension radius")
	assert.Contains(t, bicep, "Applications.Core/containers@2023-10-01-preview")
	assert.Contains(t, bicep, "param application string = 'testapp'")
	assert.Contains(t, bicep, "param environment string = 'testenv'")
}

func Test_runAspireTranslation_Error(t *testing.T) {
	t.Parallel()

	r := &Runner{
		Output:             &output.MockOutput{},
		AspireManifestPath: "/nonexistent/manifest.json",
		AspireOutputDir:    t.TempDir(),
	}

	err := r.runAspireTranslation(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manifest file not found")
}
