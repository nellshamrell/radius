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
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/to"
)

var referencePattern = regexp.MustCompile(`\[reference\('(\w+)'\)\.id\]`)

// resourceEntry is a transient intermediate type used during ARM JSON template processing.
// It holds the parsed fields from a single resource in the ARM template's resources map.
type resourceEntry struct {
	SymbolicName  string
	Type          string         // with API version stripped
	Name          string         // from properties.name
	Import        string         // e.g., "Radius"
	Properties    map[string]any // inner properties (Radius properties)
	HasCondition  bool
	SynthesizedID string
}

// stripAPIVersion removes the @apiVersion suffix from an ARM resource type string.
// For example, "Applications.Core/containers@2023-10-01-preview" becomes "Applications.Core/containers".
func stripAPIVersion(armType string) string {
	if idx := strings.Index(armType, "@"); idx >= 0 {
		return armType[:idx]
	}
	return armType
}

// synthesizeResourceID generates a synthetic Radius resource ID for use as a graph node key.
// The ID follows the format: /planes/radius/local/resourceGroups/default/providers/{Type}/{Name}
func synthesizeResourceID(resourceType, resourceName string) string {
	return fmt.Sprintf("/planes/radius/local/resourceGroups/default/providers/%s/%s", resourceType, resourceName)
}

// isRadiusResource returns true if the resource is a Radius-managed resource, determined by
// the import field being "Radius" (case-insensitive) or the resource type starting with a
// known Radius namespace prefix ("Applications." or "Radius.").
func isRadiusResource(importField string, resourceType string) bool {
	if strings.EqualFold(importField, "Radius") {
		return true
	}
	if strings.HasPrefix(resourceType, "Applications.") || strings.HasPrefix(resourceType, "Radius.") {
		return true
	}
	return false
}

// resolveExpression resolves an ARM template expression value to a concrete resource ID.
// It handles three cases:
//   - Literal strings (no "[" prefix): returned as-is with ok=true
//   - Reference expressions matching [reference('symbolicName').id]: resolved to the
//     synthesized resource ID of the referenced resource, with ok=true if found
//   - All other expressions (parameters, format, etc.): returned as ("", false)
func resolveExpression(value string, resources map[string]resourceEntry) (string, bool) {
	if !strings.HasPrefix(value, "[") {
		return value, true
	}

	matches := referencePattern.FindStringSubmatch(value)
	if len(matches) == 2 {
		symbolicName := matches[1]
		entry, found := resources[symbolicName]
		if found {
			return entry.SynthesizedID, true
		}
		return "", false
	}

	return "", false
}

