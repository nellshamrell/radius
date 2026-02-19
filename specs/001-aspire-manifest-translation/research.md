# Research: Aspire Manifest to Radius Translation Layer

**Branch**: `001-aspire-manifest-translation` | **Date**: 2026-02-19

## 1. Aspire Manifest Schema (Input)

### Decision
Parse the Aspire manifest as a JSON file conforming to the `https://json.schemastore.org/aspire-8.0.json` schema. The manifest has no Go types in this codebase — all types must be created from scratch.

### Rationale
No existing Go code in the Radius repository parses Aspire manifests. The `bicep-tools/` package in this repo operates on *Radius Resource Provider manifests* (YAML) for generating Bicep *type extensions* — a completely different input format and purpose. A clean, purpose-built parser is required.

### Alternatives Considered
- **Reuse `bicep-tools/pkg/manifest`**: Rejected — it parses `ResourceProvider` YAML manifests (namespace + types + apiVersions + schemas), not Aspire application manifests (resources with container images, env vars, bindings).
- **Use a third-party Aspire manifest parser**: None exists for Go.

### Manifest Structure

```json
{
  "$schema": "https://json.schemastore.org/aspire-8.0.json",
  "resources": {
    "<name>": { "type": "<type>", ...type-specific fields }
  }
}
```

### Resource Types

| Type | Key Fields |
|------|-----------|
| `container.v0` / `container.v1` | `image`, `entrypoint?`, `args?`, `env?`, `bindings?`, `volumes?`, `bindMounts?`, `connectionString?` |
| `project.v0` / `project.v1` | `path`, `env?`, `bindings?` |
| `value.v0` | `connectionString` |
| `parameter.v0` | `value?`, `secret?` |

### Binding Object Shape

```json
{
  "scheme": "http" | "https" | "tcp",
  "protocol": "tcp",
  "port": 8080,
  "targetPort": 8080,
  "external": true | false
}
```

### Expression Syntax

Expressions use `{...}` delimiters inside env var values and connection strings:
- `{resource.bindings.scheme.property}` — e.g., `{api.bindings.http.url}`, `{db.bindings.tcp.host}`
- `{resource.connectionString}` — e.g., `{cache.connectionString}`
- Composite: `Server={db.bindings.tcp.host};Port={db.bindings.tcp.port};Database=mydb`

The resolver must handle all three patterns, including composite strings with multiple references.

---

## 2. Radius Resource Types (Output)

### Decision
Generate Radius Bicep targeting the `2023-10-01-preview` API version for all resource types. This is the only API version defined across all relevant TypeSpec files.

### Rationale
All TypeSpec definitions in `typespec/Applications.Core/main.tsp`, `typespec/Applications.Datastores/main.tsp`, and `typespec/Applications.Messaging/main.tsp` declare a single version: `v2023_10_01_preview: "2023-10-01-preview"`. There is no newer stable version to target.

### Alternatives Considered
- **Support multiple API versions**: Rejected — only one version exists. Future versions will require a tool update regardless.

### Resource Type Property Maps

#### `Applications.Core/containers@2023-10-01-preview`

```bicep
resource <name> 'Applications.Core/containers@2023-10-01-preview' = {
  name: '<runtime-name>'
  properties: {
    application: <app-ref>
    environment: <env-ref>  // optional but recommended
    container: {
      image: '<image>'
      ports: {
        <name>: { containerPort: <int>, protocol?: 'TCP'|'UDP', scheme?: '<str>' }
      }
      env: {
        <KEY>: { value: '<string>' }
      }
      command: ['<entrypoint>']
      args: ['<arg1>', '<arg2>']
      volumes: {
        <name>: { kind: 'ephemeral'|'persistent', mountPath: '<path>', ... }
      }
    }
    connections: {
      <name>: { source: '<resource-id-or-url>' }
    }
  }
}
```

Key mappings from Aspire:
- `image` → `container.image`
- `entrypoint` → `container.command` (as array)
- `args` → `container.args`
- `env` → `container.env` (values wrapped in `{ value: '...' }` objects)
- `bindings[name].port` → `container.ports[name].containerPort`
- `bindings[name].scheme` → `container.ports[name].scheme`

Note: Radius `EnvironmentVariable` is an object with `value` or `valueFrom`, not a plain string. Env vars resolved from expressions may use Bicep string interpolation in the `value` field, or be moved to the `connections` map.

#### `Applications.Core/gateways@2023-10-01-preview`

```bicep
resource gateway 'Applications.Core/gateways@2023-10-01-preview' = {
  name: 'gateway'
  properties: {
    application: <app-ref>
    routes: [
      { path: '/', destination: '<container-url>' }
    ]
  }
}
```

#### `Applications.Core/applications@2023-10-01-preview`

```bicep
resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: '<app-name>'
  properties: {
    environment: <env-ref>
  }
}
```

This resource is always synthesized — it does not correspond to any Aspire manifest resource.

#### Portable Resources (all use recipe pattern)

All portable resources follow the same pattern:

```bicep
resource <name> '<Type>@2023-10-01-preview' = {
  name: '<runtime-name>'
  properties: {
    application: <app-ref>
    environment: <env-ref>
    resourceProvisioning: 'recipe'
    recipe: { name: 'default' }
  }
}
```

