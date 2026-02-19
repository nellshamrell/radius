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

package aspire

import (
	"github.com/spf13/cobra"
)

// NewCommand creates a new cobra.Command for the `rad aspire` command group.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "aspire",
		Short: "Manage Aspire-related tasks for Radius",
		Long:  `Manage Aspire-related tasks for Radius. Use subcommands to convert Aspire manifests to Radius Bicep files.`,
		Example: `
# Convert an Aspire manifest to a Radius Bicep file
rad aspire convert aspire-manifest.json`,
	}

	return cmd
}
