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

"gopkg.in/yaml.v3"
)

// ScanDirectory discovers the AppHost infra/ directory containing .tmpl.yaml files
// and the solution-level infra/main.bicep. It returns an AspireAppDescriptor with
// the discovered paths but does not parse files (callers should use ParseYAMLTemplate
// and ParseBicepFile for parsing).
func ScanDirectory(dirPath string) (*AspireAppDescriptor, error) {
absPath, err := filepath.Abs(dirPath)
if err != nil {
return nil, fmt.Errorf("cannot resolve path '%s': %w", dirPath, err)
}

info, err := os.Stat(absPath)
if err != nil {
return nil, fmt.Errorf("cannot access directory '%s': %w", dirPath, err)
}

if !info.IsDir() {
return nil, fmt.Errorf("'%s' is not a directory", dirPath)
}

descriptor := &AspireAppDescriptor{
RootDir: absPath,
}

// Discover AppHost infra/ directories containing .tmpl.yaml files
appHostInfraDirs, err := findAppHostInfraDirs(absPath)
if err != nil {
return nil, err
}

if len(appHostInfraDirs) == 0 {
return nil, fmt.Errorf("Input directory '%s' does not contain Aspire infrastructure artifacts.\nExpected: An Aspire application directory with <AppHost>/infra/*.tmpl.yaml files.\nRun 'azd infra synth' in your Aspire project to generate the required artifacts.", dirPath)
}

if len(appHostInfraDirs) > 1 {
return nil, fmt.Errorf("Multiple Aspire projects detected in '%s'.\nThis PoC supports a single Aspire project only. Please provide a directory with one main.bicep file.", dirPath)
}

descriptor.AppHostDir = filepath.Dir(appHostInfraDirs[0])

// Parse all .tmpl.yaml files in the AppHost infra/ directory (lexicographic order)
tmplFiles, err := findTmplYAMLFiles(appHostInfraDirs[0])
if err != nil {
return nil, fmt.Errorf("error scanning AppHost infra directory: %w", err)
}

for _, tmplPath := range tmplFiles {
st, err := ParseYAMLTemplate(tmplPath)
if err != nil {
return nil, fmt.Errorf("failed to parse YAML template '%s': %w", tmplPath, err)
}
descriptor.ServiceTemplates = append(descriptor.ServiceTemplates, st)
}

// Discover and parse solution-level infra/main.bicep
mainBicepPath := filepath.Join(absPath, "infra", "main.bicep")
if _, err := os.Stat(mainBicepPath); err == nil {
bf, err := ParseBicepFile(mainBicepPath)
if err != nil {
return nil, fmt.Errorf("failed to parse main.bicep: %w", err)
}
descriptor.MainBicep = &bf
}

// Parse main.parameters.json if it exists
paramsPath := filepath.Join(absPath, "infra", "main.parameters.json")
if _, err := os.Stat(paramsPath); err == nil {
paramsData, readErr := os.ReadFile(paramsPath)
if readErr == nil {
var params map[string]any
if jsonErr := yaml.Unmarshal(paramsData, &params); jsonErr == nil {
descriptor.ParametersJSON = params
}
}
}

return descriptor, nil
}

// findAppHostInfraDirs scans for directories that contain infra/ subdirectories with .tmpl.yaml files.
// It walks the root looking for patterns like <AppHost>/infra/*.tmpl.yaml.
func findAppHostInfraDirs(rootDir string) ([]string, error) {
var infraDirs []string

entries, err := os.ReadDir(rootDir)
if err != nil {
return nil, fmt.Errorf("cannot read directory '%s': %w", rootDir, err)
}

for _, entry := range entries {
if !entry.IsDir() {
continue
}

// Skip known non-AppHost directories
name := entry.Name()
if name == "infra" || name == ".git" || name == ".azure" || name == ".aspire" || name == "node_modules" {
continue
}

infraPath := filepath.Join(rootDir, name, "infra")
if info, statErr := os.Stat(infraPath); statErr == nil && info.IsDir() {
// Check if this infra/ directory has .tmpl.yaml files
tmplFiles, findErr := findTmplYAMLFiles(infraPath)
if findErr == nil && len(tmplFiles) > 0 {
infraDirs = append(infraDirs, infraPath)
}
}
}

return infraDirs, nil
}

