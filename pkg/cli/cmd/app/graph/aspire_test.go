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
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

// T017: Unit tests for mapAspireTypeToRadius covering all known mappings and fallback.
func Test_mapAspireTypeToRadius(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		method       string
		expectedType string
		expectedOK   bool
	}{
		// Compute types
		{"Project", "Applications.Core/containers", true},
		{"Container", "Applications.Core/containers", true},
		{"Executable", "Applications.Core/containers", true},
		{"NpmApp", "Applications.Core/containers", true},
		{"ViteApp", "Applications.Core/containers", true},
		{"PythonApp", "Applications.Core/containers", true},
		{"GolangApp", "Applications.Core/containers", true},
		{"JavaApp", "Applications.Core/containers", true},
		{"RustApp", "Applications.Core/containers", true},
		{"BunApp", "Applications.Core/containers", true},
		{"DenoApp", "Applications.Core/containers", true},

		// Cache types
		{"Redis", "Applications.Datastores/redisCaches", true},
		{"Valkey", "Applications.Datastores/redisCaches", true},
		{"Garnet", "Applications.Datastores/redisCaches", true},

		// SQL Database types
		{"SqlServer", "Applications.Datastores/sqlDatabases", true},
		{"Postgres", "Applications.Datastores/sqlDatabases", true},
		{"MySql", "Applications.Datastores/sqlDatabases", true},
		{"Oracle", "Applications.Datastores/sqlDatabases", true},

		// NoSQL Database types
		{"MongoDB", "Applications.Datastores/mongoDatabases", true},

		// Messaging types
		{"RabbitMQ", "Applications.Messaging/rabbitMQQueues", true},
		{"Kafka", "Applications.Messaging/rabbitMQQueues", true},
		{"Nats", "Applications.Messaging/rabbitMQQueues", true},

		// Fallback for unknown types
		{"Elasticsearch", "Applications.Core/extenders", false},
		{"Qdrant", "Applications.Core/extenders", false},
		{"Milvus", "Applications.Core/extenders", false},
		{"AzureStorage", "Applications.Core/extenders", false},
		{"UnknownService", "Applications.Core/extenders", false},
	}

	for _, tc := range testCases {
		t.Run(tc.method, func(t *testing.T) {
			t.Parallel()
			radiusType, known := mapAspireTypeToRadius(tc.method)
			require.Equal(t, tc.expectedType, radiusType)
			require.Equal(t, tc.expectedOK, known)
		})
	}
}

// T018: Unit tests for parseAspireAppHost covering simple declarations, fluent chains,
// inline resources, WithReference resolution, WaitFor skip, and external endpoint detection.
func Test_parseAspireAppHost_SimpleDeclarations(t *testing.T) {
	t.Parallel()

	content := `var builder = DistributedApplication.CreateBuilder(args);

var cache = builder.AddRedis("cache");

var apiservice = builder.AddProject<Projects.AspireApp_ApiService>("apiservice")
    .WithReference(cache);

builder.Build().Run();`

	resources, connections, warnings, err := parseAspireAppHost(content)
	require.NoError(t, err)
	require.Empty(t, warnings)

	// Should find 2 resources: cache and apiservice
	require.Len(t, resources, 2)

	resourceNames := make([]string, len(resources))
	for i, r := range resources {
		resourceNames[i] = r.resourceName
	}
	require.Contains(t, resourceNames, "cache")
	require.Contains(t, resourceNames, "apiservice")

	// Find cache resource and verify type
	for _, r := range resources {
		if r.resourceName == "cache" {
			require.Equal(t, "Redis", r.builderMethod)
			require.Equal(t, "Applications.Datastores/redisCaches", r.radiusType)
			require.Equal(t, "cache", r.varName)
		}
		if r.resourceName == "apiservice" {
			require.Equal(t, "Project", r.builderMethod)
			require.Equal(t, "Applications.Core/containers", r.radiusType)
		}
	}

	// Should find 1 connection: apiservice -> cache
	require.Len(t, connections, 1)
	require.Equal(t, "apiservice", connections[0].sourceResourceName)
	require.Equal(t, "cache", connections[0].targetResourceName)
}

