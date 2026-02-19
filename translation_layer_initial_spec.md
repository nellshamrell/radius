# Translation Layer: Aspire Manifest → Radius Application Graph

## Context

The [Aspire-to-Radius Bicep research](export_aspire_bicep_to_radius.md) concluded that the **Aspire manifest** (`aspire publish --publisher manifest`) is a significantly better source than Aspire's Azure-targeted Bicep for generating Radius application graphs. This document examines what a translation layer between the two formats would actually look like — its inputs, outputs, mapping rules, architecture, and open questions.

## Why the Manifest Is the Right Input

The Aspire manifest is a JSON file that captures the **logical application model** before it is lowered to any cloud-specific infrastructure. It contains:

- Resources identified by name with a `type` discriminator (`container.v0`, `project.v1`, `parameter.v0`, `value.v0`, etc.)
- Container images, ports, environment variables, volumes, and bind mounts
- Service bindings with scheme, protocol, port, and external visibility
- Inter-resource references via expression syntax (`{resource.bindings.http.host}`, `{resource.connectionString}`)
- Connection strings for backing services

This operates at the same level of abstraction as Radius's `Applications.*` resource types: both describe *what* the application looks like, not *how* to deploy it to a specific cloud.

## High-Level Architecture

```
┌─────────────────────┐
│  Aspire AppHost     │
│  (C# builder code)  │
└────────┬────────────┘
         │ aspire publish --publisher manifest
         ▼
┌─────────────────────┐
│  manifest.json      │  ← Translation layer input
│  (logical model)    │
└────────┬────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────┐
│              Translation Layer                        │
│                                                       │
│  1. Parse manifest JSON                               │
│  2. Classify each resource by type                    │
│  3. Map to Radius resource types                      │
│  4. Resolve inter-resource references → connections   │
│  5. Emit Radius Bicep files                            │
│                                                       │
└────────┬──────────────────────────────────────────────┘
         │
         ▼
┌──────────────┐
│ Radius Bicep │
│ (app.bicep)  │
└──────────────┘
```

The translation layer targets **Radius Bicep files** — generating `.bicep` files that a developer can inspect, customize, and deploy with `rad deploy`. This is the preferred option as it is transparent and enables human review of the mapping.

## Resource Type Mapping

### Direct Mappings

| Aspire Manifest Type | Aspire Resource Pattern | Radius Resource Type | Confidence |
|---|---|---|---|
| `container.v0` / `container.v1` | Any container with `image` | `Applications.Core/containers` | **High** — direct 1:1 mapping of image, ports, env, volumes |
| `project.v0` / `project.v1` | .NET project (built into container image) | `Applications.Core/containers` | **High** — projects are containers once built; the translation layer just needs the published image reference |
| `container.v0` with Redis image | `image: "redis:latest"` | `Applications.Datastores/redisCaches` | **Medium** — can map by image name convention, or keep as generic container |
| `container.v0` with Postgres image | `image: "postgres:..."` | `Applications.Datastores/sqlDatabases` | **Medium** — same image-convention approach |
| `container.v0` with MongoDB image | `image: "mongo:..."` | `Applications.Datastores/mongoDatabases` | **Medium** — same image-convention approach |
| `container.v0` with RabbitMQ image | `image: "rabbitmq:..."` | `Applications.Messaging/rabbitMQQueues` | **Medium** — maps to messaging resource type |
| `value.v0` | Connection string resource | No direct resource — inlined into `connections` | **High** — becomes a connection source string on the consuming container |
| `parameter.v0` | User-provided parameter | Radius parameter or `Applications.Core/secretStores` | **Medium** — secret parameters map to secretStores; plain parameters become Bicep params |

### What Must Be Synthesized

The Aspire manifest does **not** contain an explicit application resource. The translation layer must:

1. **Create an `Applications.Core/applications` resource** — this is the root of every Radius application graph. It would take its name from the Aspire AppHost project name or a user-supplied configuration.
2. **Create or reference an `Applications.Core/environments` resource** — Radius requires every resource to be scoped to an environment. The translation layer needs a target environment ID (provided as config or defaulting to `default`).

