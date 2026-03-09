// Empty AppHost test fixture with no builder.Add* calls.
// Tests empty graph output.

var builder = DistributedApplication.CreateBuilder(args);

builder.Build().Run();