func Test_parseAspireAppHost_FluentChains(t *testing.T) {
	t.Parallel()

	content := `var builder = DistributedApplication.CreateBuilder(args);

var sqlserver = builder.AddSqlServer("sqlserver")
    .AddDatabase("weatherdb");

var apiservice = builder.AddProject<Projects.AspireApp_ApiService>("apiservice")
    .WithReference(sqlserver);

builder.Build().Run();`

	resources, connections, warnings, err := parseAspireAppHost(content)
	require.NoError(t, err)
	require.Empty(t, warnings)

	// Should find 3 resources: sqlserver, weatherdb, apiservice
	require.Len(t, resources, 3)

	resourceNames := make([]string, len(resources))
	for i, r := range resources {
		resourceNames[i] = r.resourceName
	}
	require.Contains(t, resourceNames, "sqlserver")
	require.Contains(t, resourceNames, "weatherdb")
	require.Contains(t, resourceNames, "apiservice")

	// weatherdb should inherit sqlserver's type
	for _, r := range resources {
		if r.resourceName == "weatherdb" {
			require.Equal(t, "Applications.Datastores/sqlDatabases", r.radiusType)
			require.Equal(t, "Database", r.builderMethod)
			require.Equal(t, "sqlserver", r.parentVarName)
		}
	}

	// Connection: apiservice -> weatherdb (variable resolves to last Add* in chain)
	require.Len(t, connections, 1)
	require.Equal(t, "apiservice", connections[0].sourceResourceName)
	require.Equal(t, "weatherdb", connections[0].targetResourceName)
}

func Test_parseAspireAppHost_InlineResources(t *testing.T) {
	t.Parallel()

	content := `var builder = DistributedApplication.CreateBuilder(args);

var cache = builder.AddRedis("cache");

builder.AddProject<Projects.AspireApp_Web>("webfrontend")
    .WithExternalHttpEndpoints()
    .WithReference(cache);

builder.Build().Run();`

	resources, connections, warnings, err := parseAspireAppHost(content)
	require.NoError(t, err)
	require.Empty(t, warnings)

	// Should find 2 resources: cache, webfrontend
	require.Len(t, resources, 2)

	// webfrontend should be marked as external
	for _, r := range resources {
		if r.resourceName == "webfrontend" {
			require.True(t, r.isExternal)
			require.Equal(t, "Applications.Core/containers", r.radiusType)
		}
	}

	// Connection: webfrontend -> cache
	require.Len(t, connections, 1)
	require.Equal(t, "webfrontend", connections[0].sourceResourceName)
	require.Equal(t, "cache", connections[0].targetResourceName)
}

func Test_parseAspireAppHost_ExternalEndpoints(t *testing.T) {
	t.Parallel()

	content := `var builder = DistributedApplication.CreateBuilder(args);

var webfrontend = builder.AddProject<Projects.Web>("webfrontend")
    .WithExternalHttpEndpoints();

builder.Build().Run();`

	resources, _, warnings, err := parseAspireAppHost(content)
	require.NoError(t, err)
	require.Empty(t, warnings)

	require.Len(t, resources, 1)
	require.True(t, resources[0].isExternal)
	require.Equal(t, "webfrontend", resources[0].resourceName)
}

func Test_parseAspireAppHost_WaitForSkipped(t *testing.T) {
	t.Parallel()

	content := `var builder = DistributedApplication.CreateBuilder(args);

var cache = builder.AddRedis("cache");

var apiservice = builder.AddProject<Projects.ApiService>("apiservice")
    .WithReference(cache)
    .WaitFor(cache);

builder.Build().Run();`

	resources, connections, warnings, err := parseAspireAppHost(content)
	require.NoError(t, err)
	require.Empty(t, warnings)

	// Should only create 1 connection (WithReference), not 2 (WaitFor is skipped)
	require.Len(t, connections, 1)
	require.Len(t, resources, 2)
}

func Test_parseAspireAppHost_UnknownTypes(t *testing.T) {
	t.Parallel()

	content := `var builder = DistributedApplication.CreateBuilder(args);

var search = builder.AddElasticsearch("search");

builder.Build().Run();`

	resources, _, warnings, err := parseAspireAppHost(content)
	require.NoError(t, err)

	require.Len(t, resources, 1)
	require.Equal(t, "Applications.Core/extenders", resources[0].radiusType)

	// Should have a warning about the unknown type
	require.Len(t, warnings, 1)
	require.Contains(t, warnings[0], "unknown Aspire resource type 'AddElasticsearch'")
	require.Contains(t, warnings[0], "search")
}

