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

import (
"fmt"
"regexp"
"sort"
"strings"
)

// resourceCategory indicates the kind of Radius resource an Aspire type maps to.
type resourceCategory string

const (
categoryContainer   resourceCategory = "container"
categoryDataStore   resourceCategory = "datastore"
categoryParameter   resourceCategory = "parameter"
categoryUnsupported resourceCategory = "unsupported"
)

// resourceTypeMapping describes how an Aspire resource type maps to a Radius resource type.
type resourceTypeMapping struct {
// RadiusType is the full Radius resource type with API version.
RadiusType string

// Category indicates the kind of Radius resource.
Category resourceCategory

// Extension is the Bicep extension required for this resource type.
Extension string
}

// resourceTypeMappings is the extensible mapping table from Aspire resource type strings
// to Radius resource types and categories.
var resourceTypeMappings = map[string]resourceTypeMapping{
"container.v0": {
RadiusType: "Radius.Compute/containers@2025-08-01-preview",
Category:   categoryContainer,
Extension:  "containers",
},
"container.v1": {
RadiusType: "Radius.Compute/containers@2025-08-01-preview",
Category:   categoryContainer,
Extension:  "containers",
},
"redis.server.v0": {
RadiusType: "Applications.Datastores/redisCaches@2023-10-01-preview",
Category:   categoryDataStore,
Extension:  "radius",
},
"postgres.server.v0": {
RadiusType: "Radius.Data/postgreSqlDatabases@2025-08-01-preview",
Category:   categoryDataStore,
Extension:  "radiusResources",
},
"mysql.server.v0": {
RadiusType: "Radius.Data/mySqlDatabases@2025-08-01-preview",
Category:   categoryDataStore,
Extension:  "radiusResources",
},
"parameter.v0": {
Category: categoryParameter,
},
}

// LookupResourceType returns the resource type mapping for the given Aspire type string.
// If the type is not found, a mapping with categoryUnsupported is returned.
func LookupResourceType(aspireType string) resourceTypeMapping {
if m, ok := resourceTypeMappings[aspireType]; ok {
return m
}
return resourceTypeMapping{Category: categoryUnsupported}
}

// expressionRefPattern matches Aspire expression references like {resource.property.path}.
var expressionRefPattern = regexp.MustCompile(`\{([^}]+)\}`)

// ExpressionRef represents a parsed Aspire expression reference.
type ExpressionRef struct {
// ResourceName is the referenced resource name.
ResourceName string

// PropertyPath is the dot-separated property path after the resource name.
PropertyPath string

// FullMatch is the complete matched expression including braces.
FullMatch string
}

// ParseExpressionRefs extracts all {resource.property.path} patterns from s
// and returns a slice of ExpressionRef.
func ParseExpressionRefs(s string) []ExpressionRef {
matches := expressionRefPattern.FindAllStringSubmatch(s, -1)
refs := make([]ExpressionRef, 0, len(matches))
for _, match := range matches {
inner := match[1]
parts := strings.SplitN(inner, ".", 2)
ref := ExpressionRef{
ResourceName: parts[0],
FullMatch:    match[0],
}
if len(parts) > 1 {
ref.PropertyPath = parts[1]
}
refs = append(refs, ref)
}
return refs
}

// resolveExpression resolves a single Aspire expression string to a BicepEnvVar.
// The symNameMap maps Aspire resource names to their Bicep symbolic names.
func resolveExpression(expr string, manifest *AspireManifest, symNameMap map[string]string) BicepEnvVar {
	refs := ParseExpressionRefs(expr)
	if len(refs) == 0 {
		return BicepEnvVar{Value: expr}
	}

	// Single reference that covers the entire expression.
	if len(refs) == 1 && refs[0].FullMatch == expr {
		resolved := resolveRef(refs[0], manifest, symNameMap)
		// If the resolved value is a bare identifier (no quotes), it's a Bicep expression.
		if !strings.HasPrefix(resolved, "'") {
			return BicepEnvVar{BicepExpression: resolved}
		}
		// If the quoted literal contains interpolation expressions, it's a string interpolation.
		if strings.Contains(resolved, "${") {
			return BicepEnvVar{StringInterpolation: resolved}
		}
		// It's a quoted literal — strip quotes and return as Value.
		return BicepEnvVar{Value: strings.Trim(resolved, "'")}
	}

	// Mixed expression: build a Bicep string interpolation.
	result := expr
	for _, ref := range refs {
		resolved := resolveRef(ref, manifest, symNameMap)
		// If the resolved value is a quoted literal, strip quotes and inline it.
		if strings.HasPrefix(resolved, "'") && strings.HasSuffix(resolved, "'") {
			literal := strings.Trim(resolved, "'")
			result = strings.Replace(result, ref.FullMatch, literal, 1)
		} else {
			// It's a Bicep expression — wrap in ${...} for string interpolation.
			result = strings.Replace(result, ref.FullMatch, "${"+resolved+"}", 1)
		}
	}
	return BicepEnvVar{StringInterpolation: "'" + result + "'"}
}

