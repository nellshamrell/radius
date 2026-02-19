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
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"time"
)

// BicepFile is the complete Bicep file intermediate representation to be emitted.
type BicepFile struct {
	// Extensions is the list of required Bicep extension names (e.g., radius, containers).
	Extensions []string

	// Parameters is the list of declared Bicep parameters.
	Parameters []BicepParameter

	// Application is the Radius application resource.
	Application BicepResource

	// Containers is the list of container resources.
	Containers []BicepContainer

	// DataStores is the list of data-store resources (Redis, PostgreSQL, MySQL).
	DataStores []BicepResource

	// Gateways is the list of gateway/route resources for external bindings.
	Gateways []BicepGateway

	// Comments is the list of comments for unsupported/skipped resources.
	Comments []BicepComment

	// Warnings is the list of warning messages to display to the user (not in the Bicep file).
	Warnings []string
}

// BicepParameter is a Bicep parameter declaration.
type BicepParameter struct {
	// Name is the parameter name.
	Name string

	// Type is the Bicep type (e.g., string).
	Type string

	// Secure indicates whether to emit the @secure() decorator.
	Secure bool

	// Description is the parameter description for @description() decorator.
	Description string

	// DefaultValue is an optional default value for the parameter.
	DefaultValue string
}

// BicepResource is a generic Bicep resource declaration.
type BicepResource struct {
	// SymbolicName is the Bicep symbolic name (identifier used in Bicep code).
	SymbolicName string

	// TypeName is the full resource type (e.g., Radius.Core/applications@2025-08-01-preview).
	TypeName string

	// Name is the resource name in Radius.
	Name string

	// Properties is a map of resource properties.
	Properties map[string]any
}

// BicepContainer is a Radius container resource with full container-specific properties.
type BicepContainer struct {
	// SymbolicName is the Bicep symbolic name.
	SymbolicName string

	// TypeName is the full resource type.
	TypeName string

	// Name is the container name.
	Name string

	// Image is the container image reference.
	Image string

	// Ports is a map of port definitions.
	Ports map[string]BicepPort

	// Env is a map of environment variables.
	Env map[string]BicepEnvVar

	// Command is the entrypoint + args if specified.
	Command []string

	// Connections is a map of Radius connections to other resources.
	Connections map[string]BicepConnection

	// ApplicationRef is the Bicep reference to the application resource.
	ApplicationRef string

	// EnvironmentRef is the Bicep reference to the environment parameter.
	EnvironmentRef string

	// NeedsBuildWarning indicates whether to emit a build image warning comment.
	NeedsBuildWarning bool

	// BuildContext is the original Aspire build context path.
	BuildContext string
}

// BicepPort is a container port definition.
type BicepPort struct {
	// ContainerPort is the target port number.
	ContainerPort int

	// Protocol is the protocol (e.g., TCP).
	Protocol string
}

// BicepEnvVar is an environment variable in a container.
type BicepEnvVar struct {
	// Value is a static value (if no reference).
	Value string

	// BicepExpression is a Bicep expression (if resolved from an Aspire reference).
	// Only one of Value or BicepExpression is set.
	BicepExpression string
}

// BicepConnection is a Radius connection to another resource.
type BicepConnection struct {
	// Source is the Bicep reference to the target resource (e.g., cache.id).
	Source string
}

// BicepGateway is a gateway/route resource for external bindings.
type BicepGateway struct {
	// SymbolicName is the Bicep symbolic name.
	SymbolicName string

	// TypeName is the full resource type.
	TypeName string

	// Name is the gateway name.
	Name string

	// ContainerRef is the Bicep reference to the container resource.
	ContainerRef string

	// Routes is the list of route definitions.
	Routes []BicepGatewayRoute

	// ApplicationRef is the Bicep reference to the application.
	ApplicationRef string

	// EnvironmentRef is the Bicep reference to the environment.
	EnvironmentRef string
}

// BicepGatewayRoute is a single route within a gateway.
type BicepGatewayRoute struct {
	// Path is the URL path (default /).
	Path string

	// Port is the target port on the container.
	Port int
}

// BicepComment is a comment block in the generated Bicep for skipped/unsupported resources.
type BicepComment struct {
	// ResourceName is the Aspire resource name that was skipped.
	ResourceName string

	// ResourceType is the Aspire resource type that was unsupported.
	ResourceType string

	// Message is a human-readable explanation.
	Message string
}

// sortedKeys returns the keys of a map in sorted order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// bicepTemplateFuncs provides helper functions for the Bicep templates.
var bicepTemplateFuncs = template.FuncMap{
	"sortedKeys": func(m any) []string {
		switch v := m.(type) {
		case map[string]BicepPort:
			return sortedKeys(v)
		case map[string]BicepEnvVar:
			return sortedKeys(v)
		case map[string]BicepConnection:
			return sortedKeys(v)
		default:
			return nil
		}
	},
	"sortedPropertyKeys": func(m map[string]any) []string {
		return sortedKeys(m)
	},
	"indent": func(spaces int, s string) string {
		pad := strings.Repeat(" ", spaces)
		lines := strings.Split(s, "\n")
		for i, line := range lines {
			if line != "" {
				lines[i] = pad + line
			}
		}
		return strings.Join(lines, "\n")
	},
	"formatPropertyValue": func(v any) string {
		switch val := v.(type) {
		case string:
			return fmt.Sprintf("'%s'", val)
		case bool:
			if val {
				return "true"
			}
			return "false"
		case int:
			return fmt.Sprintf("%d", val)
		case float64:
			return fmt.Sprintf("%g", val)
		default:
			return fmt.Sprintf("'%v'", val)
		}
	},
	"bicepValue": func(ev BicepEnvVar) string {
		if ev.BicepExpression != "" {
			return ev.BicepExpression
		}
		return fmt.Sprintf("'%s'", ev.Value)
	},
}