// extractResourcesFromTemplate parses an ARM JSON template and converts its resources into
// a slice of GenericResource suitable for consumption by ComputeGraph. It resolves ARM reference
// expressions for connections, routes, and application fields. Unresolvable expressions, conditional
// resources, and module references produce warnings rather than errors.
//
// The template parameter should be the output of PrepareTemplate() or a parsed ARM JSON file.
// Returns the extracted resources in deterministic order (sorted by type, then name), any warnings
// generated during processing, and a non-nil error only for fatal template structure issues.
func extractResourcesFromTemplate(template map[string]any) ([]generated.GenericResource, []string, error) {
	resourcesRaw, ok := template["resources"]
	if !ok {
		return nil, nil, fmt.Errorf("template does not contain a 'resources' key")
	}

	resourcesMap, ok := resourcesRaw.(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("template 'resources' is not a map")
	}

	if len(resourcesMap) == 0 {
		return []generated.GenericResource{}, nil, nil
	}

	var warnings []string

	// First pass: build resourceEntry lookup table
	entries := map[string]resourceEntry{}
	for symbolicName, raw := range resourcesMap {
		resMap, ok := raw.(map[string]any)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("resource %q has unexpected format, skipping", symbolicName))
			continue
		}

		armType, _ := resMap["type"].(string)
		if armType == "" {
			warnings = append(warnings, fmt.Sprintf("resource %q has no type field, skipping", symbolicName))
			continue
		}

		strippedType := stripAPIVersion(armType)
		importField, _ := resMap["import"].(string)

		// Extract name from properties.name, fall back to symbolic name
		name := symbolicName
		if props, ok := resMap["properties"].(map[string]any); ok {
			if n, ok := props["name"].(string); ok && n != "" {
				name = n
			}
		}

		// Detect condition
		_, hasCondition := resMap["condition"]

		entry := resourceEntry{
			SymbolicName:  symbolicName,
			Type:          strippedType,
			Name:          name,
			Import:        importField,
			HasCondition:  hasCondition,
			SynthesizedID: synthesizeResourceID(strippedType, name),
		}

		// Extract inner properties for Radius resources
		if props, ok := resMap["properties"].(map[string]any); ok {
			if innerProps, ok := props["properties"].(map[string]any); ok {
				entry.Properties = innerProps
			}
		}

		entries[symbolicName] = entry
	}

	// Check for duplicate resource names
	seen := map[string]bool{}
	for _, entry := range entries {
		key := entry.Type + "/" + entry.Name
		if seen[key] {
			warnings = append(warnings, fmt.Sprintf("duplicate resource detected: %s/%s", entry.Type, entry.Name))
		}
		seen[key] = true
	}

	// Second pass: build GenericResource list
	var result []generated.GenericResource
	for _, entry := range entries {
		// Warn on conditions
		if entry.HasCondition {
			warnings = append(warnings, fmt.Sprintf("resource %q has a condition and may not be deployed", entry.Name))
		}

		// Warn and include module references
		if strings.EqualFold(entry.Type, "Microsoft.Resources/deployments") {
			warnings = append(warnings, fmt.Sprintf("module reference %q detected; nested resources are not included in the graph", entry.Name))
		}

		properties := map[string]any{
			"provisioningState": "NotDeployed",
			"status":            map[string]any{"outputResources": []any{}},
		}

		if isRadiusResource(entry.Import, entry.Type) && entry.Properties != nil {
			// Resolve application field
			if appRaw, ok := entry.Properties["application"]; ok {
				if appStr, ok := appRaw.(string); ok {
					resolved, ok := resolveExpression(appStr, entries)
					if ok {
						properties["application"] = resolved
					} else {
						warnings = append(warnings, fmt.Sprintf("cannot resolve application reference %q for resource %q: unsupported expression", appStr, entry.Name))
					}
				}
			}

			// Resolve environment field
			if envRaw, ok := entry.Properties["environment"]; ok {
				if envStr, ok := envRaw.(string); ok {
					resolved, ok := resolveExpression(envStr, entries)
					if ok {
						properties["environment"] = resolved
					}
				}
			}

			// Resolve connections
			if connectionsRaw, ok := entry.Properties["connections"]; ok {
				if connectionsMap, ok := connectionsRaw.(map[string]any); ok {
					resolvedConnections := map[string]any{}
					for connName, connRaw := range connectionsMap {
						connMap, ok := connRaw.(map[string]any)
						if !ok {
							continue
						}
						sourceRaw, ok := connMap["source"]
						if !ok {
							continue
						}
						sourceStr, ok := sourceRaw.(string)
						if !ok {
							continue
						}
						resolved, ok := resolveExpression(sourceStr, entries)
						if ok {
							resolvedConnections[connName] = map[string]any{"source": resolved}
						} else {
							warnings = append(warnings, fmt.Sprintf("cannot resolve connection source %q for resource %q: unsupported expression", sourceStr, entry.Name))
						}
					}
					if len(resolvedConnections) > 0 {
						properties["connections"] = resolvedConnections
					}
				}
			}

			// Resolve routes
			if routesRaw, ok := entry.Properties["routes"]; ok {
				if routesSlice, ok := routesRaw.([]any); ok {
					var resolvedRoutes []any
					for _, routeRaw := range routesSlice {
						routeMap, ok := routeRaw.(map[string]any)
						if !ok {
							continue
						}
						destRaw, ok := routeMap["destination"]
						if !ok {
							continue
						}
						destStr, ok := destRaw.(string)
						if !ok {
							continue
						}
						resolved, ok := resolveExpression(destStr, entries)
						if ok {
							resolvedRoute := map[string]any{}
							for k, v := range routeMap {
								resolvedRoute[k] = v
							}
							resolvedRoute["destination"] = resolved
							resolvedRoutes = append(resolvedRoutes, resolvedRoute)
						} else {
							warnings = append(warnings, fmt.Sprintf("cannot resolve route destination %q for resource %q: unsupported expression", destStr, entry.Name))
						}
					}
					if len(resolvedRoutes) > 0 {
						properties["routes"] = resolvedRoutes
					}
				}
			}
		}

		resource := generated.GenericResource{
			ID:         to.Ptr(entry.SynthesizedID),
			Name:       to.Ptr(entry.Name),
			Type:       to.Ptr(entry.Type),
			Properties: properties,
		}
		result = append(result, resource)
	}

	// Sort deterministically by type, then name
	sort.Slice(result, func(i, j int) bool {
		if *result[i].Type != *result[j].Type {
			return *result[i].Type < *result[j].Type
		}
		return *result[i].Name < *result[j].Name
	})

	// Sort warnings for deterministic output
	sort.Strings(warnings)

	return result, warnings, nil
}

// scopeToApplication analyzes the extracted resources to determine application scoping:
//   - 0 application resources: returns all resources with an empty app name (implicit application)
//   - 1 application resource: returns the app name and filters to resources referencing that application
//   - 2+ application resources: returns an error indicating multiple applications were found
func scopeToApplication(resources []generated.GenericResource) (string, []generated.GenericResource, error) {
	// Find application resources
	var appNames []string
	var appIDs []string
	for _, r := range resources {
		resType := to.String(r.Type)
		if isApplicationType(resType) {
			appNames = append(appNames, to.String(r.Name))
			appIDs = append(appIDs, to.String(r.ID))
		}
	}

	switch len(appNames) {
	case 0:
		// No application resource — include all resources in implicit application
		return "", resources, nil
	case 1:
		// Single application — filter to resources referencing that application
		appName := appNames[0]
		appID := appIDs[0]
		var filtered []generated.GenericResource
		for _, r := range resources {
			resType := to.String(r.Type)
			if isApplicationType(resType) {
				// Include the application resource itself
				filtered = append(filtered, r)
				continue
			}
			// Include resources whose "application" property references this app
			if appRef, ok := r.Properties["application"]; ok {
				if appRefStr, ok := appRef.(string); ok && appRefStr == appID {
					filtered = append(filtered, r)
					continue
				}
			}
		}
		return appName, filtered, nil
	default:
		return "", nil, fmt.Errorf("multiple applications found in template: %s. Use separate files for each application", strings.Join(appNames, ", "))
	}
}

// isApplicationType returns true if the resource type (without API version) represents
// a Radius application resource.
func isApplicationType(resourceType string) bool {
	lower := strings.ToLower(resourceType)
	return lower == "applications.core/applications" || lower == "radius.core/applications"
}