// resolveRef converts a single ExpressionRef into a Bicep expression string.
// Returns either a quoted literal (e.g., "'cache'", "'6379'") or a bare Bicep
// identifier (e.g., "cache_password", "cache_password_uri_encoded").
func resolveRef(ref ExpressionRef, manifest *AspireManifest, symNameMap map[string]string) string {
resource, exists := manifest.Resources[ref.ResourceName]

// Check if the referenced resource is a parameter.v0 → use parameter name.
if exists && resource.Type == "parameter.v0" {
paramName := sanitizeBicepIdentifier(ref.ResourceName)
return paramName
}

// Check if the referenced resource is an annotated.string → use variable name.
if exists && resource.Type == "annotated.string" {
varName := sanitizeBicepIdentifier(ref.ResourceName)
return varName
}

// Binding references (e.g., {cache.bindings.tcp.host}, {cache.bindings.tcp.port}).
if ref.PropertyPath != "" && strings.HasPrefix(ref.PropertyPath, "bindings.") {
parts := strings.Split(ref.PropertyPath, ".")
if len(parts) >= 3 {
bindingName := parts[1]
prop := parts[2]

// host → string literal of the resource name.
if prop == "host" {
return fmt.Sprintf("'%s'", ref.ResourceName)
}

// port or targetPort → string literal of the port number.
if prop == "port" || prop == "targetPort" {
if exists {
if binding, ok := resource.Bindings[bindingName]; ok {
return fmt.Sprintf("'%d'", binding.TargetPort)
}
}
// Fallback: return the resource name as a literal.
return fmt.Sprintf("'%s'", ref.FullMatch)
}

// url → full URL reference (best-effort).
if prop == "url" {
if exists {
if binding, ok := resource.Bindings[bindingName]; ok {
scheme := binding.Scheme
if scheme == "" {
scheme = "http"
}
return fmt.Sprintf("'%s://%s:%d'", scheme, ref.ResourceName, binding.TargetPort)
}
}
}
}
}

// connectionString reference → fully expand by resolving the resource's connection string template.
if ref.PropertyPath == "connectionString" {
if exists && resource.ConnectionString != "" {
return resolveConnectionString(resource.ConnectionString, manifest, symNameMap)
}
symName := lookupSymName(ref.ResourceName, symNameMap)
return symName + ".id"
}

// value reference on an unknown resource type → leave as literal.
if ref.PropertyPath == "value" {
return fmt.Sprintf("'%s'", ref.FullMatch)
}

// inputs.value reference on a parameter → use parameter name.
if ref.PropertyPath == "inputs.value" {
return sanitizeBicepIdentifier(ref.ResourceName)
}

return fmt.Sprintf("'%s'", ref.FullMatch)
}

// resolveConnectionString fully expands a connection string template by recursively
// resolving all embedded expression references. Returns the resolved value which may
// be a quoted literal, a bare expression, or a string interpolation expression.
func resolveConnectionString(connStr string, manifest *AspireManifest, symNameMap map[string]string) string {
refs := ParseExpressionRefs(connStr)
if len(refs) == 0 {
return fmt.Sprintf("'%s'", connStr)
}

result := connStr
hasExpressions := false
for _, ref := range refs {
resolved := resolveRef(ref, manifest, symNameMap)
if strings.HasPrefix(resolved, "'") && strings.HasSuffix(resolved, "'") {
// Literal — inline without quotes.
literal := strings.Trim(resolved, "'")
result = strings.Replace(result, ref.FullMatch, literal, 1)
} else {
// Bicep expression — wrap in ${...}.
result = strings.Replace(result, ref.FullMatch, "${"+resolved+"}", 1)
hasExpressions = true
}
}

if hasExpressions {
return "'" + result + "'"
}
return "'" + result + "'"
}

