// Unknown types AppHost test fixture with unrecognized builder.Add* methods.
// Tests fallback to Applications.Core/extenders type + warning generation.

var builder = DistributedApplication.CreateBuilder(args);

var search = builder.AddElasticsearch("search");

var vectordb = builder.AddQdrant("vectordb");

var myservice = builder.AddProject<Projects.MyService>("myservice")
    .WithReference(search)
    .WithReference(vectordb);

builder.Build().Run();