// findTmplYAMLFiles finds all .tmpl.yaml files in a directory in lexicographic order.
func findTmplYAMLFiles(dirPath string) ([]string, error) {
var files []string

err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
if err != nil {
return err
}

if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".tmpl.yaml") {
files = append(files, path)
}

return nil
})

if err != nil {
return nil, err
}

return files, nil
}

// --- Go template expression handling ---

var (
// Matches {{ .Image }}
imageExprRegex = regexp.MustCompile(`\{\{\s*\.Image\s*\}\}`)
// Matches {{ .Env.NAME }}
envExprRegex = regexp.MustCompile(`\{\{\s*\.Env\.\w+\s*\}\}`)
// Matches {{ securedParameter "name" }}
securedParamRegex = regexp.MustCompile(`\{\{\s*securedParameter\s+"([^"]+)"\s*\}\}`)
// Matches {{ targetPortOrDefault N }}
targetPortRegex = regexp.MustCompile(`\{\{\s*targetPortOrDefault\s+(\d+)\s*\}\}`)
// Matches {{ uriEncode (...) }} — can be nested
uriEncodeRegex = regexp.MustCompile(`\{\{\s*uriEncode\s+\([^)]*\)\s*\}\}`)
// Catch-all for any remaining {{ ... }} expressions
genericTemplateRegex = regexp.MustCompile(`\{\{[^}]*\}\}`)
)

// stripGoTemplateExpressions replaces Go template expressions with YAML-safe values
// so the YAML can be parsed.
func stripGoTemplateExpressions(content string) string {
// Replace {{ .Image }} with a placeholder
result := imageExprRegex.ReplaceAllString(content, "IMAGE_PLACEHOLDER")

// Replace {{ targetPortOrDefault N }} with the default value N
result = targetPortRegex.ReplaceAllStringFunc(result, func(match string) string {
submatch := targetPortRegex.FindStringSubmatch(match)
if len(submatch) > 1 {
return submatch[1]
}
return "0"
})

// Replace {{ securedParameter "name" }} with SECURED_PARAM_name
result = securedParamRegex.ReplaceAllStringFunc(result, func(match string) string {
submatch := securedParamRegex.FindStringSubmatch(match)
if len(submatch) > 1 {
return "SECURED_PARAM_" + submatch[1]
}
return "SECURED_PARAM"
})

// Replace {{ uriEncode (...) }} with empty string
result = uriEncodeRegex.ReplaceAllString(result, "")

// Replace {{ .Env.* }} with empty string
result = envExprRegex.ReplaceAllString(result, "")

// Replace any remaining {{ ... }} with empty string
result = genericTemplateRegex.ReplaceAllString(result, "")

return result
}

// --- YAML Template Parsing ---

// yamlTemplateDoc represents the top-level structure of a .tmpl.yaml file.
type yamlTemplateDoc struct {
APIVersion string                 `yaml:"api-version"`
Location   string                 `yaml:"location"`
Properties yamlProperties         `yaml:"properties"`
Tags       map[string]string      `yaml:"tags"`
Identity   map[string]interface{} `yaml:"identity"`
}

type yamlProperties struct {
EnvironmentID string            `yaml:"environmentId"`
Configuration yamlConfiguration `yaml:"configuration"`
Template      yamlTemplate      `yaml:"template"`
}

type yamlConfiguration struct {
ActiveRevisionsMode string        `yaml:"activeRevisionsMode"`
Ingress             *yamlIngress  `yaml:"ingress"`
Secrets             []yamlSecret  `yaml:"secrets"`
Registries          []interface{} `yaml:"registries"`
Runtime             interface{}   `yaml:"runtime"`
}

type yamlIngress struct {
External      bool   `yaml:"external"`
TargetPort    int    `yaml:"targetPort"`
Transport     string `yaml:"transport"`
AllowInsecure bool   `yaml:"allowInsecure"`
}

