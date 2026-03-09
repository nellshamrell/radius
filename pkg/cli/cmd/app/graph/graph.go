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
rad app graph --file app.bicep --output dot

# Show graph from an Aspire AppHost project (offline, no Radius environment or .NET SDK required)
rad app graph --from-aspire ./AspireApp.AppHost

# Show graph from an Aspire AppHost .csproj file
rad app graph --from-aspire ./AspireApp.AppHost/AspireApp.AppHost.csproj

# Show graph from an Aspire AppHost in JSON format
rad app graph --from-aspire ./AspireApp.AppHost --output json

# Show graph from an Aspire AppHost in Graphviz DOT format
rad app graph --from-aspire ./AspireApp.AppHost --output dot`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddApplicationNameFlag(cmd)
	commonflags.AddOutputFlag(cmd)
	cmd.Flags().StringP("file", "f", "", "Path to a .bicep or .json file for offline graph generation")
	cmd.Flags().String("from-aspire", "", "Path to a .NET Aspire AppHost project directory or .csproj file for offline graph generation")

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
	AspirePath      string
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
	r.AspirePath, _ = cmd.Flags().GetString("from-aspire")

	// Three-way mutual exclusivity check: --from-aspire, --file, and positional app name
	modeCount := 0
	if r.FilePath != "" {
		modeCount++
	}
	if r.AspirePath != "" {
		modeCount++
	}
	if len(args) > 0 {
		modeCount++
	}
	if modeCount > 1 {
		return clierrors.Message("--from-aspire, --file, and application name are mutually exclusive")
	}

	if r.AspirePath != "" {
		// Aspire mode: validate path exists, read output format, skip workspace/scope/app validation
		if _, err := os.Stat(r.AspirePath); err != nil {
			return clierrors.Message("path not found: %s", r.AspirePath)
		}

		format, err := cli.RequireOutput(cmd)
		if err != nil {
			return err
		}
		r.Format = format

		return nil
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
	if r.AspirePath != "" {
		return r.runAspireMode(ctx)
	}

	if r.FilePath != "" {
		return r.runFileMode(ctx)
	}

	return r.runLiveMode(ctx)
}

// runAspireMode executes the graph command in Aspire mode, parsing a .NET Aspire AppHost
// project's C# source to extract the application topology and produce the graph without
// requiring a Radius environment or .NET SDK.
func (r *Runner) runAspireMode(ctx context.Context) error {
	projectDir, csprojPath, err := discoverAppHostProject(r.AspirePath)
	if err != nil {
		return clierrors.Message("%s", err.Error())
	}

	entryPointFile, err := findEntryPointFile(projectDir)
	if err != nil {
		return clierrors.Message("%s", err.Error())
	}

	content, err := os.ReadFile(entryPointFile)
	if err != nil {
		return fmt.Errorf("failed to read entry point file: %w", err)
	}

	resources, connections, warnings, err := parseAspireAppHost(string(content))
	if err != nil {
		return err
	}

	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, w)
	}

	genericResources, err := aspireResourcesToGenericResources(resources, connections)
	if err != nil {
		return err
	}

	appName := deriveApplicationName(r.AspirePath, projectDir, csprojPath)

	response := applications.ComputeGraph(genericResources, nil)

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
