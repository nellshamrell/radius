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

import "fmt"

// identifierCollisionError is returned when two resource names produce the same
// Bicep identifier after sanitization.
type identifierCollisionError struct {
	name1      string
	name2      string
	identifier string
}

func (e *identifierCollisionError) Error() string {
	return fmt.Sprintf("identifier collision: resources %q and %q both produce identifier %q", e.name1, e.name2, e.identifier)
}

// unknownResourceError is returned when an expression references a nonexistent resource.
type unknownResourceError struct {
	sourceResource string
	targetResource string
}

func (e *unknownResourceError) Error() string {
	return fmt.Sprintf("expression in resource %q references unknown resource %q", e.sourceResource, e.targetResource)
}

// missingImageMappingError is returned when a project resource has no image mapping.
type missingImageMappingError struct {
	resourceName string
}

func (e *missingImageMappingError) Error() string {
	return fmt.Sprintf("project resource %q requires an image mapping", e.resourceName)
}

// unsupportedExpressionError is returned when an expression has unsupported syntax.
type unsupportedExpressionError struct {
	resourceName string
	expression   string
}

func (e *unsupportedExpressionError) Error() string {
	return fmt.Sprintf("unsupported expression syntax in resource %q: %s", e.resourceName, e.expression)
}

// circularReferenceError is returned when a circular dependency is detected.
type circularReferenceError struct {
	chain []string
}

func (e *circularReferenceError) Error() string {
	return fmt.Sprintf("circular reference detected: %s", fmt.Sprintf("%v", e.chain))
}