func Test_parseAspireAppHost_EmptyContent(t *testing.T) {
	t.Parallel()

	content := `var builder = DistributedApplication.CreateBuilder(args);

builder.Build().Run();`

	resources, connections, warnings, err := parseAspireAppHost(content)
	require.NoError(t, err)
	require.Empty(t, warnings)
	require.Empty(t, resources)
	require.Empty(t, connections)
}

func Test_parseAspireAppHost_FullTopology(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(filepath.Join("testdata", "aspire", "simple-apphost", "AppHost.cs"))
	require.NoError(t, err)

	resources, connections, warnings, parseErr := parseAspireAppHost(string(content))
	require.NoError(t, parseErr)
	require.Empty(t, warnings)

	// simple-apphost has 5 resources: webfrontend, apiservice, cache, sqlserver, weatherdb
	require.Len(t, resources, 5)

	resourceNames := make(map[string]bool)
	for _, r := range resources {
		resourceNames[r.resourceName] = true
	}
	require.True(t, resourceNames["webfrontend"])
	require.True(t, resourceNames["apiservice"])
	require.True(t, resourceNames["cache"])
	require.True(t, resourceNames["sqlserver"])
	require.True(t, resourceNames["weatherdb"])

	// 3 connections: webfrontend->apiservice, webfrontend->cache, apiservice->weatherdb
	require.Len(t, connections, 3)

	connSet := make(map[string]bool)
	for _, c := range connections {
		connSet[c.sourceResourceName+"->"+c.targetResourceName] = true
	}
	// apiservice references sqlserver variable, which resolves to weatherdb
	require.True(t, connSet["apiservice->weatherdb"], "expected apiservice->weatherdb connection")
	// webfrontend references apiservice variable (resolves to apiservice) and cache variable (resolves to cache)
	require.True(t, connSet["webfrontend->apiservice"], "expected webfrontend->apiservice connection")
	require.True(t, connSet["webfrontend->cache"], "expected webfrontend->cache connection")
}

// T034: Unit test for unresolvable .WithReference() target.
func Test_parseAspireAppHost_UnresolvableReference(t *testing.T) {
	t.Parallel()

	content := `var builder = DistributedApplication.CreateBuilder(args);

var apiservice = builder.AddProject<Projects.ApiService>("apiservice")
    .WithReference(nonexistentVar);

builder.Build().Run();`

	resources, connections, warnings, err := parseAspireAppHost(content)
	require.NoError(t, err)

	require.Len(t, resources, 1)
	require.Empty(t, connections, "unresolvable reference should not create a connection")

	require.Len(t, warnings, 1)
	require.Contains(t, warnings[0], "could not resolve reference to 'nonexistentVar'")
	require.Contains(t, warnings[0], "apiservice")
}

func Test_parseAspireAppHost_MultipleReferences(t *testing.T) {
	t.Parallel()

	content := `var builder = DistributedApplication.CreateBuilder(args);

var cache = builder.AddRedis("cache");
var db = builder.AddMongoDB("mongodb");

var apiservice = builder.AddProject<Projects.ApiService>("apiservice")
    .WithReference(cache)
    .WithReference(db);

builder.Build().Run();`

	resources, connections, warnings, err := parseAspireAppHost(content)
	require.NoError(t, err)
	require.Empty(t, warnings)

	require.Len(t, resources, 3)
	require.Len(t, connections, 2)

	connSet := make(map[string]bool)
	for _, c := range connections {
		connSet[c.sourceResourceName+"->"+c.targetResourceName] = true
	}
	require.True(t, connSet["apiservice->cache"])
	require.True(t, connSet["apiservice->mongodb"])
}

// T019: Unit tests for discoverAppHostProject and findEntryPointFile.
func Test_discoverAppHostProject_DirectCsproj(t *testing.T) {
	t.Parallel()

	csprojPath := filepath.Join("testdata", "aspire", "simple-apphost", "SimpleAppHost.AppHost.csproj")
	projectDir, foundCsproj, err := discoverAppHostProject(csprojPath)
	require.NoError(t, err)

	absExpected, _ := filepath.Abs(filepath.Join("testdata", "aspire", "simple-apphost"))
	require.Equal(t, absExpected, projectDir)
	require.Equal(t, filepath.Join(absExpected, "SimpleAppHost.AppHost.csproj"), foundCsproj)
}

