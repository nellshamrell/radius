// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package graph

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20231001preview "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/radcli"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	application := corerpv20231001preview.ApplicationResource{
		Name: to.Ptr("test-app"),
		ID:   to.Ptr(applicationResourceID),
		Type: to.Ptr("Applications.Core/applications"),
		Properties: &corerpv20231001preview.ApplicationProperties{
			Environment: to.Ptr(environmentResourceID),
		},
	}

	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name:          "Graph command application (positional)",
			Input:         []string{"test-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetApplication(gomock.Any(), "test-app").
					Return(application, nil).
					Times(1)
			},
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				// These values are used by Run()
				require.Equal(t, "test-app", runner.ApplicationName)
			},
		},
		{
			Name:          "Graph command application (flag)",
			Input:         []string{"-a", "test-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetApplication(gomock.Any(), "test-app").
					Return(application, nil).
					Times(1)
			},
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				// These values are used by Run()
				require.Equal(t, "test-app", runner.ApplicationName)
			},
		},
		{
			Name:          "Graph command missing application",
			Input:         []string{"-a", "test-app"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetApplication(gomock.Any(), "test-app").
					Return(corerpv20231001preview.ApplicationResource{}, &azcore.ResponseError{ErrorCode: v1.CodeNotFound}).
					Times(1)
			},
		},
		{
			Name:          "Graph command with incorrect args",
			Input:         []string{"foo", "bar"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	// This example is a very simple example of the application graph as an integration test.
	// The unit tests for this package cover the more complex cases.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	graph := corerpv20231001preview.ApplicationGraphResponse{
		Resources: []*corerpv20231001preview.ApplicationGraphResource{
			{
				ID:                to.Ptr(containerResourceID),
				Name:              to.Ptr(containerResourceName),
				Type:              to.Ptr(containerResourceType),
				ProvisioningState: to.Ptr(provisioningStateSuccess),
				OutputResources: []*corerpv20231001preview.ApplicationGraphOutputResource{
					{
						ID:   to.Ptr("/planes/radius/local/resourcegroups/test-group/providers/kubernetes/Deployments/demo"),
						Type: to.Ptr("kubernetes: apps/Deployment"),
						Name: to.Ptr("demo"),
					},
				},
				Connections: []*corerpv20231001preview.ApplicationGraphConnection{
					{
						ID:        to.Ptr(redisResourceID),
						Direction: &directionOutbound,
					},
				},
			},
			{
				ID:                to.Ptr(redisResourceID),
				Name:              to.Ptr(redisResourceName),
				Type:              to.Ptr(redisResourceType),
				ProvisioningState: to.Ptr(provisioningStateSuccess),
				OutputResources: []*corerpv20231001preview.ApplicationGraphOutputResource{
					{
						ID:   to.Ptr("/planes/radius/local/resourcegroups/test-group/providers/AWS.MemoryDB/Cluster/redis-aqbjixghynqgg"),
						Type: to.Ptr("aws: AWS.MemoryDB/Cluster"),
						Name: to.Ptr("redis-aqbjixghynqgg"),
					},
				},
				Connections: []*corerpv20231001preview.ApplicationGraphConnection{
					{
						ID:        to.Ptr(containerResourceID),
						Direction: &directionInbound,
					},
				},
			},
		},
	}

	appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
	appManagementClient.EXPECT().
		GetApplicationGraph(gomock.Any(), "test-app").
		Return(graph, nil).
		Times(1)

	workspace := &workspaces.Workspace{
		Connection: map[string]any{
			"kind":    "kubernetes",
			"context": "kind-kind",
		},
		Name:  "kind-kind",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}
	outputSink := &output.MockOutput{}
	runner := &Runner{
		ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
		Workspace:         workspace,
		Output:            outputSink,

		// Populated by Validate()
		ApplicationName: "test-app",
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)

	expectedOutput := `Displaying application: test-app

Name: webapp (Applications.Core/containers)
Connections:
  webapp -> redis (Applications.Datastores/redisCaches)
Resources:
  demo (kubernetes: apps/Deployment)

Name: redis (Applications.Datastores/redisCaches)
Connections:
  webapp (Applications.Core/containers) -> redis
Resources:
  redis-aqbjixghynqgg (aws: AWS.MemoryDB/Cluster)

`

	expected := []any{
		output.LogOutput{
			Format: expectedOutput,
		},
	}

	require.Equal(t, expected, outputSink.Writes)
}

