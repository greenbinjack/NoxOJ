# Sprint 2 — Local Dev Environment

**Phase 0 — Foundations & Tooling**

## What was built

- **`Dockerfile`** — a multi-stage build for the API service:
  - Stage 1 (`build`, `golang:1.26-alpine`): downloads Go module dependencies, compiles a static binary (`CGO_ENABLED=0`).
  - Stage 2 (runtime, `alpine:3.20`): copies *only* the compiled binary in, plus `ca-certificates` (needed for any future outbound HTTPS, e.g. the remote-judge crawler much later). The build toolchain, source code, and module cache never make it into the final image.
- **`docker-compose.yml`** — two services on a shared private network:
  - `postgres` (`postgres:16-alpine`), data persisted in a **named volume** (`noxoj_postgres_data`), with a `healthcheck` (`pg_isready`) so Compose can tell "container started" apart from "actually ready to accept connections."
  - `api`, built from the `Dockerfile`, with `depends_on: postgres: condition: service_healthy` — it won't even start until Postgres's healthcheck passes, not just until the postgres *container* exists.
  - Top-level `name: noxoj` — without this, Compose derives the project name from the containing folder (which is `E:\Building OJ`, giving ugly `buildingoj-*` container names); this pins it explicitly.
- **`.env.example`** (committed) documenting the three Postgres env vars Compose reads, with instructions to copy it to `.env` (already gitignored) for real local credentials. Sensible defaults are baked into `docker-compose.yml` itself (`${POSTGRES_USER:-noxoj}` etc.), so the stack works out of the box even without a `.env` file — `.env` only matters once you want to override something.
- **`.dockerignore`** — keeps `.git`, `docs/`, and other non-build content out of the image build context.

**Deliberately not done yet:** the API doesn't actually connect to Postgres. This sprint proves the two containers can run together, networked, with Postgres ready before the API starts — actual database access is Sprint 4 (migrations) and beyond. Building that connection now would mean explaining connection pooling and query patterns before there's any schema to query against.

### How to run it

```bash
docker compose up -d --build   # build + start both containers
docker compose ps              # confirm both are Up (postgres shows "healthy")
curl http://localhost:8081/    # hits the API through its published port
docker compose down            # stop and remove containers (volume persists)
```

## Concepts & theory

### Images vs. containers
An **image** is a read-only, versioned blueprint — built once by a `Dockerfile`, then reused to spin up as many identical containers as needed. A **container** is a running (or stopped) instance of that image, with its own writable layer on top. Rebuilding the image (`docker compose up --build`) doesn't touch containers already running from the old image until you recreate them — this is why `--build` is explicit, not automatic.

### Multi-stage builds
Without a multi-stage build, the final image would contain the entire Go toolchain, the module cache, and the source tree — none of which are needed to *run* the compiled binary, and all of which bloat the image and widen what an attacker could find useful if the container were ever compromised. `FROM golang:... AS build` does the compiling; `FROM alpine:...` starts a *fresh* image and `COPY --from=build` pulls across only the one artifact that matters. The `build` stage's contents never exist in the shipped image at all.

### Named volumes vs. bind mounts (concretely, this time)
Postgres's data directory (`/var/lib/postgresql/data` inside the container) is mapped to a **named volume** (`noxoj_postgres_data`), not a folder in the repo. Docker owns where that volume physically lives on disk — we don't need to know or care, and it survives `docker compose down` (only `docker compose down -v` would delete it). A bind mount would instead be something like `./pgdata:/var/lib/postgresql/data` — a specific repo-relative folder, which would mean database files sitting inside `git status`'s view of the world (and needing yet another `.gitignore` entry) for no benefit, since we never need to browse Postgres's raw data files by hand.

### Health checks vs. "the container started"
Docker considers a container "started" the moment its main process begins running — for Postgres, that's well before it's actually ready to accept connections (it does startup initialization first). Without a `healthcheck`, `depends_on: postgres` only guarantees container-start ordering, not readiness — the `api` service could start and immediately fail to connect in a real scenario where it *did* talk to the database. `condition: service_healthy` closes that gap by waiting for `pg_isready` to actually succeed. This is the same "liveness vs. readiness" distinction Sprint 6 formalizes for our *own* service — seeing it first on Postgres, a system we don't have to build ourselves, makes that concept concrete before we implement it.

## Verification

- `docker compose up -d --build` — both images built/pulled, both containers reached `Up`/`healthy`.
- `docker compose ps` — confirmed `postgres` shows `(healthy)`, `api` shows `Up`.
- `curl http://localhost:8081/` — same response as Sprint 1, now served from inside a container instead of a bare `go run`.
- `docker compose exec postgres pg_isready -U noxoj` — confirmed Postgres genuinely accepting connections, not just "container running."
- `docker compose down` — clean teardown, volume persisted (checked via `docker volume ls`).

## Next sprint

Sprint 3 — CI pipeline basics: GitHub Actions running build + test on every push.
