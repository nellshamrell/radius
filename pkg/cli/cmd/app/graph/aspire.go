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
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/to"
)

// aspireResource represents a resource declaration extracted from an Aspire AppHost C# source file.
type aspireResource struct {
	// varName is the C# variable name (e.g., "cache"). Empty for inline declarations.
	varName string
	// resourceName is the Aspire resource name from the first string argument (e.g., "cache").
	resourceName string
	// builderMethod is the Add method suffix (e.g., "Redis", "Project", "SqlServer").
	builderMethod string
	// radiusType is the mapped Radius resource type (e.g., "Applications.Datastores/redisCaches").
	radiusType string
	// isExternal is true if the resource has .WithExternalHttpEndpoints() in its chain.
	isExternal bool
	// parentVarName is the variable name of the parent resource for sub-resources (e.g., for AddDatabase chained on AddSqlServer).
	parentVarName string
}

// aspireConnection represents a connection edge between two Aspire resources, extracted from .WithReference() calls.
type aspireConnection struct {
	// sourceResourceName is the name of the resource whose chain contains the .WithReference() call.
	sourceResourceName string
	// targetResourceName is the name of the resource being referenced.
	targetResourceName string
	// connectionName is the connection name (defaults to target resource name).
	connectionName string
}

// aspireTypeMapping maps Aspire builder method suffixes to Radius resource types.
var aspireTypeMapping = map[string]string{
	// Compute
	"Project":    "Applications.Core/containers",
	"Container":  "Applications.Core/containers",
	"Executable": "Applications.Core/containers",
	"NpmApp":     "Applications.Core/containers",
	"ViteApp":    "Applications.Core/containers",
	"PythonApp":  "Applications.Core/containers",
	"GolangApp":  "Applications.Core/containers",
	"JavaApp":    "Applications.Core/containers",
	"RustApp":    "Applications.Core/containers",
	"BunApp":     "Applications.Core/containers",
	"DenoApp":    "Applications.Core/containers",

	// Cache
	"Redis":  "Applications.Datastores/redisCaches",
	"Valkey": "Applications.Datastores/redisCaches",
	"Garnet": "Applications.Datastores/redisCaches",

	// Database — SQL
	"SqlServer": "Applications.Datastores/sqlDatabases",
	"Postgres":  "Applications.Datastores/sqlDatabases",
	"MySql":     "Applications.Datastores/sqlDatabases",
	"Oracle":    "Applications.Datastores/sqlDatabases",

	// Database — NoSQL
	"MongoDB": "Applications.Datastores/mongoDatabases",

	// Messaging
	"RabbitMQ": "Applications.Messaging/rabbitMQQueues",
	"Kafka":    "Applications.Messaging/rabbitMQQueues",
	"Nats":     "Applications.Messaging/rabbitMQQueues",
}

// fallbackRadiusType is the generic fallback type used for unmapped Aspire resource types.
const fallbackRadiusType = "Applications.Core/extenders"

// Compiled regex patterns for parsing Aspire AppHost C# source.
var (
	// resourceDeclPattern matches `var {varName} = builder.Add{Method}<...>("{name}"`.
	resourceDeclPattern = regexp.MustCompile(`var\s+(\w+)\s*=\s*builder\.Add(\w+)(?:<[^>]+>)?\(\s*"([^"]+)"`)

	// inlineResourcePattern matches `builder.Add{Method}<...>("{name}"` without variable assignment.
	inlineResourcePattern = regexp.MustCompile(`builder\.Add(\w+)(?:<[^>]+>)?\(\s*"([^"]+)"`)

	// chainedAddPattern matches `.Add{Method}("{name}"` in a fluent chain.
	chainedAddPattern = regexp.MustCompile(`\.Add(\w+)\(\s*"([^"]+)"`)

	// withReferencePattern matches `.WithReference({varName}`.
	withReferencePattern = regexp.MustCompile(`\.WithReference\(\s*(\w+)`)

	// externalEndpointPattern matches `.WithExternalHttpEndpoints()`.
	externalEndpointPattern = regexp.MustCompile(`\.WithExternalHttpEndpoints\(\)`)

	// waitForPattern matches `.WaitFor({varName}` — recognized and silently skipped.
	waitForPattern = regexp.MustCompile(`\.WaitFor\(\s*(\w+)`)
)

