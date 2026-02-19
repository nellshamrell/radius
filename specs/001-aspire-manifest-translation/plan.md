# Implementation Plan: Aspire Manifest to Radius Translation Layer

**Branch**: `001-aspire-manifest-translation` | **Date**: 2026-02-19 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/001-aspire-manifest-translation/spec.md`

## Summary

Build a Go CLI tool (integrated into the `rad` CLI as `rad init --from-aspire-manifest <path>`) that parses an Aspire manifest JSON file, maps its resources to Radius equivalents, resolves inter-resource references into Radius connections, and emits a single `app.bicep` file.

The existing `bicep-tools/` package in this repo converts *Radius Resource Provider manifests* (YAML) into Bicep *type extensions* (types.json, index.json). It is a different tool and shares no direct code with this feature, though it demonstrates Go Bicep generation patterns used in the project.

## Technical Context

**Language/Version**: Go 1.26 (aligned with the repository's `go.mod`)
**Primary Dependencies**: `github.com/spf13/cobra` (CLI framework, already used by `rad` CLI), `encoding/json` (Aspire manifest parsing), `text/template` (Bicep text generation), standard library only for core logic
**Storage**: N/A — reads a JSON file, writes a `.bicep` file. No database or persistent state.
**Testing**: `go test` with table-driven tests (project convention). Unit tests for each package, integration tests with golden-file comparisons.
**Target Platform**: Linux, macOS, Windows — wherever the `rad` CLI runs
**Project Type**: CLI tool — new packages within the existing `rad` CLI monorepo
**Performance Goals**: Translate a 20-resource manifest in under 1 second. FR/SC targets ≤30s for 5+ resources (trivially met).
**Constraints**: No external network calls. No dependencies beyond what the `rad` CLI already has. Must produce Bicep that passes `rad bicep` validation.
**Scale/Scope**: Typical Aspire manifests contain 3–20 resources. The translation is a single-pass, in-memory operation.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

The project constitution (`.specify/memory/constitution.md`) is an unpopulated template — no project-specific rules are defined. All gates pass trivially.

**Pre-Phase 0 check**: PASS (no rules to violate)
**Post-Phase 1 check**: PASS (no rules to violate)

## Project Structure

### Documentation (this feature)

```text
specs/001-aspire-manifest-translation/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
pkg/cli/cmd/radinit/
├── aspire.go                  # New: Aspire manifest translation entry point (called from rad init)
├── aspire_test.go             # New: Tests for aspire.go

pkg/cli/aspire/                # New package: core translation logic
├── manifest.go                # Aspire manifest JSON parsing & types
├── manifest_test.go           # Tests for manifest parsing
├── resolver.go                # Expression resolution ({resource.bindings...})
├── resolver_test.go           # Tests for expression resolution
├── mapper.go                  # Resource type mapping (Aspire → Radius)
├── mapper_test.go             # Tests for resource mapping
├── detector.go                # Backing service image detection heuristics
├── detector_test.go           # Tests for backing service detection
├── sanitizer.go               # Bicep identifier sanitization
├── sanitizer_test.go          # Tests for sanitization
├── emitter.go                 # Bicep text generation (templates → app.bicep)
├── emitter_test.go            # Tests for Bicep generation
├── translate.go               # Orchestrator: manifest → app.bicep pipeline
├── translate_test.go          # Integration tests with golden files
└── testdata/                  # Test fixtures
    ├── simple-containers.json        # Aspire manifest: containers only
    ├── simple-containers.bicep       # Expected golden file
    ├── backing-services.json         # Aspire manifest: Redis, Postgres
    ├── backing-services.bicep        # Expected golden file
    ├── full-app.json                 # Aspire manifest: end-to-end example
    ├── full-app.bicep                # Expected golden file
    ├── gateway.json                  # Aspire manifest: external bindings
    ├── gateway.bicep                 # Expected golden file
    ├── projects.json                 # Aspire manifest: project.v1 resources
    └── projects.bicep                # Expected golden file
```

**Structure Decision**: New code lives in `pkg/cli/aspire/` as a self-contained library package, with integration into the `rad init` command via a new file in `pkg/cli/cmd/radinit/`. This follows the existing project pattern where `radinit/` orchestrates the `rad init` flow and delegates to focused packages.

## Complexity Tracking

No constitution violations — this section is intentionally empty.
