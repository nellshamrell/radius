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
	"embed"
	"fmt"
	"sort"
	"strings"
	"text/template"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// templateData holds the data passed to the app.bicep template.
type templateData struct {
	SourceDir string
	Timestamp string
	App       RadiusApplication
}

// GenerateAppBicep generates the app.bicep output string from a RadiusApplication model.
func GenerateAppBicep(app RadiusApplication, sourceDir string, timestamp string) (string, error) {
	funcMap := template.FuncMap{
		"sortedEnvVars": sortedEnvVars,
	}

	tmpl, err := template.New("app.bicep.tmpl").Funcs(funcMap).ParseFS(templateFS, "templates/app.bicep.tmpl")
	if err != nil {
		return "", fmt.Errorf("failed to parse app.bicep template: %w", err)
	}

	data := templateData{
		SourceDir: sourceDir,
		Timestamp: timestamp,
		App:       app,
	}

	var sb strings.Builder
	err = tmpl.Execute(&sb, data)
	if err != nil {
		return "", fmt.Errorf("failed to execute app.bicep template: %w", err)
	}

	return sb.String(), nil
}

// sortedEnvVars returns environment variables as a sorted slice of key-value pairs
// for deterministic template rendering.
func sortedEnvVars(envVars map[string]string) []envVarPair {
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]envVarPair, 0, len(envVars))
	for _, k := range keys {
		pairs = append(pairs, envVarPair{Key: k, Value: envVars[k]})
	}
	return pairs
}

// envVarPair represents a key-value pair for template rendering.
type envVarPair struct {
	Key   string
	Value string
}