### Mapping Backing Services: Strategy

When the manifest contains a container like `redis:latest`, the recommended strategy is:

**Map to Portable Resources**

Translate known backing services to Radius portable resource types (`Applications.Datastores/redisCaches`, etc.) and let Radius **recipes** handle provisioning. This is the idiomatic Radius approach — the developer declares intent ("I need Redis"), and the platform operator decides implementation.

```bicep
// Generated from manifest container "cache" with image "redis:latest"
resource cache 'Applications.Datastores/redisCaches@2023-10-01-preview' = {
  name: 'cache'
  properties: {
    application: app.id
    environment: env.id
    resourceProvisioning: 'recipe'
    recipe: { name: 'default' }
  }
}
```

This approach is preferred because it aligns with Radius's design philosophy: separating application definition from infrastructure decisions. Backing services should be mapped to their corresponding Radius portable resource types whenever possible, allowing recipes to handle the actual provisioning based on environment configuration.

## Field-Level Mapping Details

### Container / Project → `Applications.Core/containers`

| Aspire Manifest Field | Radius Container Property | Notes |
|---|---|---|
| `image` | `properties.container.image` | Direct mapping |
| `entrypoint` | `properties.container.command` | Aspire `entrypoint` → Radius `command` array |
| `args` | `properties.container.args` | Direct mapping |
| `env` | `properties.container.env` | Values need reference resolution (see below) |
| `bindings[name].port` | `properties.container.ports[name].containerPort` | Port mapping |
| `bindings[name].scheme` | `properties.container.ports[name].protocol` | `http`/`https`/`tcp` |
| `bindings[name].external` | Radius gateway route or `provides` | External bindings may need a `Applications.Core/gateways` resource |
| `volumes[].name` / `target` | `properties.container.volumes` | Volume mount mapping |
| `bindMounts[].source` / `target` | `properties.container.volumes` | Bind mount mapping |
| `build.dockerfile` | *(out of scope)* | Radius doesn't build images — the image must be pre-built and pushed |

### Environment Variable Reference Resolution

Aspire manifest environment variables use expression syntax like:

```json
{
  "ConnectionStrings__cache": "{cache.connectionString}",
  "BACKEND_URL": "{api.bindings.http.url}",
  "DB_HOST": "{postgres.bindings.tcp.host}"
}
```

These must be transformed based on the target resource type:

- **If the referenced resource becomes a Radius portable resource** (e.g., `cache` → `redisCaches`): The connection string is provided by Radius at deployment time via recipe outputs. The env var becomes a reference to the Radius resource's connection properties, or the resource is added to the `connections` map and Radius auto-injects `CONNECTION_<NAME>_<PROPERTY>` variables.
- **If the referenced resource stays a container**: The env var resolves to the container's service discovery address in the Radius environment (typically the Kubernetes DNS name).

The translation layer must parse the `{resource.bindings.scheme.property}` expression syntax, resolve which Aspire resource is referenced, and emit the appropriate Radius connection or environment variable.

### Connections Map Generation

The most critical part of the translation is building the Radius `connections` map. Radius uses this map to construct its application graph (via the `getgraph` API) and to inject connection environment variables.

**Algorithm:**

```
For each resource R in the manifest:
  For each env var or connectionString reference in R that points to resource T:
    If T is mapped to a Radius portable resource (redis, sql, mongo, rabbitmq):
      Add to R's connections: { <T.name>: { source: <T.radiusId> } }
    Else if T is mapped to a Radius container:
      Add to R's connections: { <T.name>: { source: '<scheme>://<T.name>:<port>' } }
```

This reconstructs the dependency edges that Radius needs for its graph. In Aspire, these edges are implicit in env var references; in Radius, they must be explicit in the `connections` property.

### External Endpoints → Gateways

Aspire bindings with `external: true` indicate publicly accessible endpoints. In Radius, this maps to an `Applications.Core/gateways` resource with routes:

```bicep
resource gateway 'Applications.Core/gateways@2023-10-01-preview' = {
  name: 'gateway'
  properties: {
    application: app.id
    routes: [
      {
        destination: frontend.id
        path: '/'
      }
    ]
  }
}
```