// lookupSymName returns the Bicep symbolic name for an Aspire resource name.
// Falls back to sanitizeBicepIdentifier if not found in the map.
func lookupSymName(resourceName string, symNameMap map[string]string) string {
if sym, ok := symNameMap[resourceName]; ok {
return sym
}
return sanitizeBicepIdentifier(resourceName)
}

// mapContainer maps an Aspire container resource to a BicepContainer.
func mapContainer(name string, resource AspireResource, manifest *AspireManifest, symNameMap map[string]string) BicepContainer {
symName := lookupSymName(name, symNameMap)

container := BicepContainer{
SymbolicName:   symName,
TypeName:       "Radius.Compute/containers@2025-08-01-preview",
Name:           name,
Image:          resource.Image,
ApplicationRef: "app.id",
EnvironmentRef: "environment",
}

// Map command (entrypoint + args).
if resource.Entrypoint != "" || len(resource.Args) > 0 {
var cmd []string
if resource.Entrypoint != "" {
cmd = append(cmd, resource.Entrypoint)
}
for _, arg := range resource.Args {
refs := ParseExpressionRefs(arg)
if len(refs) == 0 {
cmd = append(cmd, arg)
} else {
resolved := arg
for _, ref := range refs {
replacement := resolveRef(ref, manifest, symNameMap)
replacement = strings.Trim(replacement, "'")
resolved = strings.Replace(resolved, ref.FullMatch, replacement, 1)
}
cmd = append(cmd, resolved)
}
}
container.Command = cmd
}

// Map bindings to ports.
if len(resource.Bindings) > 0 {
container.Ports = make(map[string]BicepPort)
for bindingName, binding := range resource.Bindings {
container.Ports[bindingName] = mapBindingToPort(binding)
}
}

// Map environment variables.
if len(resource.Env) > 0 {
container.Env = make(map[string]BicepEnvVar)
for envKey, envVal := range resource.Env {
container.Env[envKey] = resolveExpression(envVal, manifest, symNameMap)
}
}

// Generate connections from env var references to other resources.
container.Connections = generateConnections(name, resource, manifest, symNameMap)

// Handle build warnings for container.v1 (FR-014).
	// Only set NeedsBuildWarning when buildOnly is false or absent.
	// BuildOnly resources are excluded before reaching this function (FR-019).
	if resource.Build != nil && !resource.Build.BuildOnly {
container.NeedsBuildWarning = true
container.BuildContext = resource.Build.Context
if container.Image == "" {
container.Image = fmt.Sprintf("<YOUR_REGISTRY>/%s:latest", name)
}
}

return container
}

// mapBindingToPort converts an AspireBinding to a BicepPort.
func mapBindingToPort(binding AspireBinding) BicepPort {
protocol := strings.ToUpper(binding.Protocol)
if protocol == "" {
protocol = "TCP"
}
return BicepPort{
ContainerPort: binding.TargetPort,
Protocol:      protocol,
}
}

// generateConnections detects references to other resources in env vars and connectionString,
// and produces BicepConnection entries for the consuming container.
func generateConnections(selfName string, resource AspireResource, manifest *AspireManifest, symNameMap map[string]string) map[string]BicepConnection {
connections := make(map[string]BicepConnection)

allRefs := make(map[string]bool)
for _, envVal := range resource.Env {
refs := ParseExpressionRefs(envVal)
for _, ref := range refs {
if ref.ResourceName != selfName {
allRefs[ref.ResourceName] = true
}
}
}

for _, arg := range resource.Args {
refs := ParseExpressionRefs(arg)
for _, ref := range refs {
if ref.ResourceName != selfName {
allRefs[ref.ResourceName] = true
}
}
}

for refName := range allRefs {
refResource, exists := manifest.Resources[refName]
if !exists {
continue
}

mapping := LookupResourceType(refResource.Type)
if mapping.Category == categoryContainer || mapping.Category == categoryDataStore {
symName := lookupSymName(refName, symNameMap)
connections[refName] = BicepConnection{
Source: symName + ".id",
}
}
}

return connections
}