// mapAspireTypeToRadius maps an Aspire builder method suffix to a Radius resource type.
// Returns the Radius type and a boolean indicating whether the mapping is known.
// Unknown methods return the fallback type and false.
func mapAspireTypeToRadius(method string) (string, bool) {
	if radiusType, ok := aspireTypeMapping[method]; ok {
		return radiusType, true
	}
	return fallbackRadiusType, false
}

// parseAspireAppHost parses a C# source file from an Aspire AppHost project and extracts
// the application topology: resource declarations, connection edges, and warnings.
//
// The parsing proceeds in five phases:
//  1. Resource declaration extraction (builder.Add* calls)
//  2. Fluent chain resolution (chained .Add* calls creating sub-resources)
//  3. Connection edge extraction (.WithReference() calls)
//  4. External endpoint detection (.WithExternalHttpEndpoints())
//  5. WaitFor recognition (silently skipped)
func parseAspireAppHost(content string) ([]aspireResource, []aspireConnection, []string, error) {
	var resources []aspireResource
	var connections []aspireConnection
	var warnings []string

	// Split content into statements by finding semicolons.
	// We need to work with full statements to handle multi-line fluent chains.
	statements := splitStatements(content)

	// Maps variable names to their resolved resource name (accounting for chain resolution).
	varToResolvedResource := map[string]string{}
	// Maps variable names to the resource they were declared with (first Add* call).
	varToDeclaredResource := map[string]string{}
	// Track which resource names are defined by inline declarations (no variable).
	inlineResources := map[string]bool{}
	// Track all resource names to detect the "owning" resource of a statement for WithReference.
	stmtOwnerResource := map[int]string{}

	for i, stmt := range statements {
		// Phase 1: Resource declaration extraction
		declMatch := resourceDeclPattern.FindStringSubmatch(stmt)
		if declMatch != nil {
			varName := declMatch[1]
			builderMethod := declMatch[2]
			resourceName := declMatch[3]

			radiusType, known := mapAspireTypeToRadius(builderMethod)
			if !known {
				warnings = append(warnings, fmt.Sprintf("warning: unknown Aspire resource type 'Add%s' for resource '%s', using generic fallback type", builderMethod, resourceName))
			}

			res := aspireResource{
				varName:       varName,
				resourceName:  resourceName,
				builderMethod: builderMethod,
				radiusType:    radiusType,
			}
			resources = append(resources, res)
			varToDeclaredResource[varName] = resourceName
			varToResolvedResource[varName] = resourceName
			stmtOwnerResource[i] = resourceName

			// Phase 2: Fluent chain resolution
			// Find all chained .Add*("name") calls in the statement after the initial builder.Add*
			// We need to find the part after the initial match
			initialMatchEnd := resourceDeclPattern.FindStringIndex(stmt)
			if initialMatchEnd != nil {
				remainder := stmt[initialMatchEnd[1]:]
				chainMatches := chainedAddPattern.FindAllStringSubmatch(remainder, -1)
				for _, cm := range chainMatches {
					chainMethod := cm[1]
					chainResourceName := cm[2]

					var chainRadiusType string
					if chainMethod == "Database" {
						// Database inherits parent server's Radius type
						chainRadiusType = radiusType
					} else {
						var chainKnown bool
						chainRadiusType, chainKnown = mapAspireTypeToRadius(chainMethod)
						if !chainKnown {
							warnings = append(warnings, fmt.Sprintf("warning: unknown Aspire resource type 'Add%s' for resource '%s', using generic fallback type", chainMethod, chainResourceName))
						}
					}

					chainRes := aspireResource{
						resourceName:  chainResourceName,
						builderMethod: chainMethod,
						radiusType:    chainRadiusType,
						parentVarName: varName,
					}
					resources = append(resources, chainRes)

					// The variable resolves to the last Add* in the chain
					varToResolvedResource[varName] = chainResourceName
				}
			}

			// Phase 4: External endpoint detection
			if externalEndpointPattern.MatchString(stmt) {
				// Mark the first resource in this statement as external
				for j := range resources {
					if resources[j].resourceName == resourceName {
						resources[j].isExternal = true
						break
					}
				}
			}

			continue
		}

		// Check for inline resource declarations (no variable assignment)
		inlineMatches := inlineResourcePattern.FindAllStringSubmatch(stmt, -1)
		if len(inlineMatches) > 0 {
			// Only process if this is NOT a `var x = builder.Add...` (already handled above)
			if !resourceDeclPattern.MatchString(stmt) {
				firstResourceName := ""
				for idx, im := range inlineMatches {
					builderMethod := im[1]
					resourceName := im[2]

					radiusType, known := mapAspireTypeToRadius(builderMethod)
					if !known {
						warnings = append(warnings, fmt.Sprintf("warning: unknown Aspire resource type 'Add%s' for resource '%s', using generic fallback type", builderMethod, resourceName))
					}

					res := aspireResource{
						varName:       "", // inline, no variable
						resourceName:  resourceName,
						builderMethod: builderMethod,
						radiusType:    radiusType,
					}

					if idx == 0 {
						firstResourceName = resourceName
						stmtOwnerResource[i] = resourceName
					} else {
						// Chained Add* after inline — treat as sub-resource
						res.parentVarName = "__inline_" + firstResourceName
					}

					resources = append(resources, res)
					inlineResources[resourceName] = true
				}

				// Phase 4: External endpoint detection for inline resources
				if externalEndpointPattern.MatchString(stmt) && firstResourceName != "" {
					for j := range resources {
						if resources[j].resourceName == firstResourceName {
							resources[j].isExternal = true
							break
						}
					}
				}
			}
		}
	}

	// Phase 3: Connection edge extraction and Phase 5: WaitFor skip
	for i, stmt := range statements {
		// Determine the source resource for this statement
		sourceResourceName := stmtOwnerResource[i]
		if sourceResourceName == "" {
			continue
		}

		// Phase 5: WaitFor — recognized and silently skipped
		// We don't need to explicitly handle this since we only extract WithReference edges.
		// But we document this for clarity: .WaitFor() patterns are parsed and ignored.
		_ = waitForPattern.FindAllStringSubmatch(stmt, -1)

		// Phase 3: Connection edge extraction via .WithReference(varName)
		refMatches := withReferencePattern.FindAllStringSubmatch(stmt, -1)
		for _, rm := range refMatches {
			targetVarName := rm[1]

			// Resolve the variable name to its resource name
			resolvedName, found := varToResolvedResource[targetVarName]
			if !found {
				warnings = append(warnings, fmt.Sprintf("warning: could not resolve reference to '%s' for resource '%s'", targetVarName, sourceResourceName))
				continue
			}

			conn := aspireConnection{
				sourceResourceName: sourceResourceName,
				targetResourceName: resolvedName,
				connectionName:     resolvedName,
			}
			connections = append(connections, conn)
		}
	}

	// Sort resources deterministically by name
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].resourceName < resources[j].resourceName
	})

	// Sort connections deterministically
	sort.Slice(connections, func(i, j int) bool {
		if connections[i].sourceResourceName != connections[j].sourceResourceName {
			return connections[i].sourceResourceName < connections[j].sourceResourceName
		}
		return connections[i].targetResourceName < connections[j].targetResourceName
	})

	return resources, connections, warnings, nil
}