func Test_Run_JSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	graph := corerpv20231001preview.ApplicationGraphResponse{
		Resources: []*corerpv20231001preview.ApplicationGraphResource{
			{
				ID:                to.Ptr(containerResourceID),
				Name:              to.Ptr(containerResourceName),
				Type:              to.Ptr(containerResourceType),
				ProvisioningState: to.Ptr(provisioningStateSuccess),
				OutputResources: []*corerpv20231001preview.ApplicationGraphOutputResource{
					{
						ID:   to.Ptr("/planes/radius/local/resourcegroups/test-group/providers/kubernetes/Deployments/demo"),
						Type: to.Ptr("kubernetes: apps/Deployment"),
						Name: to.Ptr("demo"),
					},
				},
				Connections: []*corerpv20231001preview.ApplicationGraphConnection{
					{
						ID:        to.Ptr(redisResourceID),
						Direction: &directionOutbound,
					},
				},
			},
		},
	}

	appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
	appManagementClient.EXPECT().
		GetApplicationGraph(gomock.Any(), "test-app").
		Return(graph, nil).
		Times(1)

	workspace := &workspaces.Workspace{
		Connection: map[string]any{
			"kind":    "kubernetes",
			"context": "kind-kind",
		},
		Name:  "kind-kind",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}
	outputSink := &output.MockOutput{}
	runner := &Runner{
		ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
		Workspace:         workspace,
		Output:            outputSink,
		Format:            output.FormatJson,

		// Populated by Validate()
		ApplicationName: "test-app",
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)

	require.Len(t, outputSink.Writes, 1)
	formatted, ok := outputSink.Writes[0].(output.FormattedOutput)
	require.True(t, ok, "expected FormattedOutput but got %T", outputSink.Writes[0])
	require.Equal(t, output.FormatJson, formatted.Format)
	require.Equal(t, graph, formatted.Obj)
}

// loadFixtureTemplate loads a test fixture JSON file and returns it as a map.
func loadFixtureTemplate(t *testing.T, name string) map[string]any {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	var template map[string]any
	err = json.Unmarshal(data, &template)
	require.NoError(t, err)
	return template
}

func Test_Run_FileMode_SimpleApp(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	template := loadFixtureTemplate(t, "simple-app.json")

	mockBicep := bicep.NewMockInterface(ctrl)
	mockBicep.EXPECT().
		PrepareTemplate("testdata/simple-app.json").
		Return(template, nil).
		Times(1)

	outputSink := &output.MockOutput{}
	runner := &Runner{
		Output:      outputSink,
		BicepClient: mockBicep,
		FilePath:    "testdata/simple-app.json",
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)

	require.Len(t, outputSink.Writes, 1)
	logOutput, ok := outputSink.Writes[0].(output.LogOutput)
	require.True(t, ok, "expected LogOutput but got %T", outputSink.Writes[0])

	// The output should contain resource names and types
	require.Contains(t, logOutput.Format, "frontend")
	require.Contains(t, logOutput.Format, "backend")
	require.Contains(t, logOutput.Format, "Applications.Core/containers")
	require.Contains(t, logOutput.Format, "Displaying application: myapp")
}

func Test_Run_FileMode_JSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	template := loadFixtureTemplate(t, "simple-app.json")

	mockBicep := bicep.NewMockInterface(ctrl)
	mockBicep.EXPECT().
		PrepareTemplate("testdata/simple-app.json").
		Return(template, nil).
		Times(1)

	outputSink := &output.MockOutput{}
	runner := &Runner{
		Output:      outputSink,
		BicepClient: mockBicep,
		FilePath:    "testdata/simple-app.json",
		Format:      output.FormatJson,
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)

	require.Len(t, outputSink.Writes, 1)
	formatted, ok := outputSink.Writes[0].(output.FormattedOutput)
	require.True(t, ok, "expected FormattedOutput but got %T", outputSink.Writes[0])
	require.Equal(t, output.FormatJson, formatted.Format)

	// Verify the response is an ApplicationGraphResponse
	response, ok := formatted.Obj.(*corerpv20231001preview.ApplicationGraphResponse)
	require.True(t, ok, "expected *ApplicationGraphResponse but got %T", formatted.Obj)
	require.NotNil(t, response.Resources)

	// Verify provisioningState is NotDeployed and outputResources is empty
	for _, r := range response.Resources {
		require.Equal(t, "NotDeployed", to.String(r.ProvisioningState))
		require.Empty(t, r.OutputResources)
	}
}