// mapBackingService maps a backing-service Aspire resource (redis, postgres, mysql)
// to a BicepResource data store.
func mapBackingService(name string, mapping resourceTypeMapping, symNameMap map[string]string) BicepResource {
symName := lookupSymName(name, symNameMap)
return BicepResource{
SymbolicName: symName,
TypeName:     mapping.RadiusType,
Name:         name,
Properties: map[string]any{
"application": BicepExpr{Expression: "app.id"},
"environment": BicepExpr{Expression: "environment"},
},
}
}

// mapGateway generates a BicepGateway for a container that has external bindings.
func mapGateway(containerName string, resource AspireResource, symNameMap map[string]string) *BicepGateway {
var routes []BicepGatewayRoute
for _, binding := range resource.Bindings {
if binding.External {
routes = append(routes, BicepGatewayRoute{
Path: "/",
Port: binding.TargetPort,
})
}
}

if len(routes) == 0 {
return nil
}

sort.Slice(routes, func(i, j int) bool {
return routes[i].Port < routes[j].Port
})

containerSymName := lookupSymName(containerName, symNameMap)
gwSymName := containerSymName + "Gateway"
return &BicepGateway{
SymbolicName:   gwSymName,
TypeName:       "Radius.Compute/routes@2025-08-01-preview",
Name:           containerName + "-gateway",
ContainerRef:   containerSymName + ".id",
Routes:         routes,
ApplicationRef: "app.id",
EnvironmentRef: "environment",
}
}

// computeSymbolicNames pre-computes the mapping of Aspire resource names to
// deduplicated Bicep symbolic names. The application resource always uses "app".
func computeSymbolicNames(manifest *AspireManifest) map[string]string {
usedNames := map[string]bool{"app": true}
symNameMap := make(map[string]string)

resourceNames := make([]string, 0, len(manifest.Resources))
for name := range manifest.Resources {
resourceNames = append(resourceNames, name)
}
sort.Strings(resourceNames)

for _, name := range resourceNames {
resource := manifest.Resources[name]
		// Skip errored resources — they have no type for mapping.
		if resource.Error != "" {
			symNameMap[name] = sanitizeBicepIdentifier(name)
			continue
		}

		// Skip buildOnly resources — they are build-time artifacts, not runtime containers.
		if resource.Build != nil && resource.Build.BuildOnly {
			symNameMap[name] = sanitizeBicepIdentifier(name)
			continue
		}

mapping := LookupResourceType(resource.Type)

switch mapping.Category {
case categoryContainer, categoryDataStore:
symName := sanitizeBicepIdentifier(name)
symName = uniqueSymbolicName(symName, usedNames)
usedNames[symName] = true
symNameMap[name] = symName

if mapping.Category == categoryContainer {
for _, binding := range resource.Bindings {
if binding.External {
gwName := symName + "Gateway"
gwName = uniqueSymbolicName(gwName, usedNames)
usedNames[gwName] = true
break
}
}
}
case categoryParameter:
symNameMap[name] = sanitizeBicepIdentifier(name)
default:
symNameMap[name] = sanitizeBicepIdentifier(name)
}
}

return symNameMap
}

// MapManifest converts a parsed AspireManifest into a BicepFile intermediate representation.
// The applicationName parameter overrides the default application name if non-empty.
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
"environment": BicepExpr{Expression: "environment"},
},
},
}

symNameMap := computeSymbolicNames(manifest)


resourceNames := make([]string, 0, len(manifest.Resources))
for name := range manifest.Resources {
resourceNames = append(resourceNames, name)
}
sort.Strings(resourceNames)

