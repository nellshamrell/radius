// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package graph

import (
	"context"
	"fmt"
	"os"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/corerp/frontend/controller/applications"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad app graph` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Shows the application graph for an application.",
		Long:  `Shows the application graph for an application.`,
		Args:  cobra.MaximumNArgs(1),
		Example: `
# Show graph for current application
rad app graph

# Show graph for specified application
rad app graph my-application

# Show graph from a Bicep file (offline, no Radius environment required)
rad app graph --file app.bicep

# Show graph from a Bicep file in JSON format
rad app graph --file app.bicep --output json

# Show graph from a Bicep file in Graphviz DOT format
rad app graph --file app.bicep --output dot`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddApplicationNameFlag(cmd)
	commonflags.AddOutputFlag(cmd)
	cmd.Flags().StringP("file", "f", "", "Path to a .bicep or .json file for offline graph generation")

	return cmd, runner
}

// Runner is the runner implementation for the `rad app graph` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	BicepClient       bicep.Interface

	ApplicationName string
	Format          string
	Workspace       *workspaces.Workspace
	FilePath        string
}

// NewRunner creates a new instance of the `rad app graph` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
		ConnectionFactory: factory.GetConnectionFactory(),
		BicepClient:       factory.GetBicep(),
	}
}

// Validate runs validation for the `rad app graph` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	r.FilePath, _ = cmd.Flags().GetString("file")

	if r.FilePath != "" && len(args) > 0 {
		return clierrors.Message("--file and application name are mutually exclusive")
	}

	if r.FilePath != "" {
		// File mode: validate file exists, read output format, skip workspace/scope/application validation
		if _, err := os.Stat(r.FilePath); err != nil {
			return clierrors.Message("file not found: %s", r.FilePath)
		}

		format, err := cli.RequireOutput(cmd)
		if err != nil {
			return err
		}
		r.Format = format

		return nil
	}

	// Live mode: existing validation
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	r.Workspace.Scope, err = cli.RequireScope(cmd, *r.Workspace)
	if err != nil {
		return err
	}

	r.ApplicationName, err = cli.RequireApplicationArgs(cmd, args, *r.Workspace)
	if err != nil {
		return err
	}

	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(cmd.Context(), *r.Workspace)
	if err != nil {
		return err
	}

	// Validate that the application exists
	_, err = client.GetApplication(cmd.Context(), r.ApplicationName)
	if clients.Is404Error(err) {
		return clierrors.Message("Application %q does not exist or has been deleted.", r.ApplicationName)
	} else if err != nil {
		return err
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	return nil
}

// Run runs the `rad app graph` command.
func (r *Runner) Run(ctx context.Context) error {
	if r.FilePath != "" {
		return r.runFileMode(ctx)
	}

	return r.runLiveMode(ctx)
}

// runFileMode executes the graph command in file mode, compiling a Bicep/JSON file and
// producing the application graph without requiring a Radius control plane.
func (r *Runner) runFileMode(ctx context.Context) error {
	template, err := r.BicepClient.PrepareTemplate(r.FilePath)
	if err != nil {
		return err
	}

	resources, warnings, err := extractResourcesFromTemplate(template)
	if err != nil {
		return err
	}

	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "Warning:", w)
	}

	appName, appResources, err := scopeToApplication(resources)
	if err != nil {
		return err
	}

	response := applications.ComputeGraph(appResources, nil)

	switch r.Format {
	case output.FormatJson:
		return r.Output.WriteFormatted(r.Format, response, output.FormatterOptions{})
	case output.FormatDot:
		d := displayDot(response.Resources, appName)
		r.Output.LogInfo(d)
		return nil
	default:
		d := display(response.Resources, appName)
		r.Output.LogInfo(d)
		return nil
	}
}

// runLiveMode executes the graph command in live mode, querying the Radius control plane.
func (r *Runner) runLiveMode(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	applicationGraphResponse, err := client.GetApplicationGraph(ctx, r.ApplicationName)
	if err != nil {
		return err
	}

	switch r.Format {
	case output.FormatJson:
		return r.Output.WriteFormatted(r.Format, applicationGraphResponse, output.FormatterOptions{})
	case output.FormatDot:
		d := displayDot(applicationGraphResponse.Resources, r.ApplicationName)
		r.Output.LogInfo(d)
		return nil
	default:
		graph := applicationGraphResponse.Resources
		d := display(graph, r.ApplicationName)
		r.Output.LogInfo(d)

		return nil
	}
}
