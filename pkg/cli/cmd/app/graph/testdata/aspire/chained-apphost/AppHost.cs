// Chained AppHost test fixture with AddSqlServer().AddDatabase() fluent chain.
// The variable 'sqlserver' resolves to the last .Add* in chain (weatherdb).

var builder = DistributedApplication.CreateBuilder(args);

var sqlserver = builder.AddSqlServer("sqlserver")
    .AddDatabase("weatherdb");

var apiservice = builder.AddProject<Projects.AspireApp_ApiService>("apiservice")
    .WithReference(sqlserver);

builder.Build().Run();
