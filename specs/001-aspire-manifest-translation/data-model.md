# Data Model: Aspire Manifest to Radius Translation Layer

**Branch**: `001-aspire-manifest-translation` | **Date**: 2026-02-19

## Entity Relationship Overview

```
AspireManifest (1) ──contains──> (N) ManifestResource
ManifestResource (1) ──has──> (N) Binding
ManifestResource (1) ──has──> (N) EnvironmentVar (may contain Expressions)
ManifestResource (1) ──references──> (N) ManifestResource (via Expressions)

Translation Pipeline:
  AspireManifest ──parse──> ManifestResource[] ──classify──> RadiusResource[]
  RadiusResource[] ──resolve refs──> RadiusResource[] (with connections)
  RadiusResource[] ──emit──> BicepFile (app.bicep)
```

## Input Entities

### AspireManifest

The top-level input structure parsed from the Aspire manifest JSON file.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `Schema` | `string` | No | JSON schema URL (e.g., `https://json.schemastore.org/aspire-8.0.json`) |
| `Resources` | `map[string]ManifestResource` | Yes | Map of resource name → resource definition |

### ManifestResource

A single resource entry in the Aspire manifest. Discriminated by `Type`.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `Type` | `string` | Yes | Resource type discriminator (e.g., `container.v0`, `project.v1`, `value.v0`, `parameter.v0`) |
| `Image` | `string` | Conditional | Container image reference. Required for `container.v0`/`container.v1`. |
| `Entrypoint` | `string` | No | Container entrypoint override |
| `Args` | `[]string` | No | Container arguments |
| `Env` | `map[string]string` | No | Environment variables. Values may contain expression references. |
| `Bindings` | `map[string]Binding` | No | Named service bindings (ports, protocols, external flag) |
| `Volumes` | `[]VolumeMount` | No | Volume mounts |
| `BindMounts` | `[]BindMount` | No | Bind mounts |
| `ConnectionString` | `string` | No | Connection string (may contain expressions). Used by `container.v*` backing services and `value.v0`. |
| `Path` | `string` | Conditional | Path to `.csproj`. Required for `project.v0`/`project.v1`. |
| `Value` | `string` | No | Default value for `parameter.v0` resources. |
| `Inputs` | `map[string]ManifestParamInput` | No | Input configuration for `parameter.v0` resources. Each key (e.g., `"value"`) maps to a `ManifestParamInput` with `Type`, `Secret`, and optional `Default`. |

### Binding

A service binding (port/protocol/visibility) attached to a container or project resource.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `Scheme` | `string` | No | Protocol scheme (`http`, `https`, `tcp`) |
| `Protocol` | `string` | No | Transport protocol (`tcp`) |
| `Port` | `int` | No | Host-side port |
| `TargetPort` | `int` | No | Container-side port |
| `External` | `bool` | No | Whether this binding is externally accessible |

### VolumeMount

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `Name` | `string` | Yes | Volume name |
| `Target` | `string` | Yes | Mount path in container |
| `ReadOnly` | `bool` | No | Whether the mount is read-only |

### BindMount

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `Source` | `string` | Yes | Source path on host |
| `Target` | `string` | Yes | Mount path in container |

### AspireExpression

A parsed reference extracted from an environment variable value or connection string.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `ResourceName` | `string` | Yes | The referenced resource name |
| `PropertyPath` | `[]string` | Yes | Path segments after the resource name (e.g., `["bindings", "http", "url"]` or `["connectionString"]`) |
| `RawText` | `string` | Yes | The original `{...}` text for error reporting |

### CompositeValue

Represents a string value that may contain zero or more expression references mixed with literal text segments.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `Parts` | `[]ValuePart` | Yes | Ordered sequence of literal strings and expression references |

### ValuePart

A single segment of a composite value — either a literal string or an expression reference.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `Literal` | `string` | Conditional | A literal text segment (mutually exclusive with `Expression`) |
| `Expression` | `*AspireExpression` | Conditional | A parsed expression reference (mutually exclusive with `Literal`) |

---

## Intermediate Entities (Translation Pipeline)

### TranslationContext

Holds the complete state during translation — the parsed manifest, configuration, and accumulated output.

| Field | Type | Description |
|-------|------|-------------|
| `Manifest` | `*AspireManifest` | Parsed input manifest |
| `Config` | `*TranslationConfig` | User-provided configuration |
| `Resources` | `map[string]*RadiusResource` | Translated Radius resources keyed by original Aspire name |
| `Warnings` | `[]string` | Non-fatal warnings (e.g., skipped unknown resource types) |
| `Errors` | `[]error` | Fatal errors accumulated during translation |

### TranslationConfig

User-provided configuration that controls translation behavior.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `AppName` | `string` | No | Application name override. Defaults to `"app"`. |
| `EnvironmentName` | `string` | No | Environment name. Defaults to `"default"`. |
| `ImageMappings` | `map[string]string` | No | Maps project resource names to container image references. Required for `project.v*` resources. |
| `ResourceOverrides` | `map[string]ResourceKind` | No | Override backing service detection for specific resources (FR-013). Maps resource names to explicit Radius resource types. To skip detection and treat as a plain container, map to `KindContainer`. |
| `OutputDir` | `string` | No | Directory to write `app.bicep`. Defaults to current directory. |

---

## Output Entities

### RadiusResource

A translated Radius resource ready for Bicep emission.

