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
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/template"
)

const apiVersion = "2023-10-01-preview"

// bicepTemplateText is the Bicep template for the generated app.bicep file.
const bicepTemplateText = `extension radius

@description('The Radius environment ID')
param environment string = '{{ .EnvironmentName }}'

@description('The Radius application name')
param application string = '{{ .AppName }}'
{{ range .Parameters }}
{{ if .Secure }}@secure()
{{ end }}@description('{{ .Description }}')
param {{ .Name }} string{{ if .DefaultValue }} = '{{ .DefaultValue }}'{{ end }}
{{ end }}
resource app 'Applications.Core/applications@{{ .APIVersion }}' = {
  name: application
  properties: {
    environment: environment
  }
}
{{ range .PortableResources }}
resource {{ .BicepIdentifier }} '{{ .RadiusType }}@{{ $.APIVersion }}' = {
  name: '{{ .RuntimeName }}'
  properties: {
    application: app.id
    environment: environment
    resourceProvisioning: 'recipe'
    recipe: {
      name: '{{ .PortableResource.RecipeName }}'
    }
  }
}
{{ end }}{{ range .Containers }}
resource {{ .BicepIdentifier }} 'Applications.Core/containers@{{ $.APIVersion }}' = {
  name: '{{ .RuntimeName }}'
  properties: {
    application: app.id
    environment: environment
    container: {
      image: '{{ .Container.Image }}'{{ if .Container.Command }}
      command: [{{ range $i, $c := .Container.Command }}{{ if $i }}, {{ end }}'{{ $c }}'{{ end }}]{{ end }}{{ if .Container.Args }}
      args: [{{ range $i, $a := .Container.Args }}{{ if $i }}, {{ end }}'{{ $a }}'{{ end }}]{{ end }}{{ if .Container.Ports }}
      ports: {{ portBlock .Container.Ports }}{{ end }}{{ if .Container.Env }}
      env: {{ envBlock .Container.Env }}{{ end }}{{ if .Container.Volumes }}
      volumes: {{ volumeBlock .Container.Volumes }}{{ end }}
    }{{ if .Connections }}
    connections: {{ connectionBlock .Connections }}{{ end }}
  }
}
{{ end }}{{ range .Gateways }}
resource {{ .BicepIdentifier }} 'Applications.Core/gateways@{{ $.APIVersion }}' = {
  name: '{{ .RuntimeName }}'
  properties: {
    application: app.id
    routes: [{{ range .Gateway.Routes }}
      {
        path: '{{ .Path }}'
        destination: '{{ .Destination }}'
      }{{ end }}
    ]
  }
}
{{ end }}`

// bicepData is the data passed to the Bicep template.
type bicepData struct {
	EnvironmentName   string
	AppName           string
	APIVersion        string
	Parameters        []BicepParameter
	PortableResources []*RadiusResource
	Containers        []*RadiusResource
	Gateways          []*RadiusResource
}

// emit renders the Bicep template from the translation context.
func emit(ctx *translationContext) (string, error) {
	data := prepareBicepData(ctx)

	funcMap := template.FuncMap{
		"portBlock":       renderPortBlock,
		"envBlock":        renderEnvBlock,
		"volumeBlock":     renderVolumeBlock,
		"connectionBlock": renderConnectionBlock,
	}

	tmpl, err := template.New("bicep").Funcs(funcMap).Parse(bicepTemplateText)
	if err != nil {
		return "", fmt.Errorf("failed to render Bicep template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render Bicep template: %w", err)
	}

	// Clean up extra blank lines (normalize to max 1 blank line between sections).
	result := normalizeBlankLines(buf.String())

	return result, nil
}

// prepareBicepData extracts and sorts resources from the translation context into bicepData.
func prepareBicepData(ctx *translationContext) *bicepData {
	data := &bicepData{
		EnvironmentName: ctx.config.environmentName,
		AppName:         ctx.config.appName,
		APIVersion:      apiVersion,
		Parameters:      ctx.parameters,
	}

	// Sort parameters by name.
	sort.Slice(data.Parameters, func(i, j int) bool {
		return data.Parameters[i].Name < data.Parameters[j].Name
	})

	// Collect and sort resources by kind.
	var portableResources, containers, gateways []*RadiusResource
	for _, resource := range ctx.resources {
		switch {
		case resource.Kind.IsPortableResource():
			portableResources = append(portableResources, resource)
		case resource.Kind == KindContainer:
			containers = append(containers, resource)
		case resource.Kind == KindGateway:
			gateways = append(gateways, resource)
		}
	}

	// Sort each group alphabetically by Bicep identifier.
	sort.Slice(portableResources, func(i, j int) bool {
		return portableResources[i].BicepIdentifier < portableResources[j].BicepIdentifier
	})
	sort.Slice(containers, func(i, j int) bool {
		return containers[i].BicepIdentifier < containers[j].BicepIdentifier
	})
	sort.Slice(gateways, func(i, j int) bool {
		return gateways[i].BicepIdentifier < gateways[j].BicepIdentifier
	})

	data.PortableResources = portableResources
	data.Containers = containers
	data.Gateways = gateways

	return data
}

