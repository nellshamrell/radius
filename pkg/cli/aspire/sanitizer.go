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
	"regexp"
	"strings"
)

// sanitize converts an Aspire resource name to a valid Bicep identifier.
//
// Rules:
//  1. Replace hyphens with underscores
//  2. Remove characters not in [a-zA-Z0-9_]
//  3. If the result starts with a digit, prepend "r_"
func sanitize(name string) string {
	// Replace hyphens with underscores.
	result := strings.ReplaceAll(name, "-", "_")

	// Remove all characters that are not alphanumeric or underscores.
	re := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	result = re.ReplaceAllString(result, "")

	// If the result starts with a digit, prepend "r_".
	if len(result) > 0 && result[0] >= '0' && result[0] <= '9' {
		result = "r_" + result
	}

	// Handle empty result (e.g., name was all special characters).
	if result == "" {
		result = "r_unnamed"
	}

	return result
}

// sanitizeAll sanitizes all names and returns a map from original names to
// sanitized Bicep identifiers. Returns an error if any collisions occur.
func sanitizeAll(names []string) (map[string]string, error) {
	result := make(map[string]string, len(names))
	reverse := make(map[string]string, len(names)) // sanitized → original

	for _, name := range names {
		sanitized := sanitize(name)

		if existing, ok := reverse[sanitized]; ok {
			return nil, &identifierCollisionError{
				name1:      existing,
				name2:      name,
				identifier: sanitized,
			}
		}

		result[name] = sanitized
		reverse[sanitized] = name
	}

	return result, nil
}