| Field | Type | Description |
|-------|------|-------------|
| `BicepIdentifier` | `string` | Sanitized Bicep identifier (e.g., `api_service`) |
| `RuntimeName` | `string` | Original Aspire resource name used as the Radius `name` property (e.g., `api-service`) |
| `RadiusType` | `string` | Fully qualified Radius resource type (e.g., `Applications.Core/containers`) |
| `APIVersion` | `string` | API version (hardcoded `2023-10-01-preview`) |
| `Kind` | `ResourceKind` | Discriminator: `Container`, `PortableResource`, `Gateway`, `Application` |
| `Container` | `*ContainerSpec` | Container details (for `Container` kind) |
| `PortableResource` | `*PortableResourceSpec` | Portable resource details (for `PortableResource` kind) |
| `Gateway` | `*GatewaySpec` | Gateway details (for `Gateway` kind) |
| `Application` | `*ApplicationSpec` | Application details (for `Application` kind) |
| `Connections` | `map[string]ConnectionSpec` | Dependencies on other resources |

### ResourceKind (enum)

| Value | Description |
|-------|-------------|
| `Container` | Maps to `Applications.Core/containers` |
| `PortableResource` | Maps to `Applications.Datastores/*` or `Applications.Messaging/*` |
| `Gateway` | Maps to `Applications.Core/gateways` |
| `Application` | Maps to `Applications.Core/applications` (synthesized) |

### ContainerSpec

| Field | Type | Description |
|-------|------|-------------|
| `Image` | `string` | Container image reference |
| `Command` | `[]string` | Entrypoint command (from Aspire `entrypoint`) |
| `Args` | `[]string` | Container arguments |
| `Env` | `map[string]EnvVarSpec` | Environment variables after expression resolution |
| `Ports` | `map[string]PortSpec` | Port mappings |
| `Volumes` | `map[string]VolumeSpec` | Volume mounts |

### PortSpec

| Field | Type | Description |
|-------|------|-------------|
| `ContainerPort` | `int` | Container-side port number |
| `Protocol` | `string` | Protocol (`TCP`/`UDP`) |
| `Scheme` | `string` | Scheme (`http`/`https`/`tcp`) |

### VolumeSpec

| Field | Type | Description |
|-------|------|-------------|
| `Kind` | `string` | Volume kind (`ephemeral` or `persistent`) |
| `MountPath` | `string` | Path where the volume is mounted in the container |
| `ReadOnly` | `bool` | Whether the volume is mounted read-only |

### EnvVarSpec

| Field | Type | Description |
|-------|------|-------------|
| `Value` | `string` | Resolved literal value (may contain Bicep interpolation syntax) |
| `IsBicepInterpolation` | `bool` | Whether `Value` contains Bicep `'...\${...}...'` interpolation |

### ConnectionSpec

| Field | Type | Description |
|-------|------|-------------|
| `Source` | `string` | Radius resource ID reference (e.g., `cache.id`) or URL (e.g., `http://api:8080`) |
| `IsBicepReference` | `bool` | Whether `Source` is a Bicep expression (e.g., `cache.id`) vs a literal string |

### PortableResourceSpec

| Field | Type | Description |
|-------|------|-------------|
| `RecipeName` | `string` | Recipe name (defaults to `"default"`) |

### GatewaySpec

| Field | Type | Description |
|-------|------|-------------|
| `Routes` | `[]GatewayRouteSpec` | Gateway routes |

### GatewayRouteSpec

| Field | Type | Description |
|-------|------|-------------|
| `Path` | `string` | Route path (e.g., `/`) |
| `Destination` | `string` | Destination URL (e.g., `http://frontend:3000`) |

### ApplicationSpec

| Field | Type | Description |
|-------|------|-------------|
| `EnvironmentRef` | `string` | Reference to the Radius environment |

---

## State Transitions

### Translation Pipeline

```
[1] Parse      → AspireManifest (validated JSON)
[2] Classify   → For each ManifestResource, determine ResourceKind and RadiusType
[3] Sanitize   → Generate BicepIdentifiers, detect collisions
[4] Map        → Convert ManifestResource fields to RadiusResource fields
[5] Resolve    → Parse expressions, build Connections maps
[6] Synthesize → Add Application resource, Gateway resource (if needed)
[7] Validate   → Check for broken references, missing image mappings
[8] Emit       → Render BicepFile from RadiusResource[]
```

### Resource Classification Rules

```
ManifestResource.Type == "container.v0"|"container.v1"
  AND image matches known backing service prefix
    → PortableResource (with detected type)
  ELSE
    → Container

ManifestResource.Type == "project.v0"|"project.v1"
    → Container (requires image mapping)

ManifestResource.Type == "value.v0"
    → No standalone resource (inlined into consumers)

ManifestResource.Type == "parameter.v0"
    → Bicep parameter (not a resource)

ManifestResource.Type == anything else
    → Skip with warning
```

### Validation Rules

| Rule | Applies To | Error Behavior |
|------|-----------|----------------|
| Manifest JSON must be valid | Entire input | Fatal error |
| `resources` map must exist | Entire input | Fatal error |
| `type` field required on every resource | All resources | Fatal error |
| `image` required for `container.v*` | Container resources | Fatal error |
| Image mapping required for `project.v*` | Project resources | Fatal error (lists missing mappings) |
| Expression references must point to existing resources | All expressions | Fatal error |
| Sanitized Bicep identifiers must be unique | All resources | Fatal error |
| No circular references in connections | All connections | Fatal error |
