@description('The environment name')
param environmentName string

@description('The location of the resources')
param location string

resource cache 'Microsoft.Cache/redis@2023-08-01' = {
  name: 'cache'
  location: location
  properties: {
    sku: {
      name: 'Basic'
      family: 'C'
      capacity: 0
    }
    enableNonSslPort: false
    minimumTlsVersion: '1.2'
  }
}
