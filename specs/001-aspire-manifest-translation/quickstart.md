# Quickstart: Aspire Manifest to Radius Bicep Translation

## Prerequisites

- [Radius CLI (`rad`)](https://docs.radapp.io/getting-started/) installed
- A .NET Aspire application with a published manifest

## Step 1: Publish the Aspire Manifest

From your .NET Aspire project directory:

```bash
dotnet run --project MyApp.AppHost -- publish --publisher manifest --output-path ./aspire-manifest.json
```

This produces a JSON manifest describing your application's containers, services, and bindings.

## Step 2: Prepare Image Mappings

Project resources (`project.v0`/`project.v1`) in the Aspire manifest represent .NET projects that need container images. Build and push those images, then note the image references:

```bash
# Build and push your project images
docker build -t myregistry/api:latest ./Api
docker push myregistry/api:latest
```

## Step 3: Generate Bicep

Run the translation command:

```bash
rad init --from-aspire-manifest ./aspire-manifest.json \
  --image-mapping api=myregistry/api:latest \
  --app-name myapp
```

Expected output:

```
Translated 4 resources from Aspire manifest:
  - cache → Applications.Datastores/redisCaches (recipe)
  - api → Applications.Core/containers
  - frontend → Applications.Core/containers
  - gateway (synthesized) → Applications.Core/gateways

Generated: ./app.bicep
```

## Step 4: Review the Generated Bicep

Open `app.bicep` and inspect the generated resources:

```bicep
extension radius

@description('The Radius environment ID')
param environment string

@description('The Radius application name')
param application string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: application
  properties: {
    environment: environment
  }
}

resource cache 'Applications.Datastores/redisCaches@2023-10-01-preview' = {
  name: 'cache'
  properties: {
    application: app.id
    environment: environment
    resourceProvisioning: 'recipe'
    recipe: {
      name: 'default'
    }
  }
}

resource api 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'api'
  properties: {
    application: app.id
    environment: environment
    container: {
      image: 'myregistry/api:latest'
      ports: {
        http: {
          containerPort: 8080
        }
      }
      env: {
        ConnectionStrings__cache: {
          value: '${cache.properties.host}:${cache.properties.port}'
        }
        HTTP_PORTS: {
          value: '8080'
        }
      }
    }
    connections: {
      cache: {
        source: cache.id
      }
    }
  }
}

resource frontend 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'frontend'
  properties: {
    application: app.id
    environment: environment
    container: {
      image: 'myapp/frontend:latest'
      ports: {
        http: {
          containerPort: 3000
        }
      }
      env: {
        API_URL: {
          value: 'http://api:8080'
        }
      }
    }
    connections: {
      api: {
        source: 'http://api:8080'
      }
    }
  }
}

resource gateway 'Applications.Core/gateways@2023-10-01-preview' = {
  name: 'gateway'
  properties: {
    application: app.id
    routes: [
      {
        path: '/'
        destination: 'http://frontend:3000'
      }
    ]
  }
}
```

## Step 5: Deploy

```bash
rad deploy app.bicep \
  -p environment=/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default \
  -p application=myapp
```

## Common Scenarios

### Override Backing Service Detection

If a container image is incorrectly classified (or you want to force container mode):

```bash
rad init --from-aspire-manifest ./aspire-manifest.json \
  --image-mapping api=myregistry/api:latest \
  --resource-override my-custom-redis=Applications.Core/containers
```

### Specify Output Directory

```bash
rad init --from-aspire-manifest ./aspire-manifest.json \
  --image-mapping api=myregistry/api:latest \
  --output-dir ./deploy
```

This writes `deploy/app.bicep` instead of `./app.bicep`.

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `project resource "api" requires an image mapping` | Missing `--image-mapping` for a `project.v0/v1` resource | Add `--image-mapping api=<image>` |
| `expression references unknown resource "db"` | Manifest has `{db.bindings...}` but no resource named `db` | Check manifest for typos or missing resources |
| `identifier collision` | Two resources produce the same Bicep identifier after sanitization | Rename one of the conflicting resources in your Aspire app |
| `Skipping unrecognized resource type` | Resource uses a type not supported by the translator | Expected for types like `executable.v0`; these are warnings |
