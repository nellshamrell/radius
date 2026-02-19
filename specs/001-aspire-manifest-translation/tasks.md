# Tasks: Aspire Manifest to Radius Translation Layer

**Input**: Design documents from `/specs/001-aspire-manifest-translation/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/ (cli-contract.md, go-package-contract.md), quickstart.md

**Tests**: Included — the plan.md specifies table-driven unit tests and golden-file integration tests as project convention.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3, US4)
- Include exact file paths in descriptions

## Path Conventions

Source code lives in:
- `pkg/cli/aspire/` — core translation library (new package)
- `pkg/cli/cmd/radinit/` — `rad init` CLI integration (existing package, new files)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create the new package structure, shared types, and foundational files

- [X] T001 Create package directory and Go types file with all manifest types (AspireManifest, ManifestResource, ManifestBinding, ManifestVolumeMount, ManifestBindMount, ManifestParamInput, ManifestParamDefault, ManifestParamGenerate) in `pkg/cli/aspire/manifest.go`
- [X] T002 [P] Create ResourceKind enum constants and TranslateOptions, TranslateResult, TranslatedResource types in `pkg/cli/aspire/types.go`
- [X] T003 [P] Create output Radius resource types (RadiusResource, ContainerSpec, PortSpec, EnvVarSpec, ConnectionSpec, PortableResourceSpec, GatewaySpec, GatewayRouteSpec, ApplicationSpec) in `pkg/cli/aspire/radius_types.go`
- [X] T004 [P] Create TranslationContext and TranslationConfig intermediate types in `pkg/cli/aspire/context.go`
- [X] T005 [P] Create testdata directory structure and initial simple-containers test fixture JSON in `pkg/cli/aspire/testdata/simple-containers.json`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core components that ALL user stories depend on — manifest parsing, identifier sanitization, expression resolution, and Bicep emission

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T006 Implement Aspire manifest JSON parser (parse file, validate required fields, return typed AspireManifest) in `pkg/cli/aspire/manifest.go`
- [X] T007 Write table-driven unit tests for manifest parser (valid JSON, malformed JSON, missing resources map, missing type field) in `pkg/cli/aspire/manifest_test.go`
- [X] T008 [P] Implement Bicep identifier sanitizer (hyphens→underscores, strip leading digits with `r_` prefix, remove invalid chars, collision detection via sanitizeAll) in `pkg/cli/aspire/sanitizer.go`
- [X] T009 [P] Write table-driven unit tests for sanitizer (basic names, hyphens, leading digits, invalid chars, collision detection) in `pkg/cli/aspire/sanitizer_test.go`
- [X] T010 Implement expression parser — scan strings for `{...}` patterns, extract AspireExpression structs with ResourceName, PropertyPath, RawText; support composite values with multiple references mixed with literals in `pkg/cli/aspire/resolver.go`
- [X] T011 Write table-driven unit tests for expression parser (single reference, connectionString reference, composite expressions, no references, nested braces edge case) in `pkg/cli/aspire/resolver_test.go`
- [X] T012 Implement Bicep template and emitter — use `text/template` to render RadiusResource slice into a valid `app.bicep` string following the ordering guarantees (extension → params → application → portable resources → containers → gateway) in `pkg/cli/aspire/emitter.go`
- [X] T013 Write unit tests for emitter with a minimal RadiusResource set (application + one container) verifying correct Bicep structure and ordering in `pkg/cli/aspire/emitter_test.go`

**Checkpoint**: Parser, sanitizer, expression parser, and emitter are independently tested. User story implementation can now begin.

---

## Phase 3: User Story 1 — Translate Simple Aspire Manifest to Radius Bicep (Priority: P1) 🎯 MVP

**Goal**: Parse an Aspire manifest with `container.v0`/`container.v1` resources, resolve inter-resource expression references into Radius connections, and emit a valid `app.bicep` file

**Independent Test**: Provide a sample Aspire manifest containing containers with env var references and bindings; verify output is a valid Bicep file with correct resource definitions, port mappings, environment variables, and connections map entries

### Implementation for User Story 1

- [X] T014 [US1] Implement resource classifier for container.v0/container.v1 types (returns KindContainer for non-backing-service images, KindUnsupported for unrecognized types, logs warning for skipped resources) — basic classification only, no backing-service detection yet — in `pkg/cli/aspire/mapper.go`
- [X] T015 [P] [US1] Write table-driven unit tests for basic resource classifier (container.v0→Container, container.v1→Container, unknown.v0→Unsupported with warning) in `pkg/cli/aspire/mapper_test.go`
- [X] T016 [US1] Implement expression resolver — resolve parsed expressions to Radius connections and Bicep string interpolation (container→URL source `'<scheme>://<name>:<port>'`, connectionString→container reference, composite expressions→Bicep interpolation syntax) in `pkg/cli/aspire/resolver.go`
- [X] T017 [P] [US1] Write table-driven unit tests for expression resolver (binding URL resolution, connectionString resolution, composite value interpolation, broken reference error, circular reference detection) in `pkg/cli/aspire/resolver_test.go`
- [X] T018 [US1] Implement container resource mapper — convert ManifestResource fields to RadiusResource/ContainerSpec (image→container.image, entrypoint→command, args→args, env→container.env with value objects, bindings→ports, volumes→volumes) in `pkg/cli/aspire/mapper.go`
- [X] T019 [P] [US1] Write table-driven unit tests for container mapper (full field mapping, minimal container, entrypoint+args, volumes, bind mounts) in `pkg/cli/aspire/mapper_test.go`
- [X] T020 [US1] Implement the top-level Translate() orchestrator function — wire together parser→classifier→sanitizer→mapper→resolver→synthesize application→emitter pipeline in `pkg/cli/aspire/translate.go`
- [X] T021 [US1] Write translate integration tests with golden-file comparison using `pkg/cli/aspire/testdata/simple-containers.json` → `pkg/cli/aspire/testdata/simple-containers.bicep` in `pkg/cli/aspire/translate_test.go`
- [X] T022 [P] [US1] Create golden file `pkg/cli/aspire/testdata/simple-containers.bicep` with expected Bicep output for the simple-containers test fixture
- [X] T023 [US1] Implement empty manifest handling — when manifest has no translatable resources, return clear message and empty result in `pkg/cli/aspire/translate.go`
- [X] T024 [US1] Add validation for broken expression references (reference to nonexistent resource) and identifier collisions in `pkg/cli/aspire/translate.go`

**Checkpoint**: User Story 1 is complete. Containers, env vars, ports, connections, and error cases all work. `Translate()` produces valid Bicep for container-only manifests.

---

## Phase 4: User Story 2 — Backing Service Detection and Portable Resource Mapping (Priority: P2)

**Goal**: Automatically detect well-known backing service images and map them to Radius portable resource types with recipe-based provisioning

**Independent Test**: Provide manifests containing known backing service images (redis, postgres, mongo, rabbitmq) and verify output uses portable resource types instead of generic containers

### Implementation for User Story 2

- [X] T025 [US2] Implement backing service image detector — extract base image name (final path segment before tag), compare case-insensitively against known prefix table (redis→redisCaches, postgres→sqlDatabases, mysql→sqlDatabases, mongo→mongoDatabases, rabbitmq→rabbitMQQueues) in `pkg/cli/aspire/detector.go`
- [X] T026 [P] [US2] Write table-driven unit tests for image detector (official images, bitnami variants, private registry mirrors, tagged images, unknown images, case sensitivity) in `pkg/cli/aspire/detector_test.go`
- [X] T027 [US2] Integrate detector into resource classifier — when container image matches a backing service, return the corresponding portable resource KindXxx instead of KindContainer; honor ResourceOverrides from TranslateOptions in `pkg/cli/aspire/mapper.go`
- [X] T028 [P] [US2] Write unit tests for classifier with backing service detection (redis image→KindRedisCache, postgres→KindSQLDB, override resource to KindContainer to force plain container mode) in `pkg/cli/aspire/mapper_test.go`
- [X] T029 [US2] Implement portable resource mapper — generate RadiusResource with PortableResourceSpec (recipe name 'default', resourceProvisioning 'recipe') in `pkg/cli/aspire/mapper.go`
- [X] T030 [US2] Update expression resolver to handle portable resource references — when a referenced resource is a portable resource, set connection source to `<resource>.id` instead of URL in `pkg/cli/aspire/resolver.go`
- [X] T031 [US2] Update emitter template to render portable resources in correct order (after application, before containers) with recipe provisioning syntax in `pkg/cli/aspire/emitter.go`
- [X] T032 [P] [US2] Create test fixture `pkg/cli/aspire/testdata/backing-services.json` and golden file `pkg/cli/aspire/testdata/backing-services.bicep` for backing services scenario
- [X] T033 [US2] Write integration test with golden-file comparison for backing-services scenario in `pkg/cli/aspire/translate_test.go`

**Checkpoint**: User Story 2 is complete. Known backing services are detected, mapped to portable resources with recipe provisioning, and referenced correctly in container connections.

---

## Phase 5: User Story 3 — External Endpoint Gateway Generation (Priority: P3)

**Goal**: Generate `Applications.Core/gateways` resources for container bindings marked `external: true`

**Independent Test**: Provide manifests with external bindings and verify output includes a gateway resource with correct routes

### Implementation for User Story 3

- [X] T034 [US3] Implement gateway synthesizer — scan all container bindings for `external: true`, collect routes (path '/' → destination URL), create GatewaySpec with routes; skip gateway if no external bindings in `pkg/cli/aspire/mapper.go`
- [X] T035 [P] [US3] Write unit tests for gateway synthesizer (single external binding, multiple external bindings across containers, no external bindings→no gateway) in `pkg/cli/aspire/mapper_test.go`
- [X] T036 [US3] Integrate gateway synthesis into Translate() orchestrator — after all containers mapped, synthesize gateway if any external bindings found in `pkg/cli/aspire/translate.go`
- [X] T037 [US3] Update emitter template to render gateway resource section (after containers, with routes array) in `pkg/cli/aspire/emitter.go`
- [X] T038 [P] [US3] Create test fixture `pkg/cli/aspire/testdata/gateway.json` and golden file `pkg/cli/aspire/testdata/gateway.bicep` for gateway scenario
- [X] T039 [US3] Write integration test with golden-file comparison for gateway scenario in `pkg/cli/aspire/translate_test.go`

**Checkpoint**: User Story 3 is complete. External bindings produce gateway resources with correct routes.

---

## Phase 6: User Story 4 — Project Resource Handling with Image Registry Mapping (Priority: P4)

**Goal**: Translate `project.v0`/`project.v1` resources to container resources using user-provided image mappings

**Independent Test**: Provide manifests with project resources and image mappings; verify containers are generated with mapped images and clear errors when mappings are missing

### Implementation for User Story 4

- [X] T040 [US4] Extend resource classifier to handle project.v0/project.v1 types (classify as KindContainer, require image mapping from TranslateOptions.ImageMappings) in `pkg/cli/aspire/mapper.go`
- [X] T041 [P] [US4] Write unit tests for project resource classification (project.v1 with mapping→Container, project.v0 without mapping→error) in `pkg/cli/aspire/mapper_test.go`
- [X] T042 [US4] Implement project-to-container mapper — use image from ImageMappings, map env/bindings same as container.v0, emit clear error listing all project resources missing image mappings in `pkg/cli/aspire/mapper.go`
- [X] T043 [US4] Implement value.v0 resource handling — inline connectionString values into consuming resources' env vars rather than creating standalone resources in `pkg/cli/aspire/mapper.go`
- [X] T044 [US4] Implement parameter.v0 resource handling — generate Bicep `param` declarations (plain or `@secure()` based on `inputs` secret flag) in `pkg/cli/aspire/mapper.go`
- [X] T045 [US4] Update emitter to render Bicep parameters section (after extension, before application resource; `@secure()` annotation for secret params) in `pkg/cli/aspire/emitter.go`
- [X] T046 [P] [US4] Create test fixture `pkg/cli/aspire/testdata/projects.json` and golden file `pkg/cli/aspire/testdata/projects.bicep` for project resources scenario
- [X] T047 [US4] Write integration test with golden-file comparison for projects scenario in `pkg/cli/aspire/translate_test.go`

**Checkpoint**: User Story 4 is complete. Project resources, value resources, and parameter resources all translate correctly.

---

## Phase 7: CLI Integration & End-to-End

**Purpose**: Wire the `pkg/cli/aspire` library into the `rad init` command and create the full-app end-to-end test

- [X] T048 Add `--from-aspire-manifest`, `--app-name`, `--image-mapping`, `--resource-override`, `--output-dir` flags to `rad init` command in `pkg/cli/cmd/radinit/init.go`
- [X] T049 Implement Aspire manifest translation entry point — when `--from-aspire-manifest` is set, skip interactive init prompts, call `aspire.Translate()`, write `app.bicep` to output dir, print summary to stdout, print warnings to stderr in `pkg/cli/cmd/radinit/aspire.go`
- [X] T050 Write unit tests for radinit Aspire integration (flag parsing, translate call, file writing, summary output, error handling) in `pkg/cli/cmd/radinit/aspire_test.go`
- [X] T051 [P] Create full-app test fixture `pkg/cli/aspire/testdata/full-app.json` (containers + backing services + projects + external bindings + value resources + parameters) and golden file `pkg/cli/aspire/testdata/full-app.bicep`
- [X] T052 Write end-to-end integration test using full-app fixture with golden-file comparison in `pkg/cli/aspire/translate_test.go`

**Checkpoint**: Full CLI integration is complete. `rad init --from-aspire-manifest` works end-to-end.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Error handling edge cases, documentation, and cleanup

- [X] T053 [P] Add circular reference detection in expression resolver — detect cycles in resource dependency graph and report clear error in `pkg/cli/aspire/resolver.go`
- [X] T054 [P] Add comprehensive error message tests verifying all error contract patterns from go-package-contract.md (file not found, invalid JSON, missing image mapping, unknown expression ref, identifier collision, unsupported expression, template render failure) in `pkg/cli/aspire/translate_test.go`
- [X] T055 [P] Add edge case tests: empty manifest, single resource, multiple same-port containers, composite expressions with multiple references, resource names requiring sanitization in `pkg/cli/aspire/translate_test.go`
- [X] T056 [P] Verify all generated Bicep in golden files uses correct `extension radius` declaration, proper `@2023-10-01-preview` API version, and follows ordering guarantees from cli-contract.md
- [X] T057 Run `go vet ./pkg/cli/aspire/...` and `go test ./pkg/cli/aspire/...` to verify all tests pass and no lint issues
- [X] T058 Run quickstart.md validation — manually trace through quickstart steps to verify CLI flags, example output, and generated Bicep match implementation

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Setup (Phase 1) — BLOCKS all user stories
- **US1 (Phase 3)**: Depends on Foundational (Phase 2) — core MVP
- **US2 (Phase 4)**: Depends on Foundational (Phase 2); integrates with US1 mapper/resolver/emitter
- **US3 (Phase 5)**: Depends on Foundational (Phase 2); integrates with US1 mapper/emitter
- **US4 (Phase 6)**: Depends on Foundational (Phase 2); integrates with US1 mapper/emitter
- **CLI Integration (Phase 7)**: Depends on US1 (Phase 3) minimum; benefits from US2–US4
- **Polish (Phase 8)**: Depends on all prior phases

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational — no dependencies on other stories
- **User Story 2 (P2)**: Can start after Foundational — extends mapper/resolver/emitter from US1 but operates on distinct code paths (detector, portable resources)
- **User Story 3 (P3)**: Can start after Foundational — extends mapper/emitter from US1 but adds a distinct resource type (gateway)
- **User Story 4 (P4)**: Can start after Foundational — extends mapper/emitter from US1 but adds distinct resource handling (projects, values, parameters)

### Within Each User Story

- Tests are written alongside implementation (Go convention: `_test.go` co-located)
- Types/models before mapping logic
- Mapping logic before resolver updates
- Resolver updates before emitter updates
- Golden files before integration tests
- Integration test confirms the full story works end-to-end

### Parallel Opportunities

- **Phase 1**: T002, T003, T004, T005 can all run in parallel (different files)
- **Phase 2**: T008+T009 (sanitizer) and T006+T007 (parser) can run in parallel; T010+T011 (resolver) in parallel with both; T012+T013 (emitter) in parallel
- **Phase 3**: T015 and T017 and T019 and T022 can run in parallel (test files and golden files)
- **Phase 4**: T026, T028, T032 can run in parallel (test files and fixtures)
- **Phase 5**: T035, T038 can run in parallel
- **Phase 6**: T041, T046 can run in parallel
- **Phase 7**: T051 can run in parallel with T048/T049
- **Phase 8**: T053, T054, T055, T056 can all run in parallel

---

## Parallel Example: User Story 1

```bash
# After Foundational phase completes, launch parallel tasks:
Task T015: "Unit tests for basic resource classifier in pkg/cli/aspire/mapper_test.go"
Task T017: "Unit tests for expression resolver in pkg/cli/aspire/resolver_test.go"
Task T019: "Unit tests for container mapper in pkg/cli/aspire/mapper_test.go"
Task T022: "Create golden file pkg/cli/aspire/testdata/simple-containers.bicep"