The translation layer should scan for external bindings and generate gateway resources as needed.

## Example: End-to-End Translation

### Input: Aspire Manifest

```json
{
  "$schema": "https://json.schemastore.org/aspire-8.0.json",
  "resources": {
    "cache": {
      "type": "container.v0",
      "image": "redis:latest",
      "connectionString": "{cache.bindings.tcp.host}:{cache.bindings.tcp.port}",
      "bindings": {
        "tcp": { "scheme": "tcp", "protocol": "tcp", "port": 6379, "targetPort": 6379 }
      }
    },
    "api": {
      "type": "project.v1",
      "path": "../Api/Api.csproj",
      "env": {
        "ConnectionStrings__cache": "{cache.connectionString}",
        "HTTP_PORTS": "8080"
      },
      "bindings": {
        "http": { "scheme": "http", "protocol": "tcp", "port": 8080, "targetPort": 8080 }
      }
    },
    "frontend": {
      "type": "container.v0",
      "image": "myapp/frontend:latest",
      "env": {
        "API_URL": "{api.bindings.http.url}"
      },
      "bindings": {
        "http": { "scheme": "http", "protocol": "tcp", "port": 3000, "targetPort": 3000, "external": true }
      }
    }
  }
}
```

### Output: Generated Radius Bicep

```bicep
extension radius

@description('The Radius environment ID')
param environment string

@description('The Radius application ID')
param application string

// Backing service: Redis cache via recipe
resource cache 'Applications.Datastores/redisCaches@2023-10-01-preview' = {
  name: 'cache'
  properties: {
    application: application
    environment: environment
    resourceProvisioning: 'recipe'
    recipe: { name: 'default' }
  }
}

// .NET project → container (image must be pre-built and pushed)
resource api 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'api'
  properties: {
    application: application
    container: {
      image: 'myregistry.azurecr.io/api:latest' // resolved from build output
      ports: {
        http: { containerPort: 8080 }
      }
      env: {
        HTTP_PORTS: '8080'
      }
    }
    connections: {
      cache: { source: cache.id }
    }
  }
}

// Frontend container
resource frontend 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'frontend'
  properties: {
    application: application
    container: {
      image: 'myapp/frontend:latest'
      ports: {
        http: { containerPort: 3000 }
      }
      env: {
        API_URL: 'http://api:8080'
      }
    }
    connections: {
      api: { source: 'http://api:8080' }
    }
  }
}

// Gateway for external access to frontend
resource gateway 'Applications.Core/gateways@2023-10-01-preview' = {
  name: 'gateway'
  properties: {
    application: application
    routes: [
      {
        destination: 'http://frontend:3000'
        path: '/'
      }
    ]
  }
}
```

## Implementation Considerations

### Language and Tooling

The translation layer is best implemented as a **`rad` CLI subcommand**. This approach integrates the translation directly into the existing Radius workflow:

```bash
rad init --from-aspire-manifest path/to/manifest.json
```

This command would:
1. Parse the Aspire manifest
2. Generate the Radius Bicep files
3. Initialize the Radius application structure in the current directory

This approach offers several advantages:

- **Integrated into existing Radius workflow** — developers work within the `rad` CLI, which is their primary interface
- **Straightforward distribution** — ships with the Radius CLI, no separate tooling needed
- **Aligns with Radius patterns** — `rad init` is already the entry point for new Radius applications
- **Single command experience** — go from Aspire manifest to Radius application in one step

Alternative implementations include:

| Option | Pros | Cons |
|---|---|---|
| **Go CLI tool** | Aligns with Radius codebase (Go); can import Radius type definitions directly | Separate tool from Aspire ecosystem |
| **C# / .NET tool** | Aligns with Aspire ecosystem; could be an Aspire publisher plugin | Can't easily import Radius Go types |
| **Aspire custom publisher** | Runs as part of `aspire publish --publisher radius`; most seamless DX | Requires changes to Aspire or a publisher extension |

The **`rad` CLI subcommand** is the most practical near-term solution.

### Backing Service Detection

