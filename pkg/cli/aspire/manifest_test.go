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
	"testing"
)

func TestParseManifest_ValidJSON(t *testing.T) {
	t.Parallel()

	manifest, err := parseManifest(filepath.Join("testdata", "simple-containers.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if manifest == nil {
		t.Fatal("expected non-nil manifest")
	}

	if len(manifest.Resources) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(manifest.Resources))
	}

	api, ok := manifest.Resources["api"]
	if !ok {
		t.Fatal("expected 'api' resource")
	}

	if api.Type != "container.v0" {
		t.Errorf("expected type 'container.v0', got %q", api.Type)
	}

	if api.Image != "myapp/api:latest" {
		t.Errorf("expected image 'myapp/api:latest', got %q", api.Image)
	}
}

func TestParseManifest_MalformedJSON(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bad.json")
	if err := os.WriteFile(path, []byte("{invalid json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := parseManifest(path)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestParseManifest_MissingResourcesMap(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "empty.json")
	if err := os.WriteFile(path, []byte(`{"$schema": "test"}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := parseManifest(path)
	if err == nil {
		t.Fatal("expected error for missing resources")
	}
}

func TestParseManifest_MissingTypeField(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "notype.json")
	content := `{"resources": {"test": {"image": "test:latest"}}}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := parseManifest(path)
	if err == nil {
		t.Fatal("expected error for missing type field")
	}
}

func TestParseManifest_FileNotFound(t *testing.T) {
	t.Parallel()

	_, err := parseManifest("nonexistent.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