func Test_discoverAppHostProject_DirectoryWithCsproj(t *testing.T) {
	t.Parallel()

	projectDir, csprojPath, err := discoverAppHostProject(filepath.Join("testdata", "aspire", "simple-apphost"))
	require.NoError(t, err)

	absExpected, _ := filepath.Abs(filepath.Join("testdata", "aspire", "simple-apphost"))
	require.Equal(t, absExpected, projectDir)
	require.Contains(t, csprojPath, "SimpleAppHost.AppHost.csproj")
}

func Test_discoverAppHostProject_SettingsJson(t *testing.T) {
	t.Parallel()

	// with-settings-json/.aspire/settings.json points to ../simple-apphost/SimpleAppHost.AppHost.csproj
	projectDir, csprojPath, err := discoverAppHostProject(filepath.Join("testdata", "aspire", "with-settings-json"))
	require.NoError(t, err)

	require.Contains(t, csprojPath, "SimpleAppHost.AppHost.csproj")
	require.NotEmpty(t, projectDir)
}

func Test_discoverAppHostProject_NoProject(t *testing.T) {
	t.Parallel()

	// empty-apphost has no .csproj file
	_, _, err := discoverAppHostProject(filepath.Join("testdata", "aspire", "empty-apphost"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "no Aspire AppHost project found")
}

func Test_discoverAppHostProject_PathNotFound(t *testing.T) {
	t.Parallel()

	_, _, err := discoverAppHostProject(filepath.Join("testdata", "aspire", "nonexistent"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "path not found")
}

func Test_findEntryPointFile_AppHostCs(t *testing.T) {
	t.Parallel()

	absDir, _ := filepath.Abs(filepath.Join("testdata", "aspire", "simple-apphost"))
	entryPoint, err := findEntryPointFile(absDir)
	require.NoError(t, err)
	require.Contains(t, entryPoint, "AppHost.cs")
}

func Test_findEntryPointFile_NoFiles(t *testing.T) {
	t.Parallel()

	// Create a temp dir with no .cs files
	tmpDir := t.TempDir()
	_, err := findEntryPointFile(tmpDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no AppHost entry point file")
}

func Test_findEntryPointFile_ProgramCsFallback(t *testing.T) {
	t.Parallel()

	// Create temp dir with only Program.cs
	tmpDir := t.TempDir()
	err := os.WriteFile(filepath.Join(tmpDir, "Program.cs"), []byte("var builder = DistributedApplication.CreateBuilder(args);"), 0644)
	require.NoError(t, err)

	entryPoint, findErr := findEntryPointFile(tmpDir)
	require.NoError(t, findErr)
	require.Contains(t, entryPoint, "Program.cs")
}

func Test_findEntryPointFile_ScanForCreateBuilder(t *testing.T) {
	t.Parallel()

	// Create temp dir with a custom .cs file containing CreateBuilder
	tmpDir := t.TempDir()
	err := os.WriteFile(filepath.Join(tmpDir, "Startup.cs"), []byte(`
var builder = DistributedApplication.CreateBuilder(args);
builder.Build().Run();
`), 0644)
	require.NoError(t, err)

	entryPoint, findErr := findEntryPointFile(tmpDir)
	require.NoError(t, findErr)
	require.Contains(t, entryPoint, "Startup.cs")
}

// T020: Unit tests for aspireResourcesToGenericResources.
func Test_aspireResourcesToGenericResources(t *testing.T) {
	t.Parallel()

	resources := []aspireResource{
		{
			varName:       "cache",
			resourceName:  "cache",
			builderMethod: "Redis",
			radiusType:    "Applications.Datastores/redisCaches",
		},
		{
			varName:       "apiservice",
			resourceName:  "apiservice",
			builderMethod: "Project",
			radiusType:    "Applications.Core/containers",
		},
	}

	connections := []aspireConnection{
		{
			sourceResourceName: "apiservice",
			targetResourceName: "cache",
			connectionName:     "cache",
		},
	}

	genericResources, err := aspireResourcesToGenericResources(resources, connections)
	require.NoError(t, err)
	require.Len(t, genericResources, 2)

	// Resources should be sorted by type, then name
	for _, gr := range genericResources {
		require.NotNil(t, gr.ID)
		require.NotNil(t, gr.Name)
		require.NotNil(t, gr.Type)
		require.NotNil(t, gr.Properties)

		props := gr.Properties
		require.Equal(t, "NotDeployed", props["provisioningState"])

		status, ok := props["status"].(map[string]any)
		require.True(t, ok)
		outputResources, ok := status["outputResources"].([]any)
		require.True(t, ok)
		require.Empty(t, outputResources)
	}

	// Find apiservice and verify connections
	for _, gr := range genericResources {
		if to.String(gr.Name) == "apiservice" {
			conns, ok := gr.Properties["connections"].(map[string]any)
			require.True(t, ok)
			require.Contains(t, conns, "cache")

			cacheConn, ok := conns["cache"].(map[string]any)
			require.True(t, ok)
			require.Contains(t, cacheConn["source"].(string), "Applications.Datastores/redisCaches")
			require.Contains(t, cacheConn["source"].(string), "cache")
		}
	}
}

func Test_aspireResourcesToGenericResources_WithExternal(t *testing.T) {
	t.Parallel()

	resources := []aspireResource{
		{
			varName:       "webfrontend",
			resourceName:  "webfrontend",
			builderMethod: "Project",
			radiusType:    "Applications.Core/containers",
			isExternal:    true,
		},
	}

	genericResources, err := aspireResourcesToGenericResources(resources, nil)
	require.NoError(t, err)
	require.Len(t, genericResources, 1)

	props := genericResources[0].Properties
	require.Equal(t, true, props["external"])
}

func Test_aspireResourcesToGenericResources_Empty(t *testing.T) {
	t.Parallel()

	genericResources, err := aspireResourcesToGenericResources(nil, nil)
	require.NoError(t, err)
	require.Empty(t, genericResources)
}

func Test_aspireResourcesToGenericResources_SynthesizedIDs(t *testing.T) {
	t.Parallel()

	resources := []aspireResource{
		{
			varName:       "cache",
			resourceName:  "cache",
			builderMethod: "Redis",
			radiusType:    "Applications.Datastores/redisCaches",
		},
	}

	genericResources, err := aspireResourcesToGenericResources(resources, nil)
	require.NoError(t, err)
	require.Len(t, genericResources, 1)

	expectedID := "/planes/radius/local/resourceGroups/default/providers/Applications.Datastores/redisCaches/cache"
	require.Equal(t, expectedID, to.String(genericResources[0].ID))
}

// T021: Unit tests for deriveApplicationName.
func Test_deriveApplicationName_FromProjectName(t *testing.T) {
	t.Parallel()

	// No rad.yaml present, should derive from csproj name
	tmpDir := t.TempDir()
	csprojPath := filepath.Join(tmpDir, "AspireApp.AppHost.csproj")
	err := os.WriteFile(csprojPath, []byte("<Project/>"), 0644)
	require.NoError(t, err)

	name := deriveApplicationName(tmpDir, tmpDir, csprojPath)
	require.Equal(t, "aspireapp", name)
}

func Test_deriveApplicationName_StripAppHostSuffix(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	csprojPath := filepath.Join(tmpDir, "MyService.AppHost.csproj")
	err := os.WriteFile(csprojPath, []byte("<Project/>"), 0644)
	require.NoError(t, err)

	name := deriveApplicationName(tmpDir, tmpDir, csprojPath)
	require.Equal(t, "myservice", name)
}

func Test_deriveApplicationName_FromRadYaml(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create .rad/rad.yaml
	radDir := filepath.Join(tmpDir, ".rad")
	err := os.MkdirAll(radDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(radDir, "rad.yaml"), []byte(`workspace:
  application: my-cool-app
`), 0644)
	require.NoError(t, err)

	csprojPath := filepath.Join(tmpDir, "Whatever.AppHost.csproj")
	name := deriveApplicationName(tmpDir, tmpDir, csprojPath)
	require.Equal(t, "my-cool-app", name)
}

func Test_deriveApplicationName_FallbackToDirectory(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	name := deriveApplicationName(tmpDir, tmpDir, "")
	require.NotEmpty(t, name)
}
