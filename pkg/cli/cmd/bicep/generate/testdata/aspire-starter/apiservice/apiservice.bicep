@description('The environment name')
param environmentName string

@description('The location of the resources')
param location string

resource apiservice 'Microsoft.App/containerApps@2024-03-01' = {
  name: 'apiservice'
  location: location
  properties: {
    environmentId: resourceId('Microsoft.App/managedEnvironments', environmentName)
    configuration: {
      ingress: {
        targetPort: 8080
        external: false
        transport: 'http'
      }
      secrets: [
        {
          name: 'connectionstrings--cache'
          value: cache.properties.configuration.ingress.fqdn
        }
      ]
    }
    template: {
      containers: [
        {
          name: 'apiservice'
          image: 'apiservice:latest'
          env: [
            {
              name: 'ASPNETCORE_URLS'
              value: 'http://+:8080'
            }
            {
              name: 'ConnectionStrings__cache'
              secretRef: 'connectionstrings--cache'
            }
          ]
        }
      ]
    }
  }
}