func Test_Run_FileMode_EmptyTemplate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	template := loadFixtureTemplate(t, "empty.json")

	mockBicep := bicep.NewMockInterface(ctrl)
	mockBicep.EXPECT().
		PrepareTemplate("testdata/empty.json").
		Return(template, nil).
		Times(1)

	outputSink := &output.MockOutput{}
	runner := &Runner{
		Output:      outputSink,
		BicepClient: mockBicep,
		FilePath:    "testdata/empty.json",
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)

	// Empty template should produce output (empty graph message)
	require.Len(t, outputSink.Writes, 1)
}

func Test_Validate_FileMode(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name:          "File mode with valid file",
			Input:         []string{"--file", "testdata/simple-app.json"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				require.Equal(t, "testdata/simple-app.json", runner.FilePath)
			},
		},
		{
			Name:          "File mode with nonexistent file",
			Input:         []string{"--file", "testdata/nonexistent.json"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "File mode with mutual exclusivity error",
			Input:         []string{"test-app", "--file", "testdata/simple-app.json"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run_FileMode_NoApp(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	template := loadFixtureTemplate(t, "no-app.json")

	mockBicep := bicep.NewMockInterface(ctrl)
	mockBicep.EXPECT().
		PrepareTemplate("testdata/no-app.json").
		Return(template, nil).
		Times(1)

	outputSink := &output.MockOutput{}
	runner := &Runner{
		Output:      outputSink,
		BicepClient: mockBicep,
		FilePath:    "testdata/no-app.json",
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)

	// No-app template should still produce output with all resources
	require.Len(t, outputSink.Writes, 1)
	logOutput, ok := outputSink.Writes[0].(output.LogOutput)
	require.True(t, ok, "expected LogOutput but got %T", outputSink.Writes[0])
	require.Contains(t, logOutput.Format, "frontend")
	require.Contains(t, logOutput.Format, "backend")
}

func Test_Run_FileMode_MultiApp(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	template := loadFixtureTemplate(t, "multi-app.json")

	mockBicep := bicep.NewMockInterface(ctrl)
	mockBicep.EXPECT().
		PrepareTemplate("testdata/multi-app.json").
		Return(template, nil).
		Times(1)

	outputSink := &output.MockOutput{}
	runner := &Runner{
		Output:      outputSink,
		BicepClient: mockBicep,
		FilePath:    "testdata/multi-app.json",
	}

	err := runner.Run(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "multiple applications")
}

func Test_Run_FileMode_Unresolvable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	template := loadFixtureTemplate(t, "unresolvable.json")

	mockBicep := bicep.NewMockInterface(ctrl)
	mockBicep.EXPECT().
		PrepareTemplate("testdata/unresolvable.json").
		Return(template, nil).
		Times(1)

	outputSink := &output.MockOutput{}
	runner := &Runner{
		Output:      outputSink,
		BicepClient: mockBicep,
		FilePath:    "testdata/unresolvable.json",
	}

	// Capture stderr for warnings
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := runner.Run(context.Background())

	w.Close()
	os.Stderr = oldStderr

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	stderrOutput := string(buf[:n])

	require.NoError(t, err)

	// Should have produced output (partial graph)
	require.Len(t, outputSink.Writes, 1)

	// Stderr should contain warnings about unresolvable expressions
	require.True(t, strings.Contains(stderrOutput, "Warning:"), "expected warnings on stderr, got: %s", stderrOutput)
}

func Test_Run_FileMode_OfflineNoWorkspace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	template := loadFixtureTemplate(t, "simple-app.json")

	mockBicep := bicep.NewMockInterface(ctrl)
	mockBicep.EXPECT().
		PrepareTemplate("testdata/simple-app.json").
		Return(template, nil).
		Times(1)

	outputSink := &output.MockOutput{}
	runner := &Runner{
		Output:      outputSink,
		BicepClient: mockBicep,
		FilePath:    "testdata/simple-app.json",
		// No Workspace, no ConnectionFactory — proves offline operation
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)
	require.Len(t, outputSink.Writes, 1)
}

func Test_Run_FileMode_JSON_Schema(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	template := loadFixtureTemplate(t, "simple-app.json")

	mockBicep := bicep.NewMockInterface(ctrl)
	mockBicep.EXPECT().
		PrepareTemplate("testdata/simple-app.json").
		Return(template, nil).
		Times(1)

	outputSink := &output.MockOutput{}
	runner := &Runner{
		Output:      outputSink,
		BicepClient: mockBicep,
		FilePath:    "testdata/simple-app.json",
		Format:      output.FormatJson,
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)

	require.Len(t, outputSink.Writes, 1)
	formatted, ok := outputSink.Writes[0].(output.FormattedOutput)
	require.True(t, ok)

	response, ok := formatted.Obj.(*corerpv20231001preview.ApplicationGraphResponse)
	require.True(t, ok)

	// Verify JSON-serializable and conforming to ApplicationGraphResponse schema
	jsonBytes, err := json.Marshal(response)
	require.NoError(t, err)
	require.True(t, len(jsonBytes) > 0)

	// Verify all resources have NotDeployed state and empty outputResources
	for _, r := range response.Resources {
		require.Equal(t, "NotDeployed", to.String(r.ProvisioningState))
		require.Empty(t, r.OutputResources)
	}
}

func Test_Run_FileMode_Dot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	template := loadFixtureTemplate(t, "simple-app.json")

	mockBicep := bicep.NewMockInterface(ctrl)
	mockBicep.EXPECT().
		PrepareTemplate("testdata/simple-app.json").
		Return(template, nil).
		Times(1)

	outputSink := &output.MockOutput{}
	runner := &Runner{
		Output:      outputSink,
		BicepClient: mockBicep,
		FilePath:    "testdata/simple-app.json",
		Format:      output.FormatDot,
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)

	require.Len(t, outputSink.Writes, 1)
	logOutput, ok := outputSink.Writes[0].(output.LogOutput)
	require.True(t, ok, "expected LogOutput but got %T", outputSink.Writes[0])

	// Verify valid DOT digraph structure
	require.Contains(t, logOutput.Format, "digraph")
	require.Contains(t, logOutput.Format, "->")
	require.Contains(t, logOutput.Format, "rankdir=LR")

	// Verify resource names and types appear
	require.Contains(t, logOutput.Format, "frontend")
	require.Contains(t, logOutput.Format, "backend")
	require.Contains(t, logOutput.Format, "Applications.Core/containers")
}

