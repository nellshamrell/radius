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
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ScanDirectory discovers all .bicep files in the given directory in lexicographic path order.
func ScanDirectory(dirPath string) ([]string, error) {
	info, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("cannot access directory '%s': %w", dirPath, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("'%s' is not a directory", dirPath)
	}

	var files []string
	err = filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".bicep") {
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error scanning directory '%s': %w", dirPath, err)
	}

	return files, nil
}

// ParseFile reads a Bicep file and extracts declarations (resources, parameters, modules, variables).
func ParseFile(filePath string) (BicepFile, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return BicepFile{}, fmt.Errorf("cannot read file '%s': %w", filePath, err)
	}

	text := string(content)
	bf := BicepFile{
		Path: filePath,
	}

	bf.Resources = parseBicepResources(text, filePath)
	bf.Parameters = parseBicepParameters(text, filePath)
	bf.Modules = parseBicepModules(text)
	bf.Variables = parseBicepVariables(text)

	return bf, nil
}

// --- Resource Parsing ---

var resourceDeclRegex = regexp.MustCompile(`(?m)^resource\s+(\w+)\s+'([^']+)'\s*=\s*\{`)

// parseBicepResources extracts all resource declarations from Bicep text.
func parseBicepResources(text string, sourceFile string) []BicepResource {
	var resources []BicepResource
	matches := resourceDeclRegex.FindAllStringSubmatchIndex(text, -1)

	for _, loc := range matches {
		symbolicName := text[loc[2]:loc[3]]
		resourceType := text[loc[4]:loc[5]]
		startLine := countLines(text[:loc[0]])

		// Find the matching closing brace for the resource body
		bodyStart := loc[1] - 1 // position of the opening brace
		bodyEnd := findMatchingBrace(text, bodyStart)
		if bodyEnd < 0 {
			continue
		}

		body := text[bodyStart : bodyEnd+1]
		props := extractResourceProperties(body)

		// Extract the name expression
		nameExpr := extractSimpleProperty(body, "name")

		resources = append(resources, BicepResource{
			SymbolicName: symbolicName,
			Type:         resourceType,
			Name:         nameExpr,
			Properties:   props,
			SourceFile:   sourceFile,
			StartLine:    startLine,
		})
	}

	return resources
}

// extractResourceProperties extracts nested properties from a resource body into a map.
func extractResourceProperties(body string) map[string]any {
	props := make(map[string]any)

	// Extract ingress block
	if ingress := extractBlock(body, "ingress"); ingress != "" {
		ingressProps := make(map[string]any)

		if tp := extractSimpleValue(ingress, "targetPort"); tp != "" {
			if port, err := strconv.Atoi(tp); err == nil {
				ingressProps["targetPort"] = port
			}
		}

		if ext := extractSimpleValue(ingress, "external"); ext != "" {
			ingressProps["external"] = ext == "true"
		}

		if transport := extractSimpleValue(ingress, "transport"); transport != "" {
			ingressProps["transport"] = strings.Trim(transport, "'\"")
		}

		if len(ingressProps) > 0 {
			props["ingress"] = ingressProps
		}
	}

	// Extract template.containers array
	if containers := extractContainers(body); len(containers) > 0 {
		props["containers"] = containers
	}

	// Extract secrets
	if secrets := extractSecrets(body); len(secrets) > 0 {
		props["secrets"] = secrets
	}

	return props
}

// extractContainers extracts the containers array from a template block.
func extractContainers(body string) []map[string]any {
	// Find the containers array inside template
	templateBlock := extractBlock(body, "template")
	if templateBlock == "" {
		// Try to find containers directly in the body
		templateBlock = body
	}

	containersStart := strings.Index(templateBlock, "containers:")
	if containersStart < 0 {
		return nil
	}

	// Find the opening bracket
	bracketStart := strings.Index(templateBlock[containersStart:], "[")
	if bracketStart < 0 {
		return nil
	}
	bracketStart += containersStart

	bracketEnd := findMatchingBracket(templateBlock, bracketStart)
	if bracketEnd < 0 {
		return nil
	}

	containersContent := templateBlock[bracketStart+1 : bracketEnd]

	// Extract individual container objects
	var containers []map[string]any
	pos := 0
	for pos < len(containersContent) {
		braceStart := strings.Index(containersContent[pos:], "{")
		if braceStart < 0 {
			break
		}
		braceStart += pos

		braceEnd := findMatchingBrace(containersContent, braceStart)
		if braceEnd < 0 {
			break
		}

		containerBody := containersContent[braceStart : braceEnd+1]
		container := make(map[string]any)

		if name := extractSimpleValue(containerBody, "name"); name != "" {
			container["name"] = strings.Trim(name, "'\"")
		}

		if image := extractSimpleValue(containerBody, "image"); image != "" {
			container["image"] = strings.Trim(image, "'\"")
		}

		// Extract env array
		if envVars := extractEnvVars(containerBody); len(envVars) > 0 {
			container["env"] = envVars
		}

		containers = append(containers, container)
		pos = braceEnd + 1
	}

	return containers
}

