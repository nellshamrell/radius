extension radius

@description('The Radius environment ID')
param environment string = 'default'

@description('The Radius application name')
param application string = 'app'

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

resource db 'Applications.Datastores/sqlDatabases@2023-10-01-preview' = {
  name: 'db'
  properties: {
    application: app.id
    environment: environment
    resourceProvisioning: 'recipe'
    recipe: {
      name: 'default'
    }
  }
}

resource queue 'Applications.Messaging/rabbitMQQueues@2023-10-01-preview' = {
  name: 'queue'
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
      image: 'myapp/api:latest'
      ports: {
        http: {
          containerPort: 8080
          scheme: 'http'
        }
      }
      env: {
        DB_URL: {
          value: '${db.id}'
        }
        REDIS_URL: {
          value: '${cache.id}'
        }
      }
    }
    connections: {
      cache: {
        source: cache.id
      }
      db: {
        source: db.id
      }
    }
  }
}
