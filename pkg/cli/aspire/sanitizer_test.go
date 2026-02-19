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

func TestSanitize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "basic name", input: "api", expected: "api"},
		{name: "hyphens to underscores", input: "api-service", expected: "api_service"},
		{name: "leading digit", input: "1cache", expected: "r_1cache"},
		{name: "invalid characters", input: "my.resource@name", expected: "myresourcename"},
		{name: "multiple hyphens", input: "my-cool-service", expected: "my_cool_service"},
		{name: "already valid", input: "myService", expected: "myService"},
		{name: "underscore preserved", input: "my_service", expected: "my_service"},
		{name: "all special chars", input: "!!!", expected: "r_unnamed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := sanitize(tt.input)
			if result != tt.expected {
				t.Errorf("sanitize(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeAll(t *testing.T) {
	t.Parallel()

	t.Run("no collisions", func(t *testing.T) {
		t.Parallel()

		result, err := sanitizeAll([]string{"api", "frontend", "cache"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result["api"] != "api" || result["frontend"] != "frontend" || result["cache"] != "cache" {
			t.Errorf("unexpected mapping: %v", result)
		}
	})

	t.Run("collision detection", func(t *testing.T) {
		t.Parallel()

		_, err := sanitizeAll([]string{"api-service", "api_service"})
		if err == nil {
			t.Fatal("expected collision error")
		}

		if _, ok := err.(*identifierCollisionError); !ok {
			t.Fatalf("expected identifierCollisionError, got %T", err)
		}
	})
}