// extractEnvVars extracts env variables from a container body.
func extractEnvVars(containerBody string) []map[string]string {
	envStart := strings.Index(containerBody, "env:")
	if envStart < 0 {
		return nil
	}

	bracketStart := strings.Index(containerBody[envStart:], "[")
	if bracketStart < 0 {
		return nil
	}
	bracketStart += envStart

	bracketEnd := findMatchingBracket(containerBody, bracketStart)
	if bracketEnd < 0 {
		return nil
	}

	envContent := containerBody[bracketStart+1 : bracketEnd]

	var envVars []map[string]string
	pos := 0
	for pos < len(envContent) {
		braceStart := strings.Index(envContent[pos:], "{")
		if braceStart < 0 {
			break
		}
		braceStart += pos

		braceEnd := findMatchingBrace(envContent, braceStart)
		if braceEnd < 0 {
			break
		}

		envBody := envContent[braceStart : braceEnd+1]
		envVar := make(map[string]string)

		if name := extractSimpleValue(envBody, "name"); name != "" {
			envVar["name"] = strings.Trim(name, "'\"")
		}

		if value := extractSimpleValue(envBody, "value"); value != "" {
			envVar["value"] = strings.Trim(value, "'\"")
		}

		if secretRef := extractSimpleValue(envBody, "secretRef"); secretRef != "" {
			envVar["secretRef"] = strings.Trim(secretRef, "'\"")
		}

		if len(envVar) > 0 {
			envVars = append(envVars, envVar)
		}

		pos = braceEnd + 1
	}

	return envVars
}

// extractSecrets extracts the secrets array from a configuration block.
func extractSecrets(body string) []map[string]string {
	secretsStart := strings.Index(body, "secrets:")
	if secretsStart < 0 {
		return nil
	}

	bracketStart := strings.Index(body[secretsStart:], "[")
	if bracketStart < 0 {
		return nil
	}
	bracketStart += secretsStart

	bracketEnd := findMatchingBracket(body, bracketStart)
	if bracketEnd < 0 {
		return nil
	}

	secretsContent := body[bracketStart+1 : bracketEnd]

	var secrets []map[string]string
	pos := 0
	for pos < len(secretsContent) {
		braceStart := strings.Index(secretsContent[pos:], "{")
		if braceStart < 0 {
			break
		}
		braceStart += pos

		braceEnd := findMatchingBrace(secretsContent, braceStart)
		if braceEnd < 0 {
			break
		}

		secretBody := secretsContent[braceStart : braceEnd+1]
		secret := make(map[string]string)

		if name := extractSimpleValue(secretBody, "name"); name != "" {
			secret["name"] = strings.Trim(name, "'\"")
		}

		if value := extractSimpleValue(secretBody, "value"); value != "" {
			secret["value"] = strings.Trim(value, "'\"")
		}

		if len(secret) > 0 {
			secrets = append(secrets, secret)
		}

		pos = braceEnd + 1
	}

	return secrets
}

// --- Parameter Parsing ---

var (
	paramRegex       = regexp.MustCompile(`(?m)^param\s+(\w+)\s+(\w+)(?:\s*=\s*(.+))?$`)
	descriptionRegex = regexp.MustCompile(`@description\('([^']*)'\)`)
	secureRegex      = regexp.MustCompile(`@secure\(\)`)
)

// parseBicepParameters extracts all parameter declarations from Bicep text.
func parseBicepParameters(text string, sourceFile string) []BicepParameter {
	var params []BicepParameter
	lines := strings.Split(text, "\n")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		match := paramRegex.FindStringSubmatch(trimmed)
		if match == nil {
			continue
		}

		param := BicepParameter{
			Name:       match[1],
			Type:       match[2],
			SourceFile: sourceFile,
		}

		if len(match) > 3 && match[3] != "" {
			param.DefaultValue = strings.TrimSpace(match[3])
		}

		// Look backwards for decorators
		for j := i - 1; j >= 0; j-- {
			prevLine := strings.TrimSpace(lines[j])
			if prevLine == "" {
				break
			}

			if descMatch := descriptionRegex.FindStringSubmatch(prevLine); descMatch != nil {
				param.Description = descMatch[1]
			}

			if secureRegex.MatchString(prevLine) {
				param.IsSecure = true
			}

			// Stop if we hit something that's not a decorator
			if !strings.HasPrefix(prevLine, "@") {
				break
			}
		}

		params = append(params, param)
	}

	return params
}

// --- Module Parsing ---

var moduleDeclRegex = regexp.MustCompile(`(?m)^module\s+(\w+)\s+'([^']+)'\s*=\s*\{`)

// parseBicepModules extracts all module declarations from Bicep text.
func parseBicepModules(text string) []BicepModule {
	var modules []BicepModule
	matches := moduleDeclRegex.FindAllStringSubmatchIndex(text, -1)

	for _, loc := range matches {
		name := text[loc[2]:loc[3]]
		source := text[loc[4]:loc[5]]

		// Find the matching closing brace
		bodyStart := loc[1] - 1
		bodyEnd := findMatchingBrace(text, bodyStart)
		if bodyEnd < 0 {
			continue
		}

		body := text[bodyStart : bodyEnd+1]

		module := BicepModule{
			Name:       name,
			Source:     source,
			Parameters: extractModuleParams(body),
			DependsOn:  extractDependsOn(body),
		}

		modules = append(modules, module)
	}

	return modules
}

