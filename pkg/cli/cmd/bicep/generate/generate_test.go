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
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/test/radcli"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	testcases := []radcli.ValidateInput{
		{
			Name:          "rad bicep generate - valid with from-aspire flag",
			Input:         []string{"--from-aspire", "./testdata/aspire-starter"},
			ExpectedValid: true,
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				require.Equal(t, "./testdata/aspire-starter", runner.FromAspirePath)
				require.Equal(t, "./app.bicep", runner.OutputPath)
				require.Equal(t, "./mapping-report.md", runner.ReportPath)
				require.False(t, runner.Quiet)
				require.False(t, runner.Deterministic)
			},
		},
		{
			Name:          "rad bicep generate - valid with all flags",
			Input:         []string{"--from-aspire", "./testdata/aspire-starter", "--output", "./custom.bicep", "--app-name", "my-app", "--report", "./custom-report.md", "--quiet", "--deterministic"},
			ExpectedValid: true,
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				require.Equal(t, "./testdata/aspire-starter", runner.FromAspirePath)
				require.Equal(t, "./custom.bicep", runner.OutputPath)
				require.Equal(t, "my-app", runner.AppName)
				require.Equal(t, "./custom-report.md", runner.ReportPath)
				require.True(t, runner.Quiet)
				require.True(t, runner.Deterministic)
			},
		},
		{
			Name:          "rad bicep generate - invalid without from-aspire",
			Input:         []string{},
			ExpectedValid: false,
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run_AspireStarter(t *testing.T) {
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "app.bicep")

	runner := &Runner{
		Output:         &output.MockOutput{},
		FromAspirePath: "./testdata/aspire-starter",
		OutputPath:     outputPath,
		AppName:        "",
		ReportPath:     filepath.Join(outputDir, "mapping-report.md"),
		Quiet:          false,
		Deterministic:  true,
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)

	// Verify output file was created
	_, err = os.Stat(outputPath)
	require.NoError(t, err, "app.bicep should be created")

	// Verify output content
	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	output := string(content)
	assert.Contains(t, output, "extension radius")
	assert.Contains(t, output, "Radius.Core/applications@2025-08-01-preview")
	assert.Contains(t, output, "resource apiservice")
	assert.Contains(t, output, "resource webfrontend")
	assert.Contains(t, output, "resource cache")
	assert.Contains(t, output, "DETERMINISTIC", "deterministic flag should set fixed timestamp")
}

func Test_Run_CustomAppName(t *testing.T) {
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "app.bicep")

	runner := &Runner{
		Output:         &output.MockOutput{},
		FromAspirePath: "./testdata/aspire-starter",
		OutputPath:     outputPath,
		AppName:        "custom-name",
		ReportPath:     filepath.Join(outputDir, "mapping-report.md"),
		Quiet:          false,
		Deterministic:  true,
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	assert.Contains(t, string(content), "param applicationName string = 'custom-name'")
}

func Test_Run_InvalidPath(t *testing.T) {
	runner := &Runner{
		Output:         &output.MockOutput{},
		FromAspirePath: "./testdata/nonexistent",
		OutputPath:     "/tmp/test-output.bicep",
		ReportPath:     "/tmp/test-report.md",
	}

	err := runner.Run(context.Background())
	require.Error(t, err, "should fail with nonexistent directory")
}