const bicepMainTemplate = `// Generated by: rad aspire convert
// Source: {{.SourceFile}}
// Date: {{.Timestamp}}
{{range .File.Extensions}}
extension {{.}}
{{- end}}
{{range .File.Parameters}}
{{- if .Secure}}

@secure()
{{- end}}
@description('{{.Description}}')
param {{.Name}} {{.Type}}{{if .DefaultValue}} = '{{.DefaultValue}}'{{end}}
{{- end}}

resource {{.File.Application.SymbolicName}} '{{.File.Application.TypeName}}' = {
  name: {{formatAppName .File.Application.Name}}
  properties: {
{{- range $key := sortedPropertyKeys .File.Application.Properties}}
    {{$key}}: {{formatPropertyValue (index $.File.Application.Properties $key)}}
{{- end}}
  }
}
{{range .File.Containers}}
{{- if .NeedsBuildWarning}}
// WARNING: {{.Name}} (container.v1) has a build configuration (context: '{{.BuildContext}}').
// Build and push the container image before deploying. For example:
//   docker build -t <registry>/{{.Name}}:latest {{.BuildContext}}
//   docker push <registry>/{{.Name}}:latest
// Then update the 'image' property below with the pushed image reference.
{{- end}}
resource {{.SymbolicName}} '{{.TypeName}}' = {
  name: '{{.Name}}'
  properties: {
    application: {{.ApplicationRef}}
    environment: {{.EnvironmentRef}}
    container: {
      image: '{{.Image}}'
{{- if .Command}}
      command: [
{{- range .Command}}
        '{{.}}'
{{- end}}
      ]
{{- end}}
{{- if .Ports}}
      ports: {
{{- range $key := sortedKeys .Ports}}
{{- $port := index $.Ports $key}}
        {{$key}}: {
          containerPort: {{$port.ContainerPort}}
          protocol: '{{$port.Protocol}}'
        }
{{- end}}
      }
{{- end}}
{{- if .Env}}
      env: {
{{- range $key := sortedKeys .Env}}
{{- $env := index $.Env $key}}
        {{$key}}: {{bicepValue $env}}
{{- end}}
      }
{{- end}}
    }
{{- if .Connections}}
    connections: {
{{- range $key := sortedKeys .Connections}}
{{- $conn := index $.Connections $key}}
      {{$key}}: {
        source: {{$conn.Source}}
      }
{{- end}}
    }
{{- end}}
  }
}
{{end}}
{{- range .File.DataStores}}
resource {{.SymbolicName}} '{{.TypeName}}' = {
  name: '{{.Name}}'
  properties: {
{{- range $key := sortedPropertyKeys .Properties}}
    {{$key}}: {{formatPropertyValue (index $.Properties $key)}}
{{- end}}
  }
}
{{end}}
{{- range .File.Gateways}}
resource {{.SymbolicName}} '{{.TypeName}}' = {
  name: '{{.Name}}'
  properties: {
    application: {{.ApplicationRef}}
    environment: {{.EnvironmentRef}}
    routes: [
{{- range .Routes}}
      {
        path: '{{.Path}}'
        destination: {{$.ContainerRef}}
        port: {{.Port}}
      }
{{- end}}
    ]
  }
}
{{end}}
{{- range .File.Comments}}
// Unsupported: {{.ResourceName}} ({{.ResourceType}}) — {{.Message}}
{{- end}}
`

// emitTemplateData holds all data passed to the Bicep template.
type emitTemplateData struct {
	File       *BicepFile
	SourceFile string
	Timestamp  string
}

// Emit renders the BicepFile intermediate representation to a Bicep text string.
func Emit(file *BicepFile, sourceFile string) (string, error) {
	funcMap := template.FuncMap{}
	for k, v := range bicepTemplateFuncs {
		funcMap[k] = v
	}
	funcMap["formatAppName"] = func(name string) string {
		// If the name looks like a Bicep expression (e.g., a parameter reference),
		// emit it without quotes. Otherwise, wrap in quotes.
		if strings.Contains(name, ".") || !strings.ContainsAny(name, " -") && isValidBicepIdentifier(name) {
			return name
		}
		return fmt.Sprintf("'%s'", name)
	}

	tmpl, err := template.New("bicep").Funcs(funcMap).Parse(bicepMainTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse Bicep template: %w", err)
	}

	data := emitTemplateData{
		File:       file,
		SourceFile: sourceFile,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render Bicep template: %w", err)
	}

	// Clean up excessive blank lines (3+ consecutive newlines → 2).
	result := cleanBlankLines(buf.String())

	return result, nil
}

// isValidBicepIdentifier checks whether the given string is a valid Bicep identifier.
func isValidBicepIdentifier(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i, c := range s {
		if i == 0 {
			if !isLetter(c) && c != '_' {
				return false
			}
		} else {
			if !isLetter(c) && !isDigit(c) && c != '_' {
				return false
			}
		}
	}
	return true
}

func isLetter(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isDigit(c rune) bool {
	return c >= '0' && c <= '9'
}

// cleanBlankLines reduces 3+ consecutive blank lines to 2 (at most one blank line between sections).
func cleanBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	var result []string
	blankCount := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			blankCount++
			if blankCount <= 2 {
				result = append(result, line)
			}
		} else {
			blankCount = 0
			result = append(result, line)
		}
	}

	// Trim trailing whitespace.
	output := strings.Join(result, "\n")
	output = strings.TrimRight(output, "\n") + "\n"
	return output
}
