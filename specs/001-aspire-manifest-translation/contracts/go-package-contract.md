# Go Package Contract: `pkg/cli/aspire`

**Date**: 2026-02-19

## Package Purpose

Translates .NET Aspire application manifests (JSON) into Radius Bicep files. Designed as a library package consumed by `rad init --from-aspire-manifest`.

## Public API

### Entry Point

```go
package aspire

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
    // bypassing automatic backing-service detection. To skip detection and treat
    // a resource as a plain container, map it to KindContainer.
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

// Translate is the top-level entry point. It reads the manifest, runs the full
// translation pipeline, and returns the generated Bicep text.
//
// Errors:
//   - Returns error if ManifestPath does not exist or is not readable.
//   - Returns error if manifest JSON is malformed.
//   - Returns error if any project.v0/v1 resource has no image mapping.
//   - Returns error if expression references nonexistent resources.
//   - Returns error if identifier sanitization produces collisions.
func Translate(opts TranslateOptions) (*TranslateResult, error)
```

### Resource Classification

```go
// ResourceKind enumerates the types of Radius resources that can be produced.
type ResourceKind string

const (
    KindContainer       ResourceKind = "Applications.Core/containers"
    KindRedisCache      ResourceKind = "Applications.Datastores/redisCaches"
    KindSQLDB           ResourceKind = "Applications.Datastores/sqlDatabases"
    KindMongoDB         ResourceKind = "Applications.Datastores/mongoDatabases"
    KindRabbitMQ        ResourceKind = "Applications.Messaging/rabbitMQQueues"
    KindGateway         ResourceKind = "Applications.Core/gateways"
    KindApplication     ResourceKind = "Applications.Core/applications"
    KindValueResource   ResourceKind = "value"       // value.v0 → inlined as env vars
    KindParameter       ResourceKind = "parameter"   // parameter.v0 → Bicep param
    KindUnsupported     ResourceKind = "unsupported"  // skipped with warning
)
```

### Internal Interfaces (package-internal, documented for design clarity)

These interfaces are **not exported** but define the internal pipeline stages:

```go
// parser reads and validates the Aspire manifest JSON.
type parser interface {
    // parse reads the manifest from the given path and returns a typed structure.
    parse(path string) (*AspireManifest, error)
}

// classifier determines ResourceKind for each manifest resource.
type classifier interface {
    // classify returns the ResourceKind for a manifest resource.
    // Uses image name matching, resource type, and overrides.
    classify(resource ManifestResource, overrides map[string]ResourceKind) ResourceKind
}

// sanitizer converts Aspire resource names to valid Bicep identifiers.
type sanitizer interface {
    // sanitize returns a valid Bicep identifier for the given name.
    sanitize(name string) string

    // sanitizeAll sanitizes all names and returns error if any collisions occur.
    sanitizeAll(names []string) (map[string]string, error)
}

// resolver resolves Aspire expressions like {cache.bindings.tcp.port}.
type resolver interface {
    // resolve replaces all expression placeholders in the value with concrete
    // Bicep references, given the full translation context.
    resolve(value string, ctx *translationContext) (string, error)
}

// emitter renders the final Bicep output from translated resources.
type emitter interface {
    // emit generates Bicep source text from the translation context.
    emit(ctx *translationContext) (string, error)
}
```

### Manifest Types (exported for testability)

```go
// AspireManifest represents the top-level Aspire manifest structure.
type AspireManifest struct {
    Schema    string                       `json:"$schema"`
    Resources map[string]ManifestResource  `json:"resources"`
}

// ManifestResource represents a single resource in the Aspire manifest.
type ManifestResource struct {
    Type             string                       `json:"type"`
    Image            string                       `json:"image,omitempty"`
    Entrypoint       string                       `json:"entrypoint,omitempty"`
    Path             string                       `json:"path,omitempty"`
    ConnectionString string                       `json:"connectionString,omitempty"`
    Env              map[string]string             `json:"env,omitempty"`
    Bindings         map[string]ManifestBinding    `json:"bindings,omitempty"`
    Args             []string                      `json:"args,omitempty"`
    Volumes          []ManifestVolumeMount         `json:"volumes,omitempty"`
    BindMounts       []ManifestBindMount           `json:"bindMounts,omitempty"`
    Value            string                        `json:"value,omitempty"`
    Inputs           map[string]ManifestParamInput `json:"inputs,omitempty"`
}

// ManifestBinding represents a network binding on an Aspire resource.
type ManifestBinding struct {
    Scheme     string `json:"scheme,omitempty"`
    Protocol   string `json:"protocol,omitempty"`
    Transport  string `json:"transport,omitempty"`
    Port       int    `json:"port,omitempty"`
    TargetPort int    `json:"targetPort,omitempty"`
    External   bool   `json:"external,omitempty"`
}

// ManifestVolumeMount represents a named volume mount.
type ManifestVolumeMount struct {
    Name   string `json:"name"`
    Target string `json:"target"`
    ReadOnly bool `json:"readOnly,omitempty"`
}

// ManifestBindMount represents a host bind mount.
type ManifestBindMount struct {
    Source   string `json:"source"`
    Target   string `json:"target"`
    ReadOnly bool   `json:"readOnly,omitempty"`
}

// ManifestParamInput defines input configuration for parameter resources.
type ManifestParamInput struct {
    Type    string `json:"type,omitempty"`
    Secret  bool   `json:"secret,omitempty"`
    Default *ManifestParamDefault `json:"default,omitempty"`
}

// ManifestParamDefault defines the default value for a parameter input.
type ManifestParamDefault struct {
    Generate *ManifestParamGenerate `json:"generate,omitempty"`
    Value    string                  `json:"value,omitempty"`
}

// ManifestParamGenerate defines auto-generation config for parameter defaults.
type ManifestParamGenerate struct {
    MinLength int `json:"minLength,omitempty"`
}
```

## Backing Service Detection Rules

| Image Prefix | Radius Resource Type |
|---|---|
| `redis` | `Applications.Datastores/redisCaches` |
| `postgres`, `mysql`, `mariadb` | `Applications.Datastores/sqlDatabases` |
| `mongo` | `Applications.Datastores/mongoDatabases` |
| `rabbitmq` | `Applications.Messaging/rabbitMQQueues` |

Matching rule: the image name (without registry prefix, without tag) is checked for a prefix match against the table above. Example: `docker.io/library/redis:7` → image name `redis` → matches `redis` prefix → `redisCaches`.

## Error Contract

All errors returned by `Translate()` are wrapped with contextual information:

| Condition | Error Message Pattern |
|---|---|
| File not found | `"manifest file not found: %s"` |
| Invalid JSON | `"failed to parse manifest: %w"` |
| Missing image mapping | `"project resource %q requires an image mapping"` |
| Unknown expression ref | `"expression in resource %q references unknown resource %q"` |
| Identifier collision | `"identifier collision: resources %q and %q both produce identifier %q"` |
| Unsupported expression | `"unsupported expression syntax in resource %q: %s"` |
| Template render failure | `"failed to render Bicep template: %w"` |

## Thread Safety

`Translate()` is safe for concurrent use — it holds no shared mutable state.

## Dependencies

- Standard library only (`encoding/json`, `text/template`, `fmt`, `os`, `regexp`, `sort`, `strings`)
- No external dependencies beyond what `go.mod` already provides