# Then sequential tasks that depend on the above:
Task T014: "Implement resource classifier in pkg/cli/aspire/mapper.go"
Task T016: "Implement expression resolver in pkg/cli/aspire/resolver.go"
Task T018: "Implement container resource mapper in pkg/cli/aspire/mapper.go"
Task T020: "Implement Translate() orchestrator in pkg/cli/aspire/translate.go"
Task T021: "Integration test with golden-file in pkg/cli/aspire/translate_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup — types, package structure
2. Complete Phase 2: Foundational — parser, sanitizer, expression parser, emitter
3. Complete Phase 3: User Story 1 — containers, connections, Translate() orchestrator
4. **STOP and VALIDATE**: Run `go test ./pkg/cli/aspire/...` — all tests pass, golden files match
5. Deploy/demo with a simple container-only Aspire manifest

### Incremental Delivery

1. Setup + Foundational → Core infrastructure ready
2. Add User Story 1 → Container translation works → MVP!
3. Add User Story 2 → Backing services auto-detected → Major value add
4. Add User Story 3 → Gateways for external access → Feature complete for web apps
5. Add User Story 4 → Project resources supported → Full Aspire coverage
6. CLI Integration → `rad init --from-aspire-manifest` works end-to-end
7. Polish → Edge cases, validation, documentation

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together
2. Once Foundational is done:
   - Developer A: User Story 1 (core containers + orchestrator)
   - Developer B: User Story 2 (detector + portable resources)
   - Developer C: User Story 3 (gateway synthesis)
3. After US1 is complete:
   - Developer A: User Story 4 (projects + values + params)
   - Developer B: CLI Integration (wiring into rad init)
   - Developer C: Polish (edge cases, error tests)

---

## Notes

- [P] tasks = different files, no dependencies on incomplete tasks
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Golden files are the primary validation mechanism (project convention)
- All generated Bicep targets `@2023-10-01-preview` API version
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- Standard library only — no external dependencies beyond existing go.mod