for _, name := range resourceNames {
resource := manifest.Resources[name]

		// FR-018: Resources with an error field (no type) are manifest-publisher
		// errors. Skip them with a distinct warning before type-based mapping.
		if resource.Error != "" {
			file.Comments = append(file.Comments, BicepComment{
				ResourceName: name,
				ResourceType: "",
				Message:      fmt.Sprintf("manifest error: %s", resource.Error),
			})
			file.Warnings = append(file.Warnings, fmt.Sprintf(
				"resource %q: manifest error — %s",
				name, resource.Error,
			))
			continue
		}
		mapping := LookupResourceType(resource.Type)

		// FR-019: Resources with build.buildOnly=true are build-time-only artifacts.
		// Skip them entirely before normal container mapping.
		if resource.Build != nil && resource.Build.BuildOnly {
			file.Comments = append(file.Comments, BicepComment{
				ResourceName: name,
				ResourceType: "",
				Message:      "build-only artifact (build.buildOnly: true), not a runtime container",
			})
			file.Warnings = append(file.Warnings, fmt.Sprintf(
				"resource %q (%s): skipped — build-only artifact (build.buildOnly: true)",
				name, resource.Type,
			))
			continue
		}

switch mapping.Category {
case categoryContainer:
container := mapContainer(name, resource, manifest, symNameMap)
file.Containers = append(file.Containers, container)

if gw := mapGateway(name, resource, symNameMap); gw != nil {
file.Gateways = append(file.Gateways, *gw)
}

	case categoryDataStore:
		dataStore := mapBackingService(name, mapping, symNameMap)
		file.DataStores = append(file.DataStores, dataStore)

	case categoryParameter:
		// FR-020: Map parameter.v0 with secret: true to @secure() Bicep parameters.
		if input, ok := resource.Inputs["value"]; ok && input.Secret {
			paramName := sanitizeBicepIdentifier(name)
			description := fmt.Sprintf("Value of parameter %s.", name)
			if input.Description != "" {
				description = input.Description
			}
			file.Parameters = append(file.Parameters, BicepParameter{
				Name:        paramName,
				Type:        "string",
				Secure:      true,
				Description: description,
			})
		}

	case categoryUnsupported:
		// Handle annotated.string with filter: "uri" → BicepVariable with uriComponent().
		if resource.Type == "annotated.string" && resource.Filter == "uri" {
			varName := sanitizeBicepIdentifier(name)
			// Resolve the value expression to find the source Bicep identifier.
			refs := ParseExpressionRefs(resource.Value)
			sourceExpr := varName // fallback
			if len(refs) > 0 {
				sourceExpr = resolveRef(refs[0], manifest, symNameMap)
				// Strip quotes if it's a literal (shouldn't be for a param ref, but be safe).
				sourceExpr = strings.Trim(sourceExpr, "'")
			}

			// Build description from source parameter's description if available.
			description := fmt.Sprintf("URI-encoded value of %s.", sourceExpr)
			if len(refs) > 0 {
				if sourceRes, ok := manifest.Resources[refs[0].ResourceName]; ok && sourceRes.Type == "parameter.v0" {
					if sourceInput, ok := sourceRes.Inputs["value"]; ok && sourceInput.Description != "" {
						// Derive variable description from source parameter description.
						// E.g., "Redis password for the cache container." → "URI-encoded Redis password (for constructing redis:// URIs)."
						srcDesc := sourceInput.Description
						// Extract the noun phrase before any preposition or period.
						idx := strings.Index(srcDesc, " for ")
						if idx > 0 {
							srcDesc = srcDesc[:idx]
						}
						srcDesc = strings.TrimSuffix(srcDesc, ".")
						description = fmt.Sprintf("URI-encoded %s (for constructing redis:// URIs).", srcDesc)
					}
				}
			}

			file.Variables = append(file.Variables, BicepVariable{
				Name:        varName,
				Expression:  fmt.Sprintf("uriComponent(%s)", sourceExpr),
				Description: description,
			})
		}

		file.Comments = append(file.Comments, BicepComment{
			ResourceName: name,
			ResourceType: resource.Type,
			Message:      "manual configuration required",
		})
		file.Warnings = append(file.Warnings, fmt.Sprintf(
			"resource %q (%s): unsupported resource type, adding comment to output",
			name, resource.Type,
		))
	}
}

	return file
}

// sanitizeBicepIdentifier converts a resource name to a valid Bicep identifier.
func sanitizeBicepIdentifier(name string) string {
	result := strings.ReplaceAll(name, "-", "_")

	var sanitized strings.Builder
	for i, c := range result {
if i == 0 {
if isLetter(c) || c == '_' {
sanitized.WriteRune(c)
} else {
sanitized.WriteRune('_')
sanitized.WriteRune(c)
}
} else {
if isLetter(c) || isDigit(c) || c == '_' {
sanitized.WriteRune(c)
} else {
sanitized.WriteRune('_')
}
}
}

return sanitized.String()
}

// uniqueSymbolicName returns a unique symbolic name by appending a suffix
// if the name already exists in the usedNames set.
func uniqueSymbolicName(name string, usedNames map[string]bool) string {
if !usedNames[name] {
return name
}
candidate := name + "Container"
if !usedNames[candidate] {
return candidate
}
for i := 2; ; i++ {
candidate = fmt.Sprintf("%s_%d", name, i)
if !usedNames[candidate] {
return candidate
}
}
}
