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

// MapToRadius converts an AspireAppDescriptor into a RadiusApplication model.
// It classifies each ServiceTemplate as a service (http transport) or dependency
// (redis=port 6379/tcp, sqlserver=port 1433/tcp) using heuristics, then maps
// containers, dependencies, connections, and parameters.
// The params map supports key=value pairs from the --parameter flag, including
// "image-namespace" to prefix container image defaults and "app-name" to override
// the application name.
func MapToRadius(descriptor *AspireAppDescriptor, params map[string]string) (RadiusApplication, error) {
	app := RadiusApplication{}

	// Derive application name
	app.Name = deriveAppName(descriptor, params["app-name"])

	// Classify templates into services vs. dependencies
	var serviceTemplates []ServiceTemplate
	var dependencyTemplates []ServiceTemplate

	for _, st := range descriptor.ServiceTemplates {
		if classifyAsDependency(st) {
			dependencyTemplates = append(dependencyTemplates, st)
		} else {
			serviceTemplates = append(serviceTemplates, st)
		}
	}

	// Map service templates to RadiusContainers
	for _, st := range serviceTemplates {
		container := mapContainer(st, params)
		app.Containers = append(app.Containers, container)

		// Add image parameter
		app.Parameters = append(app.Parameters, RadiusParameter{
			Name:         container.ImageParam,
			Type:         "string",
			DefaultValue: container.ImageDefault,
			Description:  fmt.Sprintf("Container image for %s.", container.Name),
		})
	}

	// Map dependency templates to RadiusDependencies
	for _, st := range dependencyTemplates {
		dep := mapDependency(st)
		app.Dependencies = append(app.Dependencies, dep)
	}

	// Derive connections from env vars and secrets
	// (ConnectionStrings__ prefix in env, connectionstrings-- prefix in secrets,
	// and services__ prefix in env vars)
	mapConnections(&app, descriptor.ServiceTemplates)

	// Transform env vars: filter Azure-specific vars, replace dependency-related
	// vars with Bicep resource expressions, rewrite service URLs, and fix HTTP_PORTS.
	transformEnvVars(&app, descriptor.ServiceTemplates)

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

// classifyAsDependency determines whether a ServiceTemplate represents a dependency
// (Redis, SQL Server, etc.) rather than a service container. It uses transport and
// port heuristics: tcp transport with port 6379 (Redis) or 1433 (SQL Server), or
// name-based matching.
func classifyAsDependency(st ServiceTemplate) bool {
	name := strings.ToLower(st.ServiceName)

	// Name-based heuristics
	if strings.Contains(name, "redis") || strings.Contains(name, "cache") {
		return true
	}
	if strings.Contains(name, "sqlserver") || strings.Contains(name, "mssql") {
		return true
	}

	// Transport + port heuristics
	if st.Ingress != nil && st.Ingress.Transport == "tcp" {
		switch st.Ingress.TargetPort {
		case 6379, 1433, 3306, 27017:
			return true
		}
	}

	return false
}

// mapContainer maps a service ServiceTemplate to a RadiusContainer with image
// parameter, ports from ingress, and environment variables from template containers.
// When params["image-namespace"] is set, the ImageDefault is prefixed with the
// namespace (e.g., "my-namespace/servicename:latest").
func mapContainer(st ServiceTemplate, params map[string]string) RadiusContainer {
	imageName := st.AzdServiceName
	if imageName == "" {
		imageName = st.ServiceName
	}
	imageDefault := imageName + ":latest"
	if ns, ok := params["image-namespace"]; ok && ns != "" {
		imageDefault = ns + "/" + imageName + ":latest"
	}

	container := RadiusContainer{
		Name:         st.ServiceName,
		ImageParam:   st.ServiceName + "Image",
		ImageDefault: imageDefault,
	}

	// Extract ports from ingress
	if st.Ingress != nil {
		portName := st.Ingress.Transport
		if portName == "" {
			portName = "http"
		}

		container.Ports = append(container.Ports, RadiusPort{
			Name:          portName,
			ContainerPort: st.Ingress.TargetPort,
			Protocol:      "TCP",
		})

		container.IsExternal = st.Ingress.External
	}

	// Extract env vars from first container definition
	if len(st.Containers) > 0 {
		envMap := make(map[string]RadiusEnvVar)
		for _, env := range st.Containers[0].Env {
			if env.Value != "" {
				envMap[env.Name] = RadiusEnvVar{Value: env.Value}
			} else if env.SecretRef != "" {
				envMap[env.Name] = RadiusEnvVar{Value: fmt.Sprintf("{{secret:%s}}", env.SecretRef)}
			}
		}
		if len(envMap) > 0 {
			container.EnvVars = envMap
		}

		container.Command = st.Containers[0].Command
	}

	return container
}

// mapDependency maps a dependency ServiceTemplate to a RadiusDependency. Redis
// templates are mapped to Applications.Datastores/redisCaches with IsRecipeBacked
// true. SQL Server templates are mapped to Applications.Datastores/sqlDatabases
// with IsRecipeBacked true. Other unsupported types are mapped as placeholders.
func mapDependency(st ServiceTemplate) RadiusDependency {
	name := strings.ToLower(st.ServiceName)

	// Redis detection
	if strings.Contains(name, "redis") || strings.Contains(name, "cache") {
		return RadiusDependency{
			Name:           st.ServiceName,
			Type:           "Applications.Datastores/redisCaches",
			IsRecipeBacked: true,
		}
	}

	// SQL Server detection
	if strings.Contains(name, "sqlserver") || strings.Contains(name, "mssql") {
		return RadiusDependency{
			Name:           st.ServiceName,
			Type:           "Applications.Datastores/sqlDatabases",
			IsRecipeBacked: true,
		}
	}

	// Unknown dependency — placeholder
	return RadiusDependency{
		Name:               st.ServiceName,
		IsPlaceholder:      true,
		PlaceholderComment: fmt.Sprintf("PLACEHOLDER: '%s' has no Radius Portable Resource equivalent — manual configuration required", st.ServiceName),
	}
}

// mapConnections derives connections from container environment variables and secrets.
// It identifies ConnectionStrings__ prefixed env vars, connectionstrings-- prefixed
// secrets (the Aspire convention), and services__ prefixed env vars.
func mapConnections(app *RadiusApplication, serviceTemplates []ServiceTemplate) {
	// Build lookup maps for containers and dependencies by name
	depNames := make(map[string]bool)
	for _, d := range app.Dependencies {
		depNames[d.Name] = true
	}

	containerNames := make(map[string]bool)
	for _, c := range app.Containers {
		containerNames[c.Name] = true
	}

	// Build a map of service name → ServiceTemplate for secrets lookup
	stByName := make(map[string]ServiceTemplate)
	for _, st := range serviceTemplates {
		stByName[st.ServiceName] = st
	}

	for i := range app.Containers {
		c := &app.Containers[i]
		connMap := make(map[string]bool)

		// Check env vars for connection patterns
		if c.EnvVars != nil {
			for envName := range c.EnvVars {
				// ConnectionStrings__<name> → connection to dependency or service
				if strings.HasPrefix(envName, "ConnectionStrings__") {
					targetName := strings.TrimPrefix(envName, "ConnectionStrings__")
					targetName = strings.ToLower(targetName)

					resolvedTarget := resolveConnectionTarget(targetName, depNames, containerNames)
					if resolvedTarget != "" && !connMap[resolvedTarget] {
						c.Connections = append(c.Connections, RadiusConnection{
							Name:               resolvedTarget,
							TargetResourceName: resolvedTarget,
							Source:             resolvedTarget + ".id",
						})
						connMap[resolvedTarget] = true
					}
				}

				// services__<name>__<protocol>__<index> → connection to service container
				if strings.HasPrefix(envName, "services__") {
					parts := strings.SplitN(envName, "__", 4)
					if len(parts) >= 2 {
						targetName := parts[1]
						if containerNames[targetName] && !connMap[targetName] {
							c.Connections = append(c.Connections, RadiusConnection{
								Name:               targetName,
								TargetResourceName: targetName,
								Source:             targetName + ".id",
							})
							connMap[targetName] = true
						}
					}
				}
			}
		}

		// Check secrets for connectionstrings-- prefix (Aspire convention)
		// Secret names like "connectionstrings--weatherdb" map to ConnectionStrings__weatherdb
		if st, ok := stByName[c.Name]; ok {
			for _, secret := range st.Secrets {
				secretName := strings.ToLower(secret.Name)
				if strings.HasPrefix(secretName, "connectionstrings--") {
					targetName := strings.TrimPrefix(secretName, "connectionstrings--")

					resolvedTarget := resolveConnectionTarget(targetName, depNames, containerNames)
					if resolvedTarget != "" && !connMap[resolvedTarget] {
						c.Connections = append(c.Connections, RadiusConnection{
							Name:               resolvedTarget,
							TargetResourceName: resolvedTarget,
							Source:             resolvedTarget + ".id",
						})
						connMap[resolvedTarget] = true
					}
				}
			}
		}

		// Sort connections for determinism
		sort.Slice(c.Connections, func(a, b int) bool {
			return c.Connections[a].Name < c.Connections[b].Name
		})
	}
}

// transformEnvVars post-processes container environment variables to produce Bicep-native
// resource expressions. It filters Azure-specific vars, converts dependency-related vars
// (host, port, password, etc.) to Bicep property references, rewrites service discovery
// URLs, and fixes HTTP_PORTS to use the actual ingress port.
func transformEnvVars(app *RadiusApplication, serviceTemplates []ServiceTemplate) {
	// Build dependency lookup: name → RadiusDependency
	depByName := make(map[string]*RadiusDependency)
	for i := range app.Dependencies {
		depByName[app.Dependencies[i].Name] = &app.Dependencies[i]
	}

	// Build container port lookup: name → first port number
	containerPorts := make(map[string]int)
	for _, c := range app.Containers {
		if len(c.Ports) > 0 {
			containerPorts[c.Name] = c.Ports[0].ContainerPort
		}
	}

	// Build ServiceTemplate lookup by service name
	stByName := make(map[string]ServiceTemplate)
	for _, st := range serviceTemplates {
		stByName[st.ServiceName] = st
	}

	// Define property mappings per dependency type
	type propMapping struct {
		suffix string
		expr   func(depName string) string
	}
	sqlMappings := []propMapping{
		{"_HOST", func(d string) string { return d + ".properties.server" }},
		{"_PORT", func(d string) string { return "string(" + d + ".properties.port)" }},
		{"_PASSWORD", func(d string) string { return d + ".listSecrets().password" }},
		{"_USERNAME", func(d string) string { return d + ".properties.username" }},
		{"_DATABASENAME", func(d string) string { return d + ".properties.database" }},
	}
	redisMappings := []propMapping{
		{"_HOST", func(d string) string { return d + ".properties.host" }},
		{"_PORT", func(d string) string { return "string(" + d + ".properties.port)" }},
	}

	for i := range app.Containers {
		c := &app.Containers[i]
		if c.EnvVars == nil {
			continue
		}

		st := stByName[c.Name]

		// Build secretRef lookup from original ServiceTemplate
		secretRefVars := make(map[string]string) // envName → secretRef name
		if len(st.Containers) > 0 {
			for _, env := range st.Containers[0].Env {
				if env.SecretRef != "" {
					secretRefVars[env.Name] = env.SecretRef
				}
			}
		}

		// Build prefix → dependency association by matching X_HOST values to dep names
		prefixToDep := make(map[string]*RadiusDependency)
		for envName, envVar := range c.EnvVars {
			if strings.HasSuffix(envName, "_HOST") {
				prefix := strings.TrimSuffix(envName, "_HOST")
				depName := strings.ToLower(envVar.Value)
				if dep, ok := depByName[depName]; ok {
					prefixToDep[prefix] = dep
				}
			}
		}

		// Get this container's ingress port for HTTP_PORTS fix
		containerPort := 0
		if len(c.Ports) > 0 {
			containerPort = c.Ports[0].ContainerPort
		}

		newEnvVars := make(map[string]RadiusEnvVar)
		for envName, envVar := range c.EnvVars {
			// Filter: skip env vars with empty values
			if envVar.Value == "" {
				continue
			}

			// Filter: skip Azure-specific env vars
			if strings.HasPrefix(envName, "AZURE_") {
				continue
			}

			// Transform: ConnectionStrings__X → dependency.listSecrets().connectionString
			if strings.HasPrefix(envName, "ConnectionStrings__") {
				targetName := strings.TrimPrefix(envName, "ConnectionStrings__")
				resolvedDep := resolveDependencyFromTarget(strings.ToLower(targetName), depByName)
				if resolvedDep != nil {
					newEnvVars[envName] = RadiusEnvVar{
						Value:        resolvedDep.Name + ".listSecrets().connectionString",
						IsExpression: true,
					}
					continue
				}
			}

			// Transform: HTTP_PORTS with value "0" → use actual container port
			if envName == "HTTP_PORTS" && envVar.Value == "0" && containerPort > 0 {
				newEnvVars[envName] = RadiusEnvVar{Value: fmt.Sprintf("%d", containerPort)}
				continue
			}

			// Transform: services__X__proto__N → rewritten URL with container port
			// (must come before .internal. filter since original values contain ACA domains)
			if strings.HasPrefix(envName, "services__") {
				parts := strings.SplitN(envName, "__", 4)
				if len(parts) >= 3 {
					targetName := parts[1]
					proto := parts[2]
					if port, ok := containerPorts[targetName]; ok {
						newEnvVars[envName] = RadiusEnvVar{
							Value: fmt.Sprintf("%s://%s:%d", proto, targetName, port),
						}
						continue
					}
				}
			}

			// Filter: skip values containing ".internal." (Azure Container Apps domain)
			if strings.Contains(envVar.Value, ".internal.") {
				continue
			}

			// Filter: skip URI/connection string patterns (contain "://")
			if strings.Contains(envVar.Value, "://") {
				continue
			}

			// Filter: skip JDBC connection strings
			if strings.HasSuffix(envName, "_JDBCCONNECTIONSTRING") {
				continue
			}

			// Transform: dependency-related env vars → Bicep expressions
			transformed := false
			for prefix, dep := range prefixToDep {
				var mappings []propMapping
				if strings.Contains(dep.Type, "sqlDatabases") {
					mappings = sqlMappings
				} else if strings.Contains(dep.Type, "redisCaches") {
					mappings = redisMappings
				}

				for _, m := range mappings {
					if envName == prefix+m.suffix {
						newEnvVars[envName] = RadiusEnvVar{
							Value:        m.expr(dep.Name),
							IsExpression: true,
						}
						transformed = true
						break
					}
				}
				if transformed {
					break
				}
			}
			if transformed {
				continue
			}

			// Filter: skip remaining secret-backed vars that weren't transformed above
			// (e.g., X_PASSWORD for Redis, X_URI secrets)
			if _, isSecret := secretRefVars[envName]; isSecret {
				continue
			}

			// Keep as literal value
			newEnvVars[envName] = envVar
		}

		c.EnvVars = newEnvVars
	}
}

// resolveDependencyFromTarget finds a dependency matching the target name,
// supporting both direct matches and heuristic matching (e.g., "weatherdb" → sqlserver).
func resolveDependencyFromTarget(targetName string, depByName map[string]*RadiusDependency) *RadiusDependency {
	// Direct match
	if dep, ok := depByName[targetName]; ok {
		return dep
	}

	// Heuristic: targets containing "db" or "database" → match SQL-like deps
	if strings.Contains(targetName, "db") || strings.Contains(targetName, "database") {
		for _, dep := range depByName {
			if strings.Contains(strings.ToLower(dep.Name), "sqlserver") ||
				strings.Contains(strings.ToLower(dep.Name), "mssql") ||
				strings.Contains(strings.ToLower(dep.Name), "mysql") {
				return dep
			}
		}
	}

	return nil
}

// resolveConnectionTarget attempts to match a connection string target name
// against known dependencies and containers. It handles partial matches like
// "weatherdb" by looking for sqlserver/mysql-like dependencies.
func resolveConnectionTarget(targetName string, depNames map[string]bool, containerNames map[string]bool) string {
	// Direct match against dependencies
	if depNames[targetName] {
		return targetName
	}

	// Direct match against containers
	if containerNames[targetName] {
		return targetName
	}

	// For targets like "weatherdb", try to match against sqlserver/mysql-like deps.
	lower := strings.ToLower(targetName)
	if strings.Contains(lower, "db") || strings.Contains(lower, "database") {
		for name := range depNames {
			if strings.Contains(strings.ToLower(name), "sqlserver") ||
				strings.Contains(strings.ToLower(name), "mssql") ||
				strings.Contains(strings.ToLower(name), "mysql") {
				return name
			}
		}
	}

	return ""
}

// deriveAppName determines the application name from available sources.
func deriveAppName(descriptor *AspireAppDescriptor, appNameOverride string) string {
	if appNameOverride != "" {
		return appNameOverride
	}

	// Try to derive from main.bicep environmentName parameter
	if descriptor.MainBicep != nil {
		for _, param := range descriptor.MainBicep.Parameters {
			if param.Name == "environmentName" && param.DefaultValue != "" {
				return strings.Trim(param.DefaultValue, "'\"")
			}
		}
	}

	// Fall back to directory name
	dirName := filepath.Base(descriptor.RootDir)
	if dirName == "infra" || dirName == "." {
		dirName = filepath.Base(filepath.Dir(descriptor.RootDir))
	}

	dirName = strings.ToLower(dirName)
	dirName = strings.ReplaceAll(dirName, " ", "-")
	dirName = strings.ReplaceAll(dirName, "_", "-")

	if dirName == "" || dirName == "." {
		return "aspire-app"
	}

	return dirName
}