// renderPortBlock renders a Bicep ports block.
func renderPortBlock(ports map[string]PortSpec) string {
	if len(ports) == 0 {
		return "{}"
	}

	var buf bytes.Buffer
	buf.WriteString("{\n")

	keys := sortedKeys(ports)
	for _, name := range keys {
		port := ports[name]
		buf.WriteString(fmt.Sprintf("        %s: {\n", name))
		buf.WriteString(fmt.Sprintf("          containerPort: %d\n", port.ContainerPort))
		if port.Protocol != "" && port.Protocol != "TCP" {
			buf.WriteString(fmt.Sprintf("          protocol: '%s'\n", port.Protocol))
		}
		if port.Scheme != "" {
			buf.WriteString(fmt.Sprintf("          scheme: '%s'\n", port.Scheme))
		}
		buf.WriteString("        }\n")
	}

	buf.WriteString("      }")

	return buf.String()
}

// renderEnvBlock renders a Bicep env block.
func renderEnvBlock(env map[string]EnvVarSpec) string {
	if len(env) == 0 {
		return "{}"
	}

	var buf bytes.Buffer
	buf.WriteString("{\n")

	keys := sortedKeysEnv(env)
	for _, name := range keys {
		spec := env[name]
		if spec.IsBicepInterpolation {
			buf.WriteString(fmt.Sprintf("        %s: {\n", name))
			buf.WriteString(fmt.Sprintf("          value: '%s'\n", spec.Value))
			buf.WriteString("        }\n")
		} else {
			buf.WriteString(fmt.Sprintf("        %s: {\n", name))
			buf.WriteString(fmt.Sprintf("          value: '%s'\n", spec.Value))
			buf.WriteString("        }\n")
		}
	}

	buf.WriteString("      }")

	return buf.String()
}

// renderVolumeBlock renders a Bicep volumes block.
func renderVolumeBlock(volumes map[string]VolumeSpec) string {
	if len(volumes) == 0 {
		return "{}"
	}

	var buf bytes.Buffer
	buf.WriteString("{\n")

	keys := sortedKeysVolume(volumes)
	for _, name := range keys {
		vol := volumes[name]
		buf.WriteString(fmt.Sprintf("        %s: {\n", name))
		buf.WriteString(fmt.Sprintf("          kind: '%s'\n", vol.Kind))
		buf.WriteString(fmt.Sprintf("          mountPath: '%s'\n", vol.MountPath))
		if vol.ReadOnly {
			buf.WriteString("          readOnly: true\n")
		}
		buf.WriteString("        }\n")
	}

	buf.WriteString("      }")

	return buf.String()
}

// renderConnectionBlock renders a Bicep connections block.
func renderConnectionBlock(connections map[string]ConnectionSpec) string {
	if len(connections) == 0 {
		return "{}"
	}

	var buf bytes.Buffer
	buf.WriteString("{\n")

	keys := sortedKeysConn(connections)
	for _, name := range keys {
		conn := connections[name]
		buf.WriteString(fmt.Sprintf("      %s: {\n", name))
		if conn.IsBicepReference {
			buf.WriteString(fmt.Sprintf("        source: %s\n", conn.Source))
		} else {
			buf.WriteString(fmt.Sprintf("        source: '%s'\n", conn.Source))
		}
		buf.WriteString("      }\n")
	}

	buf.WriteString("    }")

	return buf.String()
}

// normalizeBlankLines reduces multiple consecutive blank lines to a single blank line.
func normalizeBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	var result []string
	blankCount := 0

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			blankCount++
			if blankCount <= 1 {
				result = append(result, line)
			}
		} else {
			blankCount = 0
			result = append(result, line)
		}
	}

	// Trim trailing blank lines but keep a final newline.
	for len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
		result = result[:len(result)-1]
	}

	return strings.Join(result, "\n") + "\n"
}

// Helper functions for sorted map key iteration.

func sortedKeys(m map[string]PortSpec) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

func sortedKeysEnv(m map[string]EnvVarSpec) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

func sortedKeysVolume(m map[string]VolumeSpec) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

func sortedKeysConn(m map[string]ConnectionSpec) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}
