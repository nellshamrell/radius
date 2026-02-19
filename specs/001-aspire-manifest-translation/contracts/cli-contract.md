# CLI Contract: `rad init --from-aspire-manifest`

**Date**: 2026-02-19

## Command Signature

```
rad init --from-aspire-manifest <path-to-manifest.json> [flags]
```

## Flags

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `--from-aspire-manifest` | `string` | Yes | — | Path to Aspire manifest JSON file |
| `--app-name` | `string` | No | `"app"` | Name for the Radius application; sets the default value of the `application` Bicep parameter |
| `--environment` | `string` | No | `"default"` | Radius environment name/ID; sets the default value of the `environment` Bicep parameter |
| `--image-mapping` | `string[]` | No | — | Map project resources to images: `<project-name>=<image-ref>` (repeatable) |
| `--resource-override` | `string[]` | No | — | Override resource type mapping: `<resource-name>=<radius-type>` (repeatable) |
| `--output-dir` | `string` | No | `.` (current directory) | Directory to write generated `app.bicep` |

Note: When `--from-aspire-manifest` is provided, the standard `rad init` interactive prompts (cluster selection, cloud providers, recipes) are **skipped**. The command only performs the manifest translation.

## Input

### Aspire Manifest JSON (stdin or file)

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

## Output

### Success (exit code 0)

Writes `app.bicep` to `--output-dir` and prints summary to stdout:

```
Translated 3 resources from Aspire manifest:
  - cache → Applications.Datastores/redisCaches (recipe)
  - api → Applications.Core/containers
  - frontend → Applications.Core/containers
  - gateway (synthesized) → Applications.Core/gateways

Generated: ./app.bicep

Deploy with: rad deploy app.bicep -p environment=<your-env-id> -p application=<your-app-id>
```

### Warnings (exit code 0, warnings to stderr)

```
Warning: Skipping unrecognized resource type 'executable.v0' for resource 'worker'
Warning: Resource 'api-service' name sanitized to Bicep identifier 'api_service'
```

### Errors (exit code 1, errors to stderr)

```
Error: Manifest file not found: ./missing-manifest.json
```

```
Error: Failed to parse manifest: invalid JSON at line 15, column 3
```

```
Error: Project resource 'api' requires an image mapping. Use --image-mapping api=<image-ref>
```

```
Error: Expression reference '{nonexistent.bindings.http.url}' in resource 'frontend' refers to unknown resource 'nonexistent'
```

```
Error: Bicep identifier collision: resources 'api-service' and 'api_service' both produce identifier 'api_service'
```

## Generated Bicep Contract

### File: `app.bicep`

The generated file follows this structure in order:

```bicep
// 1. Extension declaration
extension radius

// 2. Parameters
// When --app-name is provided, generated as: param application string = '<app-name>'
// When --environment is provided, generated as: param environment string = '<environment>'
@description('The Radius environment ID')
param environment string = 'default'

@description('The Radius application name')
param application string = 'app'

// 2a. User parameters (from parameter.v0 resources)
@description('...')
param <paramName> string
// OR for secrets:
@secure()
@description('...')
param <paramName> string

// 3. Application resource (always synthesized)
resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: application
  properties: {
    environment: environment
  }
}

// 4. Portable resources (backing services, sorted alphabetically)
resource <name> '<Type>@2023-10-01-preview' = {
  name: '<runtime-name>'
  properties: {
    application: app.id
    environment: environment
    resourceProvisioning: 'recipe'
    recipe: { name: 'default' }
  }
}

// 5. Container resources (sorted alphabetically)
resource <name> 'Applications.Core/containers@2023-10-01-preview' = {
  name: '<runtime-name>'
  properties: {
    application: app.id
    environment: environment
    container: {
      image: '<image>'
      ports: {
        <name>: { containerPort: <port> }
      }
      env: {
        <KEY>: { value: '<resolved-value>' }
      }
    }
    connections: {
      <name>: { source: <resource>.id }      // portable resource
      <name>: { source: '<scheme>://<name>:<port>' }  // container reference
    }
  }
}

// 6. Gateway resource (if any external bindings exist)
resource gateway 'Applications.Core/gateways@2023-10-01-preview' = {
  name: 'gateway'
  properties: {
    application: app.id
    routes: [
      { path: '/', destination: 'http://<container>:<port>' }
    ]
  }
}
```

### Ordering Guarantees

1. Extension declaration first
2. Parameters second (environment, application, then user params alphabetically)
3. Application resource third
4. Portable resources fourth (alphabetical by Bicep identifier)
5. Container resources fifth (alphabetical by Bicep identifier)
6. Gateway resource last (if present)

This ordering ensures Bicep can resolve forward references (containers reference portable resources, gateway references containers).
