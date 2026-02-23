targetScope = 'subscription'

@description('The environment name')
param environmentName string

@description('The location of the resources')
param location string = resourceGroup().location

module apiservice './apiservice/apiservice.bicep' = {
  name: 'apiservice'
  params: {
    location: location
    environmentName: environmentName
  }
}

module webfrontend './webfrontend/webfrontend.bicep' = {
  name: 'webfrontend'
  params: {
    location: location
    environmentName: environmentName
  }
  dependsOn: [
    apiservice
  ]
}

module cache './cache/cache.bicep' = {
  name: 'cache'
  params: {
    location: location
    environmentName: environmentName
  }
  dependsOn: [
    apiservice
  ]
}
