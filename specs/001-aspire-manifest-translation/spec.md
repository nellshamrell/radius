# Feature Specification: Aspire Manifest to Radius Translation Layer

**Feature Branch**: `001-aspire-manifest-translation`  
**Created**: 2026-02-18  
**Status**: Draft  
**Input**: User description: "Translation Layer: Aspire Manifest to Radius Application Graph — parse Aspire manifests, map resource types to Radius equivalents, resolve inter-resource references into connections, and emit Radius Bicep files"

## Clarifications

### Session 2026-02-19

- Q: Should the output be a single Bicep file or multiple files? → A: Single `app.bicep` file containing all translated resources.
- Q: How specific should backing service image matching be? → A: Match on the base image name (final path segment before the tag), case-insensitively. E.g., `redis`, `bitnami/redis`, `myregistry.io/redis` all match on `redis`.
- Q: Should composite expressions (literal text mixed with multiple `{...}` references) be supported? → A: Yes — resolve all `{...}` references inline, preserving surrounding literal text via Bicep string interpolation.
- Q: How should Aspire resource names that are not valid Bicep identifiers be handled? → A: Auto-sanitize Bicep identifiers (e.g., hyphens → underscores, strip leading digits) while preserving the original name in the Radius resource's runtime `name` property.
- Q: Which Bicep API version should the generated resources use? → A: Hardcode the latest supported Radius API version at build time; update only with new tool releases.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Translate a Simple Aspire Manifest to Radius Bicep (Priority: P1)

As a developer with an existing .NET Aspire application, I want to translate my Aspire manifest into Radius Bicep files so that I can deploy my application using Radius without manually rewriting my application definition.

I run a single command, point it at my Aspire manifest file, and receive valid Radius Bicep files that represent the same application topology — containers, backing services, connections between them, and external access points.

**Why this priority**: This is the core value proposition. Without the ability to parse an Aspire manifest and produce valid Radius Bicep, no other functionality matters. Even a minimal translation (containers and connections only) delivers immediate value by eliminating hours of manual conversion work.

**Independent Test**: Can be fully tested by providing a sample Aspire manifest containing containers, environment variable references, and service bindings, then verifying the output is a valid Radius Bicep file with correct resource definitions and connections.

**Acceptance Scenarios**:

1. **Given** an Aspire manifest containing `container.v0` resources with images, ports, and environment variables, **When** the user runs the translation command pointing to that manifest, **Then** the system produces Radius Bicep files with `Applications.Core/containers` resources that have correct image, port, and environment variable mappings.
2. **Given** an Aspire manifest with inter-resource references in environment variables (e.g., `{api.bindings.http.url}`), **When** the translation is performed, **Then** the output Bicep files contain explicit `connections` entries linking the consuming resource to the referenced resource.
3. **Given** an Aspire manifest with no resources, **When** the user runs the translation command, **Then** the system reports a clear message indicating no translatable resources were found and produces no output files.

---

### User Story 2 - Backing Service Detection and Portable Resource Mapping (Priority: P2)

As a developer whose Aspire application uses well-known backing services (Redis, PostgreSQL, MongoDB, RabbitMQ), I want these containers to be automatically recognized and mapped to Radius portable resource types so that my application takes advantage of Radius recipes for infrastructure provisioning.

When a container in my manifest uses a known backing service image (e.g., `redis:latest`, `postgres:16`), the translation layer maps it to the appropriate Radius portable resource type (e.g., `Applications.Datastores/redisCaches`) instead of a generic container, and configures it with recipe-based provisioning.

**Why this priority**: Portable resource mapping is the key differentiator that makes the translation "Radius-native" rather than a simple container-to-container copy. It enables the separation of application definition from infrastructure decisions, which is Radius's core design philosophy.

**Independent Test**: Can be tested by providing Aspire manifests containing known backing service images and verifying the output Bicep uses portable resource types with recipe-based provisioning instead of generic container definitions.

**Acceptance Scenarios**:

1. **Given** an Aspire manifest with a container using `redis:latest` as its image, **When** the translation is performed, **Then** the output Bicep contains an `Applications.Datastores/redisCaches` resource with `resourceProvisioning: 'recipe'` instead of an `Applications.Core/containers` resource.
2. **Given** an Aspire manifest with a container using `postgres:16` as its image, **When** the translation is performed, **Then** the output Bicep contains an `Applications.Datastores/sqlDatabases` resource with recipe-based provisioning.
3. **Given** an Aspire manifest with a container using a custom/unknown image (e.g., `mycompany/custom-service:v2`), **When** the translation is performed, **Then** the container is mapped to a generic `Applications.Core/containers` resource.

---

### User Story 3 - External Endpoint Gateway Generation (Priority: P3)

As a developer with an Aspire application that exposes public-facing endpoints, I want external bindings in my manifest to automatically generate Radius gateway resources so that my application is accessible from outside the cluster after deployment.

When a container binding is marked as `external: true` in the manifest, the translation layer creates an `Applications.Core/gateways` resource with routes pointing to the appropriate container.

**Why this priority**: External access is essential for any web-facing application but is a discrete, additive feature that builds on the core container translation. The application functions without it (internal services still communicate), making it a lower priority than core translation and backing service detection.

**Independent Test**: Can be tested by providing an Aspire manifest with at least one binding marked `external: true` and verifying the output Bicep includes a gateway resource with correct routes.

**Acceptance Scenarios**:

1. **Given** an Aspire manifest where a container's binding has `external: true`, **When** the translation is performed, **Then** the output Bicep includes an `Applications.Core/gateways` resource with a route pointing to that container.
2. **Given** an Aspire manifest where no bindings are marked external, **When** the translation is performed, **Then** no gateway resource is generated.
3. **Given** an Aspire manifest with multiple containers having external bindings, **When** the translation is performed, **Then** the gateway resource includes routes for all externally-exposed containers.

---

### User Story 4 - Project Resource Handling with Image Registry Mapping (Priority: P4)

As a developer whose Aspire application includes `project.v1` resources (.NET projects), I want the translation layer to produce container definitions that reference pre-built container images so that my .NET projects are deployable through Radius.

Since Radius does not build source code, the translation layer requires the user to provide an image registry mapping (either via configuration or command-line options) that maps project names to their published container image references.

**Why this priority**: Project resources are common in Aspire applications but require additional user input (image references), making the workflow slightly more complex. Core container support must work first.

**Independent Test**: Can be tested by providing an Aspire manifest with `project.v1` resources along with an image mapping configuration, then verifying the output Bicep contains container resources with the mapped image references.

**Acceptance Scenarios**:

1. **Given** an Aspire manifest with a `project.v1` resource and a user-provided image mapping (e.g., `api → myregistry.azurecr.io/api:latest`), **When** the translation is performed, **Then** the output Bicep contains an `Applications.Core/containers` resource using the mapped image.
2. **Given** an Aspire manifest with a `project.v1` resource and no image mapping provided, **When** the translation is performed, **Then** the system reports a clear error indicating which project resources need image references.

---

### Edge Cases

