# NoxOJ

A hybrid, highly-scalable Online Judge platform — combining Codeforces-grade concurrency, DMOJ-grade sandboxing, and VJudge-grade remote-judge federation. Deployable to the cloud or to a disconnected LAN.

This is an active, in-progress build. Current status: **Sprint 1 of 122** (see [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md)).

## Documentation

| File | What it is |
|---|---|
| [ARCHITECTURE.md](ARCHITECTURE.md) | The destination system design — full architecture, security model, database schemas, tech stack rationale |
| [FEATURES.md](FEATURES.md) | The complete feature list (158 features across 12 modules) with a description of what each one does |
| [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md) | The sprint-by-sprint build order — 122 sprints across 14 phases, simple-first, evolving toward the full architecture |
| [docs/sprints/](docs/sprints/) | Per-sprint documentation, written as each sprint completes: what was built, and the concepts/theory behind it |

## Quick start

Requires [Go](https://go.dev/dl/) 1.26+.

```bash
go run ./cmd/api
```

Starts the API skeleton on `:8081`.

## Project structure

```
cmd/api/       entry point for the API service
docs/sprints/  per-sprint build documentation
```

This will grow an `internal/` tree of packages as sprints add real functionality (config, database access, domain logic, etc.) — deliberately not scaffolded ahead of need.

## Branching strategy

Trunk-based development: `main` is always in a working state. Each sprint is built on its own short-lived branch (`sprint-NNN-short-title`), merged back into `main` once that sprint's implementation and documentation are both done. No `develop`/`release` branches — this project only ever ships one version at a time, so GitFlow-style branching would be ceremony without a corresponding problem.

## Versioning

[Semantic versioning](https://semver.org/) (`MAJOR.MINOR.PATCH`), tagged at the end of each phase rather than every sprint. Currently pre-release (`0.x`).