type yamlTemplate struct {
Containers []yamlContainer `yaml:"containers"`
Scale      interface{}     `yaml:"scale"`
}

type yamlContainer struct {
Image   string       `yaml:"image"`
Name    string       `yaml:"name"`
Env     []yamlEnvVar `yaml:"env"`
Command []string     `yaml:"command"`
Args    []string     `yaml:"args"`
}

type yamlEnvVar struct {
Name      string `yaml:"name"`
Value     string `yaml:"value"`
SecretRef string `yaml:"secretRef"`
}

type yamlSecret struct {
Name  string `yaml:"name"`
Value string `yaml:"value"`
}

// ParseYAMLTemplate reads a .tmpl.yaml file, strips Go template expressions,
// parses the cleaned YAML, and extracts a ServiceTemplate with IngressConfig,
// ContainerDef, EnvVar, SecretDef, and tags.
func ParseYAMLTemplate(filePath string) (ServiceTemplate, error) {
content, err := os.ReadFile(filePath)
if err != nil {
return ServiceTemplate{}, fmt.Errorf("cannot read file '%s': %w", filePath, err)
}

// Strip Go template expressions before YAML parsing
cleanedContent := stripGoTemplateExpressions(string(content))

var doc yamlTemplateDoc
if err := yaml.Unmarshal([]byte(cleanedContent), &doc); err != nil {
return ServiceTemplate{}, fmt.Errorf("cannot parse YAML in '%s': %w", filePath, err)
}

st := ServiceTemplate{
Path: filePath,
Tags: doc.Tags,
}

// Derive service name from tags or filename
if name, ok := doc.Tags["aspire-resource-name"]; ok && name != "" {
st.ServiceName = name
} else {
// Fall back to filename stem (remove .tmpl.yaml)
base := filepath.Base(filePath)
st.ServiceName = strings.TrimSuffix(base, ".tmpl.yaml")
}

if azdName, ok := doc.Tags["azd-service-name"]; ok {
st.AzdServiceName = azdName
}

// Extract ingress configuration
if doc.Properties.Configuration.Ingress != nil {
ing := doc.Properties.Configuration.Ingress
st.Ingress = &IngressConfig{
External:   ing.External,
TargetPort: ing.TargetPort,
Transport:  ing.Transport,
}
}

// Extract containers
for _, c := range doc.Properties.Template.Containers {
containerDef := ContainerDef{
Image:   c.Image,
Name:    c.Name,
Command: c.Command,
Args:    c.Args,
}

for _, env := range c.Env {
containerDef.Env = append(containerDef.Env, EnvVar{
Name:      env.Name,
Value:     env.Value,
SecretRef: env.SecretRef,
})
}

st.Containers = append(st.Containers, containerDef)
}

// Extract secrets
for _, s := range doc.Properties.Configuration.Secrets {
st.Secrets = append(st.Secrets, SecretDef{
Name:  s.Name,
Value: s.Value,
})
}

return st, nil
}

// --- Bicep File Parsing ---

// ParseBicepFile reads a Bicep file and extracts parameters and modules using regexp.
func ParseBicepFile(filePath string) (BicepFile, error) {
content, err := os.ReadFile(filePath)
if err != nil {
return BicepFile{}, fmt.Errorf("cannot read file '%s': %w", filePath, err)
}

text := string(content)
bf := BicepFile{
Path: filePath,
}

bf.Parameters = parseBicepParameters(text, filePath)
bf.Modules = parseBicepModules(text)
bf.Variables = parseBicepVariables(text)

return bf, nil
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

// Stop if we hit something that's not a decorator or metadata
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
patterns := []string{
name + ":",
}

for _, pattern := range patterns {
idx := strings.Index(body, pattern)
if idx < 0 {
continue
}

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

// extractTargetPortDefault extracts the default port from a {{ targetPortOrDefault N }} expression.
func extractTargetPortDefault(value string) (int, bool) {
match := targetPortRegex.FindStringSubmatch(value)
if len(match) > 1 {
port, err := strconv.Atoi(match[1])
if err == nil {
return port, true
}
}
return 0, false
}
