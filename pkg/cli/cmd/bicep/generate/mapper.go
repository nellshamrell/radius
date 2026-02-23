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
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// dependencyTypeMap maps Azure resource types to Radius Portable Resource types.
var dependencyTypeMap = map[string]string{
	"Microsoft.Cache/redis":                        "Applications.Datastores/redisCaches",
	"Microsoft.DocumentDB/databaseAccounts":        "Applications.Datastores/mongoDatabases",
	"Microsoft.Sql/servers":                        "Applications.Datastores/sqlDatabases",
}

// MapToRadius converts parsed Bicep files into a RadiusApplication model.
func MapToRadius(files []BicepFile, appNameOverride string, sourceDir string) (RadiusApplication, error) {
	app := RadiusApplication{}

	// Find main.bicep to discover topology
	var mainFile *BicepFile
	for i := range files {
		if filepath.Base(files[i].Path) == "main.bicep" {
			mainFile = &files[i]
			break
		}
	}

	// Derive application name
	app.Name = deriveAppName(mainFile, appNameOverride, sourceDir)

	// Classify resources from all files
	var containerAppFiles []BicepFile
	var dependencyFiles []BicepFile

	for _, f := range files {
		if filepath.Base(f.Path) == "main.bicep" {
			continue
		}

		for _, res := range f.Resources {
			if isContainerApp(res.Type) {
				containerAppFiles = append(containerAppFiles, f)
				break
			}

			if isDependencyResource(res.Type) {
				dependencyFiles = append(dependencyFiles, f)
				break
			}
		}
	}

	// Map containers
	for _, f := range containerAppFiles {
		for _, res := range f.Resources {
			if !isContainerApp(res.Type) {
				continue
			}

			container := mapContainer(res, f.Path, sourceDir)
			app.Containers = append(app.Containers, container)

			// Add image parameter
			app.Parameters = append(app.Parameters, RadiusParameter{
				Name:         container.ImageParam,
				Type:         "string",
				DefaultValue: container.ImageDefault,
				Description:  fmt.Sprintf("Container image for %s.", container.Name),
			})
		}
	}

	// Map dependencies
	for _, f := range dependencyFiles {
		for _, res := range f.Resources {
			if !isDependencyResource(res.Type) {
				continue
			}

			dep := mapDependency(res, sourceDir)
			app.Dependencies = append(app.Dependencies, dep)
		}
	}

	// Sort for determinism
	sort.Slice(app.Containers, func(i, j int) bool {
		return app.Containers[i].Name < app.Containers[j].Name
	})
	sort.Slice(app.Dependencies, func(i, j int) bool {
		return app.Dependencies[i].Name < app.Dependencies[j].Name
	})
	sort.Slice(app.Parameters, func(i, j int) bool {
		return app.Parameters[i].Name < app.Parameters[j].Name
	})

	return app, nil
}

// deriveAppName determines the application name from available sources.
func deriveAppName(mainFile *BicepFile, appNameOverride string, sourceDir string) string {
	if appNameOverride != "" {
		return appNameOverride
	}

	// Try to derive from main.bicep environmentName parameter
	if mainFile != nil {
		for _, param := range mainFile.Parameters {
			if param.Name == "environmentName" && param.DefaultValue != "" {
				return strings.Trim(param.DefaultValue, "'\"")
			}
		}
	}

	// Fall back to directory name
	dirName := filepath.Base(sourceDir)
	if dirName == "infra" || dirName == "." {
		// Go up one level
		dirName = filepath.Base(filepath.Dir(sourceDir))
	}

	// Replace conventions for Aspire project names
	dirName = strings.ToLower(dirName)
	dirName = strings.ReplaceAll(dirName, " ", "-")
	dirName = strings.ReplaceAll(dirName, "_", "-")

	if dirName == "" || dirName == "." {
		return "aspire-app"
	}

	return dirName
}