// extractModuleParams extracts parameter expressions from a module body.
func extractModuleParams(body string) map[string]string {
	params := make(map[string]string)
	paramsBlock := extractBlock(body, "params")
	if paramsBlock == "" {
		return params
	}

	paramAssignRegex := regexp.MustCompile(`(?m)^\s*(\w+)\s*:\s*(.+)$`)
	matches := paramAssignRegex.FindAllStringSubmatch(paramsBlock, -1)
	for _, match := range matches {
		params[match[1]] = strings.TrimSpace(match[2])
	}

	return params
}

// extractDependsOn extracts the dependsOn array from a module body.
func extractDependsOn(body string) []string {
	dependsOnStart := strings.Index(body, "dependsOn:")
	if dependsOnStart < 0 {
		return nil
	}

	bracketStart := strings.Index(body[dependsOnStart:], "[")
	if bracketStart < 0 {
		return nil
	}
	bracketStart += dependsOnStart

	bracketEnd := findMatchingBracket(body, bracketStart)
	if bracketEnd < 0 {
		return nil
	}

	content := body[bracketStart+1 : bracketEnd]
	var deps []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			deps = append(deps, trimmed)
		}
	}

	return deps
}

// --- Variable Parsing ---

var varRegex = regexp.MustCompile(`(?m)^var\s+(\w+)\s*=\s*(.+)$`)

// parseBicepVariables extracts all variable declarations from Bicep text.
func parseBicepVariables(text string) []BicepVariable {
	var variables []BicepVariable
	matches := varRegex.FindAllStringSubmatch(text, -1)

	for _, match := range matches {
		variables = append(variables, BicepVariable{
			Name:  match[1],
			Value: strings.TrimSpace(match[2]),
		})
	}

	return variables
}

// --- Utility Functions ---

// findMatchingBrace finds the position of the closing brace matching the opening brace at position start.
func findMatchingBrace(text string, start int) int {
	if start >= len(text) || text[start] != '{' {
		return -1
	}

	depth := 0
	inString := false
	stringChar := byte(0)

	for i := start; i < len(text); i++ {
		ch := text[i]

		if inString {
			if ch == stringChar {
				inString = false
			}
			continue
		}

		if ch == '\'' || ch == '"' {
			inString = true
			stringChar = ch
			continue
		}

		// Skip single-line comments
		if ch == '/' && i+1 < len(text) && text[i+1] == '/' {
			// Skip to end of line
			for i < len(text) && text[i] != '\n' {
				i++
			}
			continue
		}

		// Skip block comments
		if ch == '/' && i+1 < len(text) && text[i+1] == '*' {
			i += 2
			for i+1 < len(text) {
				if text[i] == '*' && text[i+1] == '/' {
					i++
					break
				}
				i++
			}
			continue
		}

		if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return -1
}

// findMatchingBracket finds the position of the closing bracket matching the opening bracket at position start.
func findMatchingBracket(text string, start int) int {
	if start >= len(text) || text[start] != '[' {
		return -1
	}

	depth := 0
	inString := false
	stringChar := byte(0)

	for i := start; i < len(text); i++ {
		ch := text[i]

		if inString {
			if ch == stringChar {
				inString = false
			}
			continue
		}

		if ch == '\'' || ch == '"' {
			inString = true
			stringChar = ch
			continue
		}

		if ch == '[' {
			depth++
		} else if ch == ']' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return -1
}

// extractBlock extracts the content of a named block (e.g., "ingress: { ... }").
func extractBlock(body string, name string) string {
	// Match "name: {" or "name: \n {"
	patterns := []string{
		name + ":",
	}

	for _, pattern := range patterns {
		idx := strings.Index(body, pattern)
		if idx < 0 {
			continue
		}

		// Find the opening brace after the colon
		afterColon := body[idx+len(pattern):]
		braceIdx := strings.Index(afterColon, "{")
		if braceIdx < 0 {
			continue
		}

		absIdx := idx + len(pattern) + braceIdx
		endIdx := findMatchingBrace(body, absIdx)
		if endIdx < 0 {
			continue
		}

		return body[absIdx : endIdx+1]
	}

	return ""
}

// extractSimpleProperty extracts a simple property value like "name: 'value'".
func extractSimpleProperty(body string, name string) string {
	re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `\s*:\s*(.+)$`)
	match := re.FindStringSubmatch(body)
	if match == nil {
		return ""
	}
	return strings.TrimSpace(match[1])
}

// extractSimpleValue extracts a simple value for a property, trimming whitespace.
func extractSimpleValue(body string, name string) string {
	val := extractSimpleProperty(body, name)
	return strings.TrimSpace(val)
}

// countLines counts the number of newlines before the given position (1-based line number).
func countLines(text string) int {
	return strings.Count(text, "\n") + 1
}
