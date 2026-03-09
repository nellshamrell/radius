// Simple Aspire AppHost test fixture with 5-resource topology.
// Resources: webfrontend, apiservice, cache, sqlserver, weatherdb
// Connections: webfrontend -> apiservice, webfrontend -> cache, apiservice -> weatherdb

var builder = DistributedApplication.CreateBuilder(args);

var cache = builder.AddRedis("cache");

var sqlserver = builder.AddSqlServer("sqlserver")
    .AddDatabase("weatherdb");

var apiservice = builder.AddProject<Projects.AspireApp_ApiService>("apiservice")
    .WithReference(sqlserver);

builder.AddProject<Projects.AspireApp_Web>("webfrontend")
    .WithExternalHttpEndpoints()
    .WithReference(apiservice)
    .WithReference(cache);

builder.Build().Run();
