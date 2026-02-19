extension radius

@description('The Radius environment ID')
param environment string = 'default'

@description('The Radius application name')
param application string = 'app'

@secure()
@description('Parameter: db-password')
param db_password string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: application
  properties: {
    environment: environment
  }
}

resource webapp 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'webapp'
  properties: {
    application: app.id
    environment: environment
    container: {
      image: 'myregistry.io/webapp:v1.0'
      ports: {
        http: {
          containerPort: 5000
          scheme: 'http'
        }
      }
      env: {
        DB_SECRET: {
          value: '${db_password}'
        }
        WORKER_URL: {
          value: 'http://worker:5001'
        }
      }
    }
    connections: {
      worker: {
        source: 'http://worker:5001'
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
    }
  }
}
