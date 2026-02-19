extension radius

@description('The Radius environment ID')
param environment string = 'default'

@description('The Radius application name')
param application string = 'fullapp'

@secure()
@description('Parameter: api-key')
param api_key string

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
      image: 'myregistry.io/api:v1.0'
      ports: {
        http: {
          containerPort: 8080
          scheme: 'http'
        }
      }
      env: {
        API_KEY: {
          value: '${api_key}'
        }
        DB_CONN: {
          value: '${db.id}'
        }
        HTTP_PORTS: {
          value: '8080'
        }
        QUEUE_URL: {
          value: '${queue.id}'
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
      queue: {
        source: queue.id
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
          scheme: 'http'
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

resource worker 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'worker'
  properties: {
    application: app.id
    environment: environment
    container: {
      image: 'myregistry.io/worker:v1.0'
      ports: {
        http: {
          containerPort: 5001
          scheme: 'http'
        }
      }
      env: {
        QUEUE_URL: {
          value: '${queue.id}'
        }
      }
    }
    connections: {
      queue: {
        source: queue.id
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