- What happens when the manifest contains resource types the translation layer does not recognize (e.g., `executable.v0`, future unknown types)? The system must skip unrecognized resources, emit a warning listing them, and continue translating the rest.
- What happens when an environment variable references a resource that does not exist in the manifest? The system must report a clear error identifying the broken reference and the resource that contains it.
- What happens when the manifest contains circular references between resources? The system must detect the cycle, report it as an error, and not produce invalid output.
- What happens when multiple containers use the same port number? Each container is an independent Radius resource — this is valid and the translation should proceed normally.
- What happens when a container has environment variables that mix literal values and resource references? Literal values are passed through directly; resource references are resolved to connections or Radius-injected variables.
- What happens when an environment variable contains multiple `{...}` references mixed with literal text (composite expression)? Each reference is resolved individually and the result is emitted as Bicep string interpolation preserving all literal segments.
- What happens when two Aspire resource names produce the same Bicep identifier after sanitization (e.g., `api-service` and `api_service`)? The system must report an error identifying the collision and not produce output.
- What happens when the Aspire manifest JSON is malformed or does not conform to the expected schema? The system must fail with a clear parse error message identifying the issue.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST accept an Aspire manifest JSON file as input and parse it according to the Aspire manifest schema (version 8.0+).
- **FR-002**: System MUST synthesize an `Applications.Core/applications` resource in the output, deriving its name from the manifest metadata or a user-provided configuration value.
- **FR-003**: System MUST synthesize or reference an `Applications.Core/environments` resource, using a user-provided environment identifier or defaulting to `default`.
- **FR-004**: System MUST map `container.v0` and `container.v1` manifest resources to `Applications.Core/containers` Radius resources, preserving image, ports, environment variables, args, and volume mounts.
- **FR-005**: System MUST map `project.v0` and `project.v1` manifest resources to `Applications.Core/containers` Radius resources, requiring user-provided image references for each project.
- **FR-006**: System MUST detect well-known backing service images (Redis, PostgreSQL, MySQL/MariaDB, MongoDB, RabbitMQ) by checking whether the base image name (the final path segment before the tag, compared case-insensitively) starts with a known service keyword (e.g., `redis`, `postgres`, `mysql`, `mariadb`, `mongo`, `rabbitmq`). Matched images MUST be mapped to their corresponding Radius portable resource types (`Applications.Datastores/redisCaches`, `Applications.Datastores/sqlDatabases`, `Applications.Datastores/mongoDatabases`, `Applications.Messaging/rabbitMQQueues`) with recipe-based provisioning. Users can override or opt out of detection for specific resources via FR-013.
- **FR-007**: System MUST parse Aspire expression syntax (`{resource.bindings.scheme.property}`, `{resource.connectionString}`) in environment variables and resolve them to Radius connections or environment variable references. The system MUST support composite expressions where a single value contains multiple `{...}` references mixed with literal text (e.g., `Server={db.bindings.tcp.host};Port={db.bindings.tcp.port};Database=mydb`), resolving each reference inline and emitting the result as Bicep string interpolation.
- **FR-008**: System MUST generate a `connections` map on each container resource that explicitly lists all resources it depends on, reconstructing the application dependency graph.
- **FR-009**: System MUST generate `Applications.Core/gateways` resources for container bindings marked as `external: true`, with routes pointing to the appropriate container.
- **FR-010**: System MUST produce a single valid, human-readable Radius Bicep file (`app.bicep`) as output containing all translated resources, deployable with `rad deploy app.bicep` without modification (assuming correct environment and image references).
- **FR-011**: System MUST emit clear, actionable error messages when it encounters unrecognized resource types, broken references, malformed JSON, or missing required configuration (such as image mappings for project resources).
- **FR-012**: System MUST skip unrecognized manifest resource types with a warning and continue translating the remaining resources.
- **FR-013**: System MUST support a user-provided configuration mechanism for overriding the default backing service detection (e.g., mapping a specific container name to a specific Radius resource type, or opting out of automatic detection for a resource).
- **FR-014**: System MUST map container entrypoints to Radius container `command` arrays and container args to Radius container `args` arrays.
- **FR-015**: System MUST map `value.v0` manifest resources (connection string resources) by inlining their values into the `connections` or environment variables of consuming resources, rather than creating standalone Radius resources.
- **FR-016**: System MUST map `parameter.v0` manifest resources to Bicep parameters in the generated output. Plain parameters become standard Bicep parameters; secret parameters become `@secure()` annotated Bicep parameters.
- **FR-017**: System MUST sanitize Aspire resource names to produce valid Bicep identifiers (replacing hyphens with underscores, stripping leading digits, removing other invalid characters) while preserving the original Aspire resource name as the Radius runtime resource `name` property value. If sanitization causes two identifiers to collide, the system MUST report an error.
- **FR-018**: System MUST use a single hardcoded Radius API version for all generated resource type references (e.g., `@2023-10-01-preview`). The API version is determined at build time and updated only when the translation tool is released with newer API version support.

### Key Entities

