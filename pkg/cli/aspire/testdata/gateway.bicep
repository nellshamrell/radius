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