// mapContainer maps a Microsoft.App/containerApps resource to a RadiusContainer.
func mapContainer(res BicepResource, filePath string, sourceDir string) RadiusContainer {
	container := RadiusContainer{
		Name:       res.SymbolicName,
		ImageParam: res.SymbolicName + "Image",
	}

	// Extract image from containers
	containers, _ := res.Properties["containers"].([]map[string]any)
	if len(containers) > 0 {
		if image, ok := containers[0]["image"].(string); ok && image != "" {
			container.ImageDefault = image
		}
	}

	if container.ImageDefault == "" {
		container.ImageDefault = res.SymbolicName + ":latest"
	}

	// Extract ports from ingress
	ingress, _ := res.Properties["ingress"].(map[string]any)
	if ingress != nil {
		if targetPort, ok := ingress["targetPort"].(int); ok {
			transport := "TCP"
			if t, ok := ingress["transport"].(string); ok {
				switch strings.ToLower(t) {
				case "http", "http2":
					transport = "TCP"
				case "tcp":
					transport = "TCP"
				case "udp":
					transport = "UDP"
				}
			}

			container.Ports = append(container.Ports, RadiusPort{
				Name:          "http",
				ContainerPort: targetPort,
				Protocol:      transport,
			})
		}

		if external, ok := ingress["external"].(bool); ok {
			container.IsExternal = external
		}
	}

	// Extract env vars and derive connections
	if len(containers) > 0 {
		if envVars, ok := containers[0]["env"].([]map[string]string); ok {
			connStrPrefix := "ConnectionStrings__"

			for _, env := range envVars {
				name := env["name"]

				if strings.HasPrefix(name, connStrPrefix) {
					// This is a connection reference
					targetName := strings.TrimPrefix(name, connStrPrefix)
					container.Connections = append(container.Connections, RadiusConnection{
						Name:               targetName,
						TargetResourceName: targetName,
						Source:             targetName + ".id",
					})
				}
			}
		}
	}

	// Sort connections for determinism
	sort.Slice(container.Connections, func(i, j int) bool {
		return container.Connections[i].Name < container.Connections[j].Name
	})

	return container
}

// mapDependency maps a dependency resource to a RadiusDependency.
func mapDependency(res BicepResource, sourceDir string) RadiusDependency {
	dep := RadiusDependency{
		Name: res.SymbolicName,
	}

	// Look up the resource type in the mapping table
	baseType := extractBaseResourceType(res.Type)
	if radiusType, ok := dependencyTypeMap[baseType]; ok {
		dep.Type = radiusType
		dep.IsRecipeBacked = true
	} else {
		// Unsupported type — create placeholder
		dep.IsPlaceholder = true
		dep.PlaceholderComment = fmt.Sprintf("PLACEHOLDER: Azure resource type '%s' has no Portable Resource equivalent — manual configuration required", res.Type)
	}

	return dep
}

// isContainerApp checks if a resource type is a Microsoft.App/containerApps resource.
func isContainerApp(resourceType string) bool {
	return strings.HasPrefix(strings.ToLower(resourceType), "microsoft.app/containerapps")
}

// isDependencyResource checks if a resource type is a known dependency resource.
func isDependencyResource(resourceType string) bool {
	baseType := extractBaseResourceType(resourceType)
	// Check the mapping table
	if _, ok := dependencyTypeMap[baseType]; ok {
		return true
	}

	// Also match any non-containerApp Azure resource as a potential dependency
	lower := strings.ToLower(resourceType)
	return strings.HasPrefix(lower, "microsoft.cache/") ||
		strings.HasPrefix(lower, "microsoft.documentdb/") ||
		strings.HasPrefix(lower, "microsoft.sql/") ||
		strings.HasPrefix(lower, "microsoft.dbforpostgresql/")
}

// extractBaseResourceType extracts the base resource type without the API version.
// e.g., "Microsoft.Cache/redis@2023-08-01" -> "Microsoft.Cache/redis"
func extractBaseResourceType(resourceType string) string {
	if idx := strings.Index(resourceType, "@"); idx >= 0 {
		return resourceType[:idx]
	}
	return resourceType
}
