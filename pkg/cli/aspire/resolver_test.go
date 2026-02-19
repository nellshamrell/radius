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

import "testing"

func TestParseExpressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		input          string
		expectedParts  int
		hasExpressions bool
	}{
		{
			name:           "single reference",
			input:          "{api.bindings.http.url}",
			expectedParts:  1,
			hasExpressions: true,
		},
		{
			name:           "connectionString reference",
			input:          "{cache.connectionString}",
			expectedParts:  1,
			hasExpressions: true,
		},
		{
			name:           "composite expression",
			input:          "Server={db.bindings.tcp.host};Port={db.bindings.tcp.port};Database=mydb",
			expectedParts:  5,
			hasExpressions: true,
		},
		{
			name:           "no references",
			input:          "plain-value",
			expectedParts:  1,
			hasExpressions: false,
		},
		{
			name:           "empty string",
			input:          "",
			expectedParts:  0,
			hasExpressions: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cv := parseExpressions(tt.input)
			if len(cv.parts) != tt.expectedParts {
				t.Errorf("expected %d parts, got %d", tt.expectedParts, len(cv.parts))
			}

			if cv.hasExpressions() != tt.hasExpressions {
				t.Errorf("hasExpressions() = %v, want %v", cv.hasExpressions(), tt.hasExpressions)
			}
		})
	}
}

func TestParseExpressions_SingleReference(t *testing.T) {
	t.Parallel()

	cv := parseExpressions("{api.bindings.http.url}")
	if len(cv.parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(cv.parts))
	}

	expr := cv.parts[0].expression
	if expr == nil {
		t.Fatal("expected expression, got nil")
	}

	if expr.ResourceName != "api" {
		t.Errorf("expected resource name 'api', got %q", expr.ResourceName)
	}

	if len(expr.PropertyPath) != 3 || expr.PropertyPath[0] != "bindings" || expr.PropertyPath[1] != "http" || expr.PropertyPath[2] != "url" {
		t.Errorf("unexpected property path: %v", expr.PropertyPath)
	}
}

func TestParseExpressions_CompositeValue(t *testing.T) {
	t.Parallel()

	cv := parseExpressions("Server={db.bindings.tcp.host};Port={db.bindings.tcp.port};Database=mydb")
	if len(cv.parts) != 5 {
		t.Fatalf("expected 5 parts, got %d", len(cv.parts))
	}

	// Part 0: literal "Server="
	if cv.parts[0].literal != "Server=" {
		t.Errorf("expected literal 'Server=', got %q", cv.parts[0].literal)
	}

	// Part 1: expression {db.bindings.tcp.host}
	if cv.parts[1].expression == nil {
		t.Fatal("expected expression for part 1")
	}

	if cv.parts[1].expression.ResourceName != "db" {
		t.Errorf("expected resource 'db', got %q", cv.parts[1].expression.ResourceName)
	}

	// Part 2: literal ";Port="
	if cv.parts[2].literal != ";Port=" {
		t.Errorf("expected literal ';Port=', got %q", cv.parts[2].literal)
	}

	// Part 3: expression {db.bindings.tcp.port}
	if cv.parts[3].expression == nil {
		t.Fatal("expected expression for part 3")
	}

	// Part 4: literal ";Database=mydb"
	if cv.parts[4].literal != ";Database=mydb" {
		t.Errorf("expected literal ';Database=mydb', got %q", cv.parts[4].literal)
	}
}

func TestDetectCircularReferences(t *testing.T) {
	t.Parallel()

	t.Run("no cycles", func(t *testing.T) {
		t.Parallel()

		ctx := &translationContext{
			manifest: &AspireManifest{
				Resources: map[string]ManifestResource{
					"api":      {Type: "container.v0", Env: map[string]string{"URL": "{frontend.bindings.http.url}"}},
					"frontend": {Type: "container.v0"},
				},
			},
		}

		err := detectCircularReferences(ctx)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("direct cycle via connectionString", func(t *testing.T) {
		t.Parallel()

		ctx := &translationContext{
			manifest: &AspireManifest{
				Resources: map[string]ManifestResource{
					"a": {Type: "value.v0", ConnectionString: "{b.connectionString}"},
					"b": {Type: "value.v0", ConnectionString: "{a.connectionString}"},
				},
			},
		}

		err := detectCircularReferences(ctx)
		if err == nil {
			t.Error("expected circular reference error")
		}
	})
}
