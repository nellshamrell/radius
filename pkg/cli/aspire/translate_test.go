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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// updateGolden is a test helper that updates the golden file when the -update flag is set.
const updateGolden = false

func TestTranslate_SimpleContainers(t *testing.T) {
	t.Parallel()

	result, err := Translate(TranslateOptions{
		ManifestPath: filepath.Join("testdata", "simple-containers.json"),
		AppName:      "app",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Bicep == "" {
		t.Fatal("expected non-empty Bicep output")
	}

	goldenPath := filepath.Join("testdata", "simple-containers.bicep")

	if updateGolden {
		if err := os.WriteFile(goldenPath, []byte(result.Bicep), 0644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}

		t.Log("Golden file updated")
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		// If golden file doesn't exist yet, write it.
		if os.IsNotExist(err) {
			if err := os.WriteFile(goldenPath, []byte(result.Bicep), 0644); err != nil {
				t.Fatalf("failed to write golden file: %v", err)
			}

			t.Log("Golden file created")
			return
		}

		t.Fatalf("failed to read golden file: %v", err)
	}

	if result.Bicep != string(expected) {
		t.Errorf("Bicep output does not match golden file.\nGot:\n%s\nExpected:\n%s", result.Bicep, string(expected))
	}
}

func TestTranslate_EmptyManifest(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "empty.json")
	content := `{"resources": {"worker": {"type": "executable.v0"}}}`
	if err := os.WriteFile(manifestPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := Translate(TranslateOptions{
		ManifestPath: manifestPath,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Bicep != "" {
		t.Error("expected empty Bicep output for empty manifest")
	}

	if len(result.Warnings) == 0 {
		t.Error("expected warnings for empty manifest")
	}
}

func TestTranslate_BrokenReference(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "broken-ref.json")
	content := `{
		"resources": {
			"api": {
				"type": "container.v0",
				"image": "api:latest",
				"env": {
					"DB_URL": "{nonexistent.bindings.http.url}"
				}
			}
		}
	}`
	if err := os.WriteFile(manifestPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Translate(TranslateOptions{
		ManifestPath: manifestPath,
	})
	if err == nil {
		t.Fatal("expected error for broken reference")
	}

	if !strings.Contains(err.Error(), "unknown resource") {
		t.Errorf("expected unknown resource error, got: %v", err)
	}
}

func TestTranslate_SimpleContainersResult(t *testing.T) {
	t.Parallel()

	result, err := Translate(TranslateOptions{
		ManifestPath: filepath.Join("testdata", "simple-containers.json"),
		AppName:      "testapp",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify we have the right resources.
	if len(result.Resources) < 2 {
		t.Fatalf("expected at least 2 resources, got %d", len(result.Resources))
	}

	// Verify Bicep structure.
	bicep := result.Bicep
	if !strings.Contains(bicep, "extension radius") {
		t.Error("missing 'extension radius'")
	}

	if !strings.Contains(bicep, "Applications.Core/containers@2023-10-01-preview") {
		t.Error("missing container resource type")
	}

	if !strings.Contains(bicep, "Applications.Core/applications@2023-10-01-preview") {
		t.Error("missing application resource type")
	}
}

func TestTranslate_BackingServices(t *testing.T) {
	t.Parallel()

	result, err := Translate(TranslateOptions{
		ManifestPath: filepath.Join("testdata", "backing-services.json"),
		AppName:      "app",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Bicep == "" {
		t.Fatal("expected non-empty Bicep output")
	}

	goldenPath := filepath.Join("testdata", "backing-services.bicep")

	if updateGolden {
		if err := os.WriteFile(goldenPath, []byte(result.Bicep), 0644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}

		t.Log("Golden file updated")
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.WriteFile(goldenPath, []byte(result.Bicep), 0644); err != nil {
				t.Fatalf("failed to write golden file: %v", err)
			}

			t.Log("Golden file created")
			return
		}

		t.Fatalf("failed to read golden file: %v", err)
	}

	if result.Bicep != string(expected) {
		t.Errorf("Bicep output does not match golden file.\nGot:\n%s\nExpected:\n%s", result.Bicep, string(expected))
	}
}

func TestTranslate_BackingServicesResult(t *testing.T) {
	t.Parallel()

	result, err := Translate(TranslateOptions{
		ManifestPath: filepath.Join("testdata", "backing-services.json"),
		AppName:      "app",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify we have portable resources.
	bicep := result.Bicep
	if !strings.Contains(bicep, "Applications.Datastores/redisCaches@2023-10-01-preview") {
		t.Error("missing redisCaches resource type")
	}

	if !strings.Contains(bicep, "Applications.Datastores/sqlDatabases@2023-10-01-preview") {
		t.Error("missing sqlDatabases resource type")
	}

	if !strings.Contains(bicep, "Applications.Messaging/rabbitMQQueues@2023-10-01-preview") {
		t.Error("missing rabbitMQQueues resource type")
	}

	if !strings.Contains(bicep, "resourceProvisioning: 'recipe'") {
		t.Error("missing recipe provisioning")
	}
}

func TestTranslate_Gateway(t *testing.T) {
	t.Parallel()

	result, err := Translate(TranslateOptions{
		ManifestPath: filepath.Join("testdata", "gateway.json"),
		AppName:      "app",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Bicep == "" {
		t.Fatal("expected non-empty Bicep output")
	}

	goldenPath := filepath.Join("testdata", "gateway.bicep")

	if updateGolden {
		if err := os.WriteFile(goldenPath, []byte(result.Bicep), 0644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}

		t.Log("Golden file updated")
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.WriteFile(goldenPath, []byte(result.Bicep), 0644); err != nil {
				t.Fatalf("failed to write golden file: %v", err)
			}

			t.Log("Golden file created")
			return
		}

		t.Fatalf("failed to read golden file: %v", err)
	}

	if result.Bicep != string(expected) {
		t.Errorf("Bicep output does not match golden file.\nGot:\n%s\nExpected:\n%s", result.Bicep, string(expected))
	}
}

func TestTranslate_GatewayResult(t *testing.T) {
	t.Parallel()

	result, err := Translate(TranslateOptions{
		ManifestPath: filepath.Join("testdata", "gateway.json"),
		AppName:      "app",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bicep := result.Bicep
	if !strings.Contains(bicep, "Applications.Core/gateways@2023-10-01-preview") {
		t.Error("missing gateway resource type")
	}

	if !strings.Contains(bicep, "routes:") {
		t.Error("missing gateway routes")
	}
}

func TestTranslate_Projects(t *testing.T) {
	t.Parallel()

	result, err := Translate(TranslateOptions{
		ManifestPath: filepath.Join("testdata", "projects.json"),
		AppName:      "app",
		ImageMappings: map[string]string{
			"webapp": "myregistry.io/webapp:v1.0",
			"worker": "myregistry.io/worker:v1.0",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Bicep == "" {
		t.Fatal("expected non-empty Bicep output")
	}

	goldenPath := filepath.Join("testdata", "projects.bicep")

	if updateGolden {
		if err := os.WriteFile(goldenPath, []byte(result.Bicep), 0644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}

		t.Log("Golden file updated")
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.WriteFile(goldenPath, []byte(result.Bicep), 0644); err != nil {
				t.Fatalf("failed to write golden file: %v", err)
			}

			t.Log("Golden file created")
			return
		}

		t.Fatalf("failed to read golden file: %v", err)
	}

	if result.Bicep != string(expected) {
		t.Errorf("Bicep output does not match golden file.\nGot:\n%s\nExpected:\n%s", result.Bicep, string(expected))
	}
}

func TestTranslate_ProjectMissingMapping(t *testing.T) {
	t.Parallel()

	_, err := Translate(TranslateOptions{
		ManifestPath: filepath.Join("testdata", "projects.json"),
		AppName:      "app",
		// Missing image mappings for project resources.
	})
	if err == nil {
		t.Fatal("expected error for missing image mapping")
	}

	if !strings.Contains(err.Error(), "requires an image mapping") {
		t.Errorf("expected missing image mapping error, got: %v", err)
	}
}

func TestTranslate_FullApp(t *testing.T) {
	t.Parallel()

	result, err := Translate(TranslateOptions{
		ManifestPath: filepath.Join("testdata", "full-app.json"),
		AppName:      "fullapp",
		ImageMappings: map[string]string{
			"api":    "myregistry.io/api:v1.0",
			"worker": "myregistry.io/worker:v1.0",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Bicep == "" {
		t.Fatal("expected non-empty Bicep output")
	}

	goldenPath := filepath.Join("testdata", "full-app.bicep")

	if updateGolden {
		if err := os.WriteFile(goldenPath, []byte(result.Bicep), 0644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}

		t.Log("Golden file updated")
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.WriteFile(goldenPath, []byte(result.Bicep), 0644); err != nil {
				t.Fatalf("failed to write golden file: %v", err)
			}

			t.Log("Golden file created")
			return
		}

		t.Fatalf("failed to read golden file: %v", err)
	}

	if result.Bicep != string(expected) {
		t.Errorf("Bicep output does not match golden file.\nGot:\n%s\nExpected:\n%s", result.Bicep, string(expected))
	}
}

func TestTranslate_FullAppResult(t *testing.T) {
	t.Parallel()

	result, err := Translate(TranslateOptions{
		ManifestPath: filepath.Join("testdata", "full-app.json"),
		AppName:      "fullapp",
		ImageMappings: map[string]string{
			"api":    "myregistry.io/api:v1.0",
			"worker": "myregistry.io/worker:v1.0",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bicep := result.Bicep

	// Verify all resource types are present.
	if !strings.Contains(bicep, "Applications.Core/applications@2023-10-01-preview") {
		t.Error("missing application resource")
	}

	if !strings.Contains(bicep, "Applications.Core/containers@2023-10-01-preview") {
		t.Error("missing container resource")
	}

	if !strings.Contains(bicep, "Applications.Datastores/redisCaches@2023-10-01-preview") {
		t.Error("missing redisCaches resource")
	}

	if !strings.Contains(bicep, "Applications.Datastores/sqlDatabases@2023-10-01-preview") {
		t.Error("missing sqlDatabases resource")
	}

	if !strings.Contains(bicep, "Applications.Messaging/rabbitMQQueues@2023-10-01-preview") {
		t.Error("missing rabbitMQQueues resource")
	}

	if !strings.Contains(bicep, "Applications.Core/gateways@2023-10-01-preview") {
		t.Error("missing gateway resource")
	}

	if !strings.Contains(bicep, "@secure()") {
		t.Error("missing @secure() annotation")
	}

	// Verify skipped resources generate warnings.
	foundSkipWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "executable.v0") {
			foundSkipWarning = true
			break
		}
	}

	if !foundSkipWarning {
		t.Error("expected warning for skipped executable.v0 resource")
	}
}

// --- Phase 8: Comprehensive error message tests ---

func TestTranslate_ErrorMessages(t *testing.T) {
	t.Parallel()

	t.Run("file not found", func(t *testing.T) {
		t.Parallel()

		_, err := Translate(TranslateOptions{
			ManifestPath: "/nonexistent/manifest.json",
		})
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), "manifest file not found") {
			t.Errorf("expected file not found error, got: %v", err)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "bad.json")
		if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := Translate(TranslateOptions{ManifestPath: path})
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), "failed to parse manifest") {
			t.Errorf("expected parse error, got: %v", err)
		}
	})

	t.Run("missing image mapping", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "project.json")
		content := `{"resources": {"api": {"type": "project.v1", "path": "api.csproj"}}}`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := Translate(TranslateOptions{ManifestPath: path})
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), "requires an image mapping") {
			t.Errorf("expected missing image mapping error, got: %v", err)
		}
	})

	t.Run("unknown expression reference", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "broken.json")
		content := `{"resources": {"api": {"type": "container.v0", "image": "api:latest", "env": {"URL": "{ghost.bindings.http.url}"}}}}`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := Translate(TranslateOptions{ManifestPath: path})
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), "unknown resource") {
			t.Errorf("expected unknown resource error, got: %v", err)
		}
	})

	t.Run("circular reference", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "circular.json")
		content := `{"resources": {
			"a": {"type": "value.v0", "connectionString": "{b.connectionString}"},
			"b": {"type": "value.v0", "connectionString": "{a.connectionString}"}
		}}`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := Translate(TranslateOptions{ManifestPath: path})
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), "circular reference") {
			t.Errorf("expected circular reference error, got: %v", err)
		}
	})
}

// --- Phase 8: Edge case tests ---

func TestTranslate_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("single container", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "single.json")
		content := `{"resources": {"app": {"type": "container.v0", "image": "myapp:latest"}}}`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		result, err := Translate(TranslateOptions{ManifestPath: path})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(result.Bicep, "Applications.Core/containers") {
			t.Error("expected container resource in output")
		}
	})

	t.Run("multiple same-port containers", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "sameport.json")
		content := `{"resources": {
			"api1": {"type": "container.v0", "image": "api1:latest", "bindings": {"http": {"scheme": "http", "port": 8080, "targetPort": 8080}}},
			"api2": {"type": "container.v0", "image": "api2:latest", "bindings": {"http": {"scheme": "http", "port": 8080, "targetPort": 8080}}}
		}}`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		result, err := Translate(TranslateOptions{ManifestPath: path})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(result.Bicep, "api1") || !strings.Contains(result.Bicep, "api2") {
			t.Error("expected both containers in output")
		}
	})

	t.Run("composite expression with multiple references", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "composite.json")
		// Use a non-backing-service image so the container binding resolution path is taken.
		content := `{"resources": {
			"db": {"type": "container.v0", "image": "mydb:latest", "bindings": {"tcp": {"scheme": "tcp", "port": 5432, "targetPort": 5432}}},
			"api": {"type": "container.v0", "image": "api:latest", "env": {"DB_CONN": "Server={db.bindings.tcp.host};Port={db.bindings.tcp.port};Database=mydb"}}
		}}`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		result, err := Translate(TranslateOptions{ManifestPath: path})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(result.Bicep, "Server=db;Port=5432;Database=mydb") {
			t.Errorf("expected composite expression to be resolved, got:\n%s", result.Bicep)
		}
	})

	t.Run("resource names requiring sanitization", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "sanitize.json")
		content := `{"resources": {
			"my-api-service": {"type": "container.v0", "image": "api:latest"},
			"123worker": {"type": "container.v0", "image": "worker:latest"}
		}}`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		result, err := Translate(TranslateOptions{ManifestPath: path})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// my-api-service → my_api_service
		if !strings.Contains(result.Bicep, "resource my_api_service") {
			t.Error("expected sanitized identifier 'my_api_service'")
		}

		// 123worker → r_123worker
		if !strings.Contains(result.Bicep, "resource r_123worker") {
			t.Error("expected sanitized identifier 'r_123worker'")
		}
	})

	t.Run("only unsupported resources", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "unsupported.json")
		content := `{"resources": {"a": {"type": "executable.v0"}, "b": {"type": "dockerfile.v0"}}}`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		result, err := Translate(TranslateOptions{ManifestPath: path})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Bicep != "" {
			t.Error("expected empty Bicep for all-unsupported manifest")
		}

		if len(result.Warnings) == 0 {
			t.Error("expected warnings for unsupported resources")
		}
	})
}