func Test_Run_Dot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	graph := corerpv20231001preview.ApplicationGraphResponse{
		Resources: []*corerpv20231001preview.ApplicationGraphResource{
			{
				ID:                to.Ptr(containerResourceID),
				Name:              to.Ptr(containerResourceName),
				Type:              to.Ptr(containerResourceType),
				ProvisioningState: to.Ptr(provisioningStateSuccess),
				OutputResources:   []*corerpv20231001preview.ApplicationGraphOutputResource{},
				Connections: []*corerpv20231001preview.ApplicationGraphConnection{
					{
						ID:        to.Ptr(redisResourceID),
						Direction: &directionOutbound,
					},
				},
			},
			{
				ID:                to.Ptr(redisResourceID),
				Name:              to.Ptr(redisResourceName),
				Type:              to.Ptr(redisResourceType),
				ProvisioningState: to.Ptr(provisioningStateSuccess),
				OutputResources:   []*corerpv20231001preview.ApplicationGraphOutputResource{},
				Connections: []*corerpv20231001preview.ApplicationGraphConnection{
					{
						ID:        to.Ptr(containerResourceID),
						Direction: &directionInbound,
					},
				},
			},
		},
	}

	appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
	appManagementClient.EXPECT().
		GetApplicationGraph(gomock.Any(), "test-app").
		Return(graph, nil).
		Times(1)

	workspace := &workspaces.Workspace{
		Connection: map[string]any{
			"kind":    "kubernetes",
			"context": "kind-kind",
		},
		Name:  "kind-kind",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}
	outputSink := &output.MockOutput{}
	runner := &Runner{
		ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
		Workspace:         workspace,
		Output:            outputSink,
		Format:            output.FormatDot,

		// Populated by Validate()
		ApplicationName: "test-app",
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)

	require.Len(t, outputSink.Writes, 1)
	logOutput, ok := outputSink.Writes[0].(output.LogOutput)
	require.True(t, ok, "expected LogOutput but got %T", outputSink.Writes[0])

	// Verify valid DOT digraph structure
	require.Contains(t, logOutput.Format, `digraph "test-app"`)
	require.Contains(t, logOutput.Format, containerResourceName)
	require.Contains(t, logOutput.Format, redisResourceName)
	require.Contains(t, logOutput.Format, "->")
	require.Contains(t, logOutput.Format, "rankdir=LR")
}
