# Sprint 1 — Project Scaffolding & Git Workflow

**Phase 0 — Foundations & Tooling**

## What was built

- **Go module** (`go.mod`): `module noxoj`, Go 1.26.5. This is the root identity of the whole codebase — every package we write from here on is imported relative to `noxoj/...`.
- **API skeleton** (`cmd/api/main.go`): a [Chi](https://github.com/go-chi/chi) router serving one route, `GET /`, on port `:8081`. Verified by actually running the binary and curling it — not just checking that it compiles.
- **`.gitignore`**: excludes build artifacts (`bin/`, `*.exe`), local secrets (`.env`), editor/IDE files, Claude Code's local settings (`.claude/`), and — a deliberate call for now — the three heavy planning docs (`ARCHITECTURE.md`, `FEATURES.md`, `IMPLEMENTATION_PLAN.md`), which stay as personal local reference rather than tracked repo content. `README.md` *is* tracked, since a repo's front door is expected to be visible even while the deeper planning docs stay private for now.
- **Git repo**: initialized with `main` as the default branch, first commit made directly to `main` (the one exception to the sprint-branch workflow below — there's nothing to branch *from* on the very first commit), remote connected to `git@github.com:greenbinjack/NoxOJ.git`, pushed.

### How to run it

```bash
go run ./cmd/api
curl http://localhost:8081/
```

## Concepts & theory

### Go modules
`go.mod` declares the module's import path and its dependencies with exact versions. `go.sum` is separate — it holds cryptographic checksums of every dependency's exact contents, so a `go build` on any machine either fetches bit-for-bit the same code every time or fails loudly. Without `go.sum`, "works on my machine" could mean "works with whatever version of a dependency happened to be available today," which is exactly the kind of nondeterminism a judge platform — where reproducibility is a stated design principle — can't tolerate anywhere, including its own build.

### Why Chi, and why so little of it
Chi is a *router*, not a framework — it matches HTTP requests to handler functions and lets you compose middleware, but it doesn't dictate project structure, an ORM, or a templating engine the way Spring or Django would. That's the right fit for Go generally (the standard library already handles most of what a "framework" would otherwise own) and specifically for this project, since we're building the pieces (auth, config, DB access) deliberately one sprint at a time rather than inheriting a framework's opinions about all of them at once. Sprint 1's `main.go` is intentionally bare — one route, no middleware, no logging — because health checks and structured logging are Sprint 6's concept to introduce cleanly, not something to half-introduce here and re-explain later.

### Branching strategy: trunk-based vs. GitFlow
GitFlow (`develop`, `release/*`, `hotfix/*`, `main`) exists to manage *multiple simultaneously-supported versions* of a product — e.g., patching v2.3 in production while v3.0 is in development. NoxOJ only ever has one version moving forward, so that problem doesn't exist here, and GitFlow's extra branches would be process for its own sake. Trunk-based development instead keeps one branch (`main`) always in a working state, with short-lived branches per unit of work merged back quickly. From Sprint 2 onward, each sprint gets its own branch (`sprint-NNN-short-title`), merged into `main` once both the implementation and this documentation file are done.

### Semantic versioning
`MAJOR.MINOR.PATCH`. We're pre-1.0 (`0.x`), which is the conventional way of saying "still actively taking shape, no stability guarantees yet." Tags happen at phase boundaries, not every sprint — a tag on every single sprint would be too fine-grained to mean anything.

### `.gitignore` as a design decision, not just hygiene
Two different reasons things get excluded: build artifacts (`bin/`, `*.exe`) are excluded because they're *regenerable* — committing them would mean tracking bytes that `go build` recreates identically from source, pure noise in the diff history. Secrets (`.env`) are excluded because committing them once means they're in git history forever, even after later deletion — impossible to fully undo without rewriting history. The three planning docs are a third, different kind of exclusion: not regenerable, not secret, just a scope decision about what's public-repo-ready right now versus personal working notes — a call that's easy to reverse later (as we just did for `README.md`) since `.gitignore` only affects *future* tracking, not anything already committed.

## Verification

- `go build ./cmd/api` — compiles clean.
- Ran the binary, `curl http://localhost:8081/` returned `200 OK` with the expected body.
- `git log --oneline` shows one commit on `main`, pushed to `origin/main`.

## Next sprint

Sprint 2 — local dev environment: Docker Compose wiring Postgres alongside the Go app container.