// --- Bicep structure validation ---

func TestTranslate_BicepStructure(t *testing.T) {
	t.Parallel()

	result, err := Translate(TranslateOptions{
		ManifestPath: filepath.Join("testdata", "full-app.json"),
		AppName:      "fullapp",
		ImageMappings: map[string]string{
			"api":    "myregistry.io/api:v1.0",
			"worker": "myregistry.io/worker:v1.0",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bicep := result.Bicep

	// Verify extension radius is first.
	if !strings.HasPrefix(bicep, "extension radius\n") {
		t.Error("expected 'extension radius' at the start")
	}

	// Verify API version is correct throughout.
	if count := strings.Count(bicep, "@2023-10-01-preview"); count < 5 {
		t.Errorf("expected at least 5 @2023-10-01-preview references, got %d", count)
	}

	// Verify ordering: extension → params → application → portable → containers → gateway.
	appIdx := strings.Index(bicep, "Applications.Core/applications")
	cacheIdx := strings.Index(bicep, "Applications.Datastores/redisCaches")
	containerIdx := strings.Index(bicep, "Applications.Core/containers")
	gatewayIdx := strings.Index(bicep, "Applications.Core/gateways")

	if appIdx >= cacheIdx {
		t.Error("application should come before portable resources")
	}

	if cacheIdx >= containerIdx {
		t.Error("portable resources should come before containers")
	}

	if containerIdx >= gatewayIdx {
		t.Error("containers should come before gateway")
	}
}