The translation layer needs heuristics to determine when a container resource is actually a backing service that should map to a Radius portable resource type. Approaches:

1. **Image name matching** — match known images (`redis`, `postgres`, `mongo`, `rabbitmq`, `mysql`, `kafka`) to their Radius portable resource types. Fragile but simple.
2. **Aspire resource type hints** — Aspire's in-memory model has specific resource types (`RedisResource`, `PostgresServerResource`) that are richer than the manifest's generic `container.v0`. If the manifest schema evolves to include these type hints, detection becomes trivial.
3. **Connection string pattern matching** — infer the backing service type from the connection string format (e.g., `amqp://` → RabbitMQ, `redis://` → Redis).
4. **User-provided mapping file** — allow users to supply a configuration file that explicitly maps manifest resource names to Radius resource types.

A combination of (1) and (4) is practical today: use image name conventions as defaults, with a config file for overrides.

### Handling `project.v1` Resources

Aspire `project.v1` resources represent .NET projects that must be built into container images before deployment. The manifest contains a `path` to the `.csproj` file but no image reference. The translation layer has two options:

1. **Require pre-built images** — assume the image has already been built and pushed (e.g., via CI/CD), and require the user to provide an image registry mapping.
2. **Integrate with build** — trigger `dotnet publish` to produce a container image, push it to a registry, and use the resulting image tag. This is more complex but more automated.

Option (1) is simpler and aligns with how Radius typically operates — it deploys pre-built artifacts, not source code.

### What Cannot Be Translated

| Aspire Feature | Why It Doesn't Translate |
|---|---|
| `WaitFor` / health-check ordering | Radius uses Kubernetes-native health checks and reconciliation, not startup ordering |
| Conditional logic in AppHost C# | The manifest is a snapshot — any `if/else` logic was already resolved when the manifest was generated |
| Aspire Dashboard integration | Radius has its own dashboard; Aspire's dashboard is specific to the Aspire orchestrator |
| Local development (DCP) features | The translation targets deployment, not local dev; `F5` debugging stays in Aspire |
| `executable.v0` resources | Radius doesn't have a native "run an executable" resource type; these would need manual handling |

## Open Questions

1. **Should the translation layer support round-tripping?** If a developer modifies the generated Radius Bicep, can changes flow back to the Aspire manifest? Likely not worth the complexity.

Let's not support this for now.

2. **How should secrets be handled?** Aspire `parameter.v0` resources with `secret: true` should map to Radius `Applications.Core/secretStores`, but the actual secret values won't be in the manifest. The translation layer needs a strategy for secret injection (environment variables, external secret stores, or Radius secret references).

Let's put this off for now.

3. **Should the layer be aware of Radius recipes?** If the target Radius environment has specific recipes registered (e.g., an Azure Redis recipe vs. a container Redis recipe), should the translation layer query the environment and select the appropriate recipe? This would make it environment-aware but more complex.

Let's put this off for now.

4. **What about Dapr resources?** Aspire has Dapr integrations (`AddDaprStateStore`, `AddDaprPubSub`) that map cleanly to Radius's `Applications.Dapr/*` types. The manifest representation of these resources should be investigated for direct mapping potential.

Let's delay this for now.

## Conclusion

A translation layer from the Aspire manifest to Radius application graphs is **feasible and architecturally sound**. The two models operate at the same level of abstraction — logical application topology — and the mapping between their resource types is largely mechanical:

- **Containers and projects** map to `Applications.Core/containers`
- **Known backing services** map to portable resource types (`Datastores`, `Messaging`, `Dapr`)
- **Environment variable references** translate to explicit `connections` entries
- **External bindings** generate `Applications.Core/gateways`
- **An application root and environment reference** must be synthesized

The primary complexity is not in the type mapping but in **reference resolution** (parsing Aspire's `{resource.property}` expressions) and **backing service detection** (identifying which containers are infrastructure vs. application code). Both are tractable problems with well-defined heuristics.

The most natural implementation would be a **`rad` CLI subcommand** (`rad init --from-aspire-manifest`), enabling developers to go from an Aspire manifest to a deployable Radius application graph in a single step.