// splitStatements splits C# source content into statements by semicolons,
// preserving multi-line fluent chains as single statements.
func splitStatements(content string) []string {
	var statements []string
	var current strings.Builder

	for _, ch := range content {
		current.WriteRune(ch)
		if ch == ';' {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
		}
	}

	// Handle any trailing content without a semicolon
	if remaining := strings.TrimSpace(current.String()); remaining != "" {
		statements = append(statements, remaining)
	}

	return statements
}

// aspireResourcesToGenericResources converts parsed Aspire resources and connections into
// a slice of GenericResource suitable for consumption by ComputeGraph.
func aspireResourcesToGenericResources(resources []aspireResource, connections []aspireConnection) ([]generated.GenericResource, error) {
	// Build a map of connections grouped by source resource name
	connectionsBySource := map[string][]aspireConnection{}
	for _, conn := range connections {
		connectionsBySource[conn.sourceResourceName] = append(connectionsBySource[conn.sourceResourceName], conn)
	}

	var result []generated.GenericResource

	for _, res := range resources {
		properties := map[string]any{
			"provisioningState": "NotDeployed",
			"status":            map[string]any{"outputResources": []any{}},
		}

		// Build connections map for this resource
		if conns, ok := connectionsBySource[res.resourceName]; ok {
			connMap := map[string]any{}
			for _, conn := range conns {
				targetID := synthesizeResourceID(findRadiusTypeForResource(resources, conn.targetResourceName), conn.targetResourceName)
				connMap[conn.connectionName] = map[string]any{
					"source": targetID,
				}
			}
			if len(connMap) > 0 {
				properties["connections"] = connMap
			}
		}

		// Add external annotation if applicable
		if res.isExternal {
			properties["external"] = true
		}

		gr := generated.GenericResource{
			ID:         to.Ptr(synthesizeResourceID(res.radiusType, res.resourceName)),
			Name:       to.Ptr(res.resourceName),
			Type:       to.Ptr(res.radiusType),
			Properties: properties,
		}
		result = append(result, gr)
	}

	// Sort deterministically by type, then name
	sort.Slice(result, func(i, j int) bool {
		if *result[i].Type != *result[j].Type {
			return *result[i].Type < *result[j].Type
		}
		return *result[i].Name < *result[j].Name
	})

	return result, nil
}

