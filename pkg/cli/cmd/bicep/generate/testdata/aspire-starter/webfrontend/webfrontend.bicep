@description('The environment name')
param environmentName string

@description('The location of the resources')
param location string

resource webfrontend 'Microsoft.App/containerApps@2024-03-01' = {
  name: 'webfrontend'
  location: location
  properties: {
    environmentId: resourceId('Microsoft.App/managedEnvironments', environmentName)
    configuration: {
      ingress: {
        targetPort: 8080
        external: true
        transport: 'http'
      }
    }
    template: {
      containers: [
        {
          name: 'webfrontend'
          image: 'webfrontend:latest'
          env: [
            {
              name: 'ASPNETCORE_URLS'
              value: 'http://+:8080'
            }
            {
              name: 'ConnectionStrings__apiservice'
              value: apiservice.properties.configuration.ingress.fqdn
            }
          ]
        }
      ]
    }
  }
}
