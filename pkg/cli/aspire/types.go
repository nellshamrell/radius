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

// ResourceKind enumerates the types of Radius resources that can be produced.
type ResourceKind string

const (
	// KindContainer maps to Applications.Core/containers.
	KindContainer ResourceKind = "Applications.Core/containers"

	// KindRedisCache maps to Applications.Datastores/redisCaches.
	KindRedisCache ResourceKind = "Applications.Datastores/redisCaches"

	// KindSQLDB maps to Applications.Datastores/sqlDatabases.
	KindSQLDB ResourceKind = "Applications.Datastores/sqlDatabases"

	// KindMongoDB maps to Applications.Datastores/mongoDatabases.
	KindMongoDB ResourceKind = "Applications.Datastores/mongoDatabases"

	// KindRabbitMQ maps to Applications.Messaging/rabbitMQQueues.
	KindRabbitMQ ResourceKind = "Applications.Messaging/rabbitMQQueues"

	// KindGateway maps to Applications.Core/gateways.
	KindGateway ResourceKind = "Applications.Core/gateways"

	// KindApplication maps to Applications.Core/applications.
	KindApplication ResourceKind = "Applications.Core/applications"

	// KindValueResource represents value.v0 resources inlined as env vars.
	KindValueResource ResourceKind = "value"

	// KindParameter represents parameter.v0 resources emitted as Bicep params.
	KindParameter ResourceKind = "parameter"

	// KindUnsupported represents unrecognized resource types that are skipped with a warning.
	KindUnsupported ResourceKind = "unsupported"
)

// IsPortableResource returns true if the kind is a portable resource type.
func (k ResourceKind) IsPortableResource() bool {
	switch k {
	case KindRedisCache, KindSQLDB, KindMongoDB, KindRabbitMQ:
		return true
	default:
		return false
	}
}

// TranslateOptions configures the manifest-to-Bicep translation pipeline.
type TranslateOptions struct {
	// ManifestPath is the file path to the Aspire manifest JSON file.
	ManifestPath string

	// AppName is the Radius application name. When set, the generated Bicep
	// application resource uses this as its name (default: "app").
	AppName string

	// EnvironmentName is the Radius environment name/ID. When set, it is used
	// as the default value for the environment Bicep parameter (default: "default").
	EnvironmentName string

	// ImageMappings maps project.v0/v1 resource names to container image references.
	// Required for every project.v0/v1 resource in the manifest.
	ImageMappings map[string]string

	// ResourceOverrides maps resource names to explicit Radius resource types,
	// bypassing automatic backing-service detection.
	ResourceOverrides map[string]ResourceKind
}

// TranslateResult contains the output of a successful translation.
type TranslateResult struct {
	// Bicep is the generated Bicep source code as a string.
	Bicep string

	// Resources is the list of translated resources (for summary display).
	Resources []TranslatedResource

	// Warnings is a list of non-fatal warning messages produced during translation.
	Warnings []string
}

// TranslatedResource describes a single resource in the translation output.
type TranslatedResource struct {
	// OriginalName is the resource name from the Aspire manifest.
	OriginalName string

	// BicepIdentifier is the sanitized Bicep identifier.
	BicepIdentifier string

	// Kind is the Radius resource kind assigned to this resource.
	Kind ResourceKind

	// Synthesized is true if this resource was auto-generated (e.g., gateway, application).
	Synthesized bool
}
