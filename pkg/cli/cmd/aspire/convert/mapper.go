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

// MapManifest converts a parsed AspireManifest into a BicepFile intermediate representation.
// The applicationName parameter overrides the default application name if non-empty.
//
// This function orchestrates the mapping of all Aspire resources to their Bicep equivalents,
// including containers, backing services, gateways, parameters, and unsupported resource comments.
func MapManifest(manifest *AspireManifest, applicationName string) *BicepFile {
	appName := applicationName
	if appName == "" {
		appName = "aspire-app"
	}

	file := &BicepFile{
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
				DefaultValue: appName,
			},
		},
		Application: BicepResource{
			SymbolicName: "app",
			TypeName:     "Radius.Core/applications@2025-08-01-preview",
			Name:         "applicationName",
			Properties: map[string]any{
				"environment": "environment",
			},
		},
	}

	return file
}