Types:
- `Applications.Datastores/redisCaches` — for Redis images
- `Applications.Datastores/sqlDatabases` — for PostgreSQL/MySQL images
- `Applications.Datastores/mongoDatabases` — for MongoDB images
- `Applications.Messaging/rabbitMQQueues` — for RabbitMQ images

---

## 3. `rad init` Integration

### Decision
Add a `--from-aspire-manifest` flag to the existing `rad init` command that accepts a file path. When set, the command parses the manifest, generates `app.bicep`, and writes it to the current directory (or a user-specified output directory).

### Rationale
The `rad init` command is already the entry point for initializing Radius applications. Adding a flag aligns with the existing UX pattern and avoids creating a separate tool.

### Integration Points

1. **Flag addition**: In `radinit.NewCommand()`, add `cmd.Flags().String("from-aspire-manifest", "", "...")`.
2. **Runner field**: Add `AspireManifestPath string` to the `Runner` struct.
3. **Validate phase**: If the flag is set, read and validate the manifest file.
4. **Run phase**: Call the translation pipeline, write `app.bicep` to the output directory.

### Alternatives Considered
- **Separate `rad aspire translate` command**: Rejected — fragmented UX; the initial spec explicitly recommends `rad init --from-aspire-manifest`.
- **Standalone `manifest-to-bicep` binary**: Rejected — would be a separate distribution concern.

---

## 4. Bicep Text Generation

### Decision
Use Go `text/template` with a single Bicep template to emit the `app.bicep` file. The template receives a structured Go object representing the complete translated application.

### Rationale
The output is a single text file with a well-defined structure. `text/template` is standard Go, has no external dependencies, and is well-suited for text generation with conditional sections. The alternative (string builders/concatenation) is harder to maintain and review.

### Alternatives Considered
- **String builder (`strings.Builder`)**: Rejected — harder to visualize the output structure, more brittle to modify.
- **AST-based Bicep generation**: Rejected — no Go Bicep AST library exists. Overkill for generating a single file.
- **External template engine (e.g., `pongo2`)**: Rejected — unnecessary dependency for straightforward template rendering.

---

## 5. Backing Service Detection Heuristics

### Decision
Use a registry of known image name prefixes mapped to Radius portable resource types. Match against the base image name (final path segment before the tag), case-insensitively. Allow user overrides via a `--resource-override` flag or inline configuration.

### Default Detection Table

| Image Prefix | Radius Type |
|-------------|------------|
| `redis` | `Applications.Datastores/redisCaches` |
| `postgres` | `Applications.Datastores/sqlDatabases` |
| `mysql` | `Applications.Datastores/sqlDatabases` |
| `mongo` | `Applications.Datastores/mongoDatabases` |
| `rabbitmq` | `Applications.Messaging/rabbitMQQueues` |

### Rationale
Image name prefix matching covers the vast majority of real-world cases (Docker Hub official images, Bitnami variants, private registry mirrors). The clarification session confirmed this approach (base image name, case-insensitive). The override mechanism satisfies FR-013.

### Alternatives Considered
- **Exact official image names only**: Rejected — too narrow; misses `bitnami/redis`, `myregistry/redis`, etc.
- **Connection string pattern matching**: Rejected — fragile and not always available in the manifest.
- **Aspire resource type hints**: Not available in the current manifest schema (`container.v0` is generic).

---

## 6. Expression Resolution Strategy

### Decision
Implement a two-pass approach:
1. **Parse**: Scan all string values for `{...}` patterns, extract referenced resource names and property paths.
2. **Resolve**: Based on what the referenced resource maps to in Radius:
   - If it maps to a **portable resource**: Add to `connections` map with `source: <resource>.id`. Remove the env var (Radius auto-injects connection variables).
   - If it maps to a **container**: Resolve to a URL/address constructed from the container's Kubernetes service name and port. Add to `connections` map with `source: '<scheme>://<name>:<port>'`.
   - For **composite expressions**: Use Bicep string interpolation to combine resolved references with literal text.

### Rationale
This is the algorithm described in the initial spec's "Connections Map Generation" section. Separating parse and resolve allows the expression parser to be unit-tested independently.

### Alternatives Considered
- **Single-pass inline resolution**: Rejected — harder to test and doesn't handle forward references well.
- **Leave all references as env vars (no connections)**: Rejected — defeats the purpose of building a Radius application graph.

---

## 7. Bicep Identifier Sanitization

### Decision
Replace hyphens with underscores, strip leading digits (prepend `r_` if the name starts with a digit), remove all other non-alphanumeric/non-underscore characters. Preserve the original name in the `name` property of the Bicep resource.

### Rationale
Bicep identifiers must match `[a-zA-Z_][a-zA-Z0-9_]*`. Aspire resource names are free-form. The clarification session confirmed auto-sanitization with collision detection.

### Sanitization Rules
1. Replace `-` with `_`
2. Remove characters not in `[a-zA-Z0-9_]`
3. If starts with digit, prepend `r_`
4. If collision detected (two names produce same identifier), report error

### Alternatives Considered
- **Reject non-Bicep-safe names**: Rejected — too restrictive for common Aspire naming conventions.
- **Numbered identifiers**: Rejected — poor developer experience (non-descriptive names).
