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

import "strings"

// detectBackingService examines a container image reference and returns the matching
// portable resource kind, or KindUnsupported if no match is found.
//
// The matching rule: extract the base image name (final path segment before the tag),
// then compare case-insensitively against known prefixes.
func detectBackingService(image string) ResourceKind {
	baseName := extractBaseImageName(image)
	lower := strings.ToLower(baseName)

	// Match against known prefixes in priority order.
	for _, entry := range backingServiceTable {
		if strings.HasPrefix(lower, entry.prefix) {
			return entry.kind
		}
	}

	return KindUnsupported
}

// backingServiceEntry maps an image name prefix to a Radius resource kind.
type backingServiceEntry struct {
	prefix string
	kind   ResourceKind
}

// backingServiceTable defines the known backing service image prefixes.
var backingServiceTable = []backingServiceEntry{
	{prefix: "redis", kind: KindRedisCache},
	{prefix: "postgres", kind: KindSQLDB},
	{prefix: "mysql", kind: KindSQLDB},
	{prefix: "mariadb", kind: KindSQLDB},
	{prefix: "mongo", kind: KindMongoDB},
	{prefix: "rabbitmq", kind: KindRabbitMQ},
}

// extractBaseImageName extracts the base image name from a full image reference.
// For example: "docker.io/bitnami/redis:7" → "redis"
//
//	"redis:latest" → "redis"
//	"myregistry.io/library/postgres:14" → "postgres"
func extractBaseImageName(image string) string {
	// Remove tag (everything after the last colon, but handle ports).
	name := image

	// Split on "/" and take the last segment.
	parts := strings.Split(name, "/")
	lastPart := parts[len(parts)-1]

	// Remove tag (after colon).
	if idx := strings.LastIndex(lastPart, ":"); idx != -1 {
		lastPart = lastPart[:idx]
	}

	return lastPart
}