// findRadiusTypeForResource looks up the Radius type for a resource by name.
func findRadiusTypeForResource(resources []aspireResource, name string) string {
	for _, r := range resources {
		if r.resourceName == name {
			return r.radiusType
		}
	}
	return fallbackRadiusType
}

// discoverAppHostProject discovers the Aspire AppHost project directory and .csproj file
// from the given path. The discovery follows this priority order:
//  1. If path is a .csproj file, use it directly
//  2. If path is a directory containing *.AppHost.csproj, use it
//  3. If .aspire/settings.json exists in the directory or parent directory, follow appHostPath
//  4. Error if none found
func discoverAppHostProject(path string) (string, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", "", fmt.Errorf("path not found: %s", path)
	}

	// Case 1: Direct .csproj file
	if !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".csproj") {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return "", "", err
		}
		return filepath.Dir(absPath), absPath, nil
	}

	// Resolve to directory
	dir := path
	if !info.IsDir() {
		dir = filepath.Dir(path)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}

	// Case 2: Look for *.AppHost.csproj in directory
	csprojPath, found := findAppHostCsproj(absDir)
	if found {
		return absDir, csprojPath, nil
	}

	// Case 3: Look for .aspire/settings.json in the directory
	settingsPath := filepath.Join(absDir, ".aspire", "settings.json")
	projectDir, csprojPath, err := resolveFromSettings(settingsPath)
	if err == nil {
		return projectDir, csprojPath, nil
	}

	// Case 3b: Look for .aspire/settings.json in parent directory
	parentDir := filepath.Dir(absDir)
	if parentDir != absDir {
		settingsPath = filepath.Join(parentDir, ".aspire", "settings.json")
		projectDir, csprojPath, err = resolveFromSettings(settingsPath)
		if err == nil {
			return projectDir, csprojPath, nil
		}
	}

	return "", "", fmt.Errorf("no Aspire AppHost project found at %s", path)
}

// findAppHostCsproj looks for a *.AppHost.csproj file in the given directory.
func findAppHostCsproj(dir string) (string, bool) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.AppHost.csproj"))
	if err != nil || len(matches) == 0 {
		return "", false
	}
	return matches[0], true
}

// aspireSettings represents the structure of .aspire/settings.json.
type aspireSettings struct {
	AppHostPath string `json:"appHostPath"`
}