- **Aspire Manifest**: A JSON file produced by `aspire publish --publisher manifest` that contains the logical application model — resources, bindings, environment variables, and inter-resource references. This is the sole input to the translation layer.
- **Manifest Resource**: An entry in the manifest's `resources` map, identified by name and discriminated by `type` (e.g., `container.v0`, `project.v1`, `value.v0`, `parameter.v0`). Each resource has type-specific properties such as image, bindings, env, and connectionString.
- **Aspire Expression**: A string in the format `{resource.property}` or `{resource.bindings.scheme.property}` used within environment variables and connection strings to reference other resources. The translation layer must parse and resolve these.
- **Radius Application Resource**: The synthesized `Applications.Core/applications` root resource that groups all translated resources into a single Radius application graph.
- **Radius Container Resource**: An `Applications.Core/containers` resource generated from an Aspire container or project, containing image, ports, environment variables, volumes, and a connections map.
- **Radius Portable Resource**: A Radius resource type representing a backing service (e.g., `Applications.Datastores/redisCaches`) generated when a known backing service image is detected, configured with recipe-based provisioning.
- **Connections Map**: A property on Radius container resources that explicitly declares dependencies on other resources, forming the application dependency graph. Built by resolving Aspire expression references.
- **Gateway Resource**: An `Applications.Core/gateways` resource generated from Aspire bindings marked `external: true`, providing external access to application containers.
- **Image Mapping**: A user-provided configuration that maps Aspire project resource names to their pre-built container image references, required because Radius does not build source code.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A developer can translate a representative Aspire manifest (5+ resources including containers, projects, and backing services) into deployable Radius Bicep in under 30 seconds of wall-clock time, including command invocation and file generation.
- **SC-002**: 100% of container resources in a valid Aspire manifest are translated to Radius container resources with correct image, port, and environment variable mappings — verified by deploying the output and confirming all containers run.
- **SC-003**: 100% of inter-resource references in environment variables are resolved to explicit entries in the Radius `connections` map — verified by inspecting the application graph via `rad app graph` and confirming all dependency edges are present.
- **SC-004**: Known backing services (Redis, PostgreSQL, MongoDB, RabbitMQ) are correctly identified and mapped to their Radius portable resource types in at least 95% of cases where standard image naming conventions are used.
- **SC-005**: All generated Bicep files pass Radius Bicep validation without errors — verified by running the Radius Bicep compiler against the output.
- **SC-006**: Developers report that the generated Bicep files are readable and understandable without requiring Aspire knowledge — validated through user review of generated output.
- **SC-007**: When the translation encounters errors (malformed JSON, broken references, missing image mappings), 100% of error messages clearly identify the problem and suggest a corrective action — verified through error scenario testing.
- **SC-008**: The end-to-end workflow from Aspire manifest to deployed Radius application completes in under 5 minutes for a typical application (excluding image build and push time).

## Assumptions

- The Aspire manifest conforms to the Aspire 8.0+ manifest schema. Earlier schema versions are not supported.
- Container images referenced in `container.v0`/`container.v1` resources are already built and available in a container registry accessible to the target Radius environment.
- For `project.v1` resources, the developer has already built and pushed container images and can provide the image references via configuration.
- A Radius environment is already provisioned and accessible. The translation layer does not create or manage Radius environments.
- Backing service detection uses image name conventions as the default heuristic (e.g., an image starting with `redis` maps to `redisCaches`). Users can override this via configuration.
- Secrets referenced in `parameter.v0` resources with `secret: true` are out of scope for the initial version. The translation will generate `@secure()` Bicep parameters as placeholders, and secret values must be supplied at deployment time.
- Round-tripping (Radius Bicep back to Aspire manifest) is not supported.
- Dapr resource types in Aspire manifests are out of scope for the initial version.
- The generated Bicep uses a single hardcoded Radius API version determined at build time. Users who need a different API version can edit the generated output.
- The `WaitFor` / health-check ordering, conditional AppHost logic, Aspire Dashboard integration, local development (DCP) features, and `executable.v0` resources are not translatable and are explicitly out of scope.

## Out of Scope

- Building container images from source code (`.csproj` projects must be pre-built)
- Secret value injection or secret store provisioning (initial version generates parameter placeholders only)
- Dapr resource type mapping (`Applications.Dapr/*`)
- Round-trip translation (Radius Bicep → Aspire manifest)
- Querying the target Radius environment for available recipes
- Aspire Dashboard or local development (DCP) feature translation
- Translation of `executable.v0` resources
- Startup ordering or health-check dependency management