// resolveFromSettings reads .aspire/settings.json and resolves the appHostPath to a project directory.
func resolveFromSettings(settingsPath string) (string, string, error) {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return "", "", err
	}

	var settings aspireSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return "", "", fmt.Errorf("invalid settings.json: %w", err)
	}

	if settings.AppHostPath == "" {
		return "", "", fmt.Errorf("settings.json has no appHostPath")
	}

	// appHostPath is relative to the .aspire/ directory
	aspireDir := filepath.Dir(settingsPath)
	resolvedPath := filepath.Join(aspireDir, settings.AppHostPath)

	// Check if the resolved path is a .csproj file
	if strings.HasSuffix(strings.ToLower(resolvedPath), ".csproj") {
		absPath, err := filepath.Abs(resolvedPath)
		if err != nil {
			return "", "", err
		}
		if _, err := os.Stat(absPath); err != nil {
			return "", "", fmt.Errorf("appHostPath target not found: %s", absPath)
		}
		return filepath.Dir(absPath), absPath, nil
	}

	// It's a directory — look for *.AppHost.csproj inside it
	absDir, err := filepath.Abs(resolvedPath)
	if err != nil {
		return "", "", err
	}

	csprojPath, found := findAppHostCsproj(absDir)
	if found {
		return absDir, csprojPath, nil
	}

	return "", "", fmt.Errorf("no AppHost .csproj found in %s", absDir)
}

// findEntryPointFile finds the C# entry point file in the AppHost project directory.
// It follows this priority order:
//  1. AppHost.cs (current Aspire 9.x+ convention)
//  2. Program.cs (older convention)
//  3. Scan *.cs files for DistributedApplication.CreateBuilder
func findEntryPointFile(projectDir string) (string, error) {
	// Priority 1: AppHost.cs
	appHostPath := filepath.Join(projectDir, "AppHost.cs")
	if _, err := os.Stat(appHostPath); err == nil {
		return appHostPath, nil
	}

	// Priority 2: Program.cs
	programPath := filepath.Join(projectDir, "Program.cs")
	if _, err := os.Stat(programPath); err == nil {
		return programPath, nil
	}

	// Priority 3: Scan *.cs files for DistributedApplication.CreateBuilder
	matches, err := filepath.Glob(filepath.Join(projectDir, "*.cs"))
	if err != nil {
		return "", fmt.Errorf("no AppHost entry point file (AppHost.cs or Program.cs) found in %s", projectDir)
	}

	for _, match := range matches {
		data, err := os.ReadFile(match)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "DistributedApplication.CreateBuilder") {
			return match, nil
		}
	}

	return "", fmt.Errorf("no AppHost entry point file (AppHost.cs or Program.cs) found in %s", projectDir)
}

// deriveApplicationName determines the application name for graph display.
// It follows this priority order:
//  1. Search for .rad/rad.yaml starting from the --from-aspire input path, read workspace.application
//  2. Strip .AppHost suffix from .csproj project name and lowercase
func deriveApplicationName(inputPath string, projectDir string, csprojPath string) string {
	// Priority 1: Look for .rad/rad.yaml starting from the input path
	searchDir := inputPath
	if info, err := os.Stat(inputPath); err == nil && !info.IsDir() {
		searchDir = filepath.Dir(inputPath)
	}

	absSearchDir, err := filepath.Abs(searchDir)
	if err == nil {
		appName := findRadYamlAppName(absSearchDir)
		if appName != "" {
			return appName
		}
	}

	// Priority 2: Derive from .csproj project name
	if csprojPath != "" {
		baseName := filepath.Base(csprojPath)
		projectName := strings.TrimSuffix(baseName, filepath.Ext(baseName))
		// Strip .AppHost suffix (case-insensitive)
		if idx := strings.LastIndex(strings.ToLower(projectName), ".apphost"); idx >= 0 {
			projectName = projectName[:idx]
		}
		return strings.ToLower(projectName)
	}

	// Fallback: use the directory name
	return strings.ToLower(filepath.Base(projectDir))
}

// radYamlConfig represents the minimal structure of .rad/rad.yaml needed for app name derivation.
type radYamlConfig struct {
	Workspace struct {
		Application string `json:"application" yaml:"application"`
	} `json:"workspace" yaml:"workspace"`
}

// findRadYamlAppName searches for .rad/rad.yaml in the given directory and returns
// the workspace.application value if present.
func findRadYamlAppName(dir string) string {
	radYamlPath := filepath.Join(dir, ".rad", "rad.yaml")
	data, err := os.ReadFile(radYamlPath)
	if err != nil {
		return ""
	}

	// Parse as simple key-value (rad.yaml is a simple YAML format).
	// We parse manually to avoid requiring a YAML dependency for this simple case.
	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "application:") {
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, "application:"))
			// Remove quotes if present
			value = strings.Trim(value, `"'`)
			if value != "" {
				return value
			}
		}
	}

	return ""
}
