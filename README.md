# League Simulation API

A Go REST backend that simulates a football league: double round-robin
fixtures, Premier-League scoring, live standings, result editing, and
championship predictions.

**Deployed at:** https://football.ibc.lol

## Features

- **Season simulation:** play the next week, play all remaining weeks at once, or reset a league back to an unplayed schedule.
- **Editable results:** change any played match score and the standings recompute on the next read. The table is always derived from matches, never cached.
- **Two prediction strategies** behind one validated contract:
  - **Monte Carlo:** deterministic, ChaCha8-seeded simulation, so the same inputs always produce the same forecast.
  - **AI analyst:** OpenAI Structured Outputs, with a one-shot retry if the model returns a table that fails validation.
- **What-if analysis:** feed in hypothetical match results and get the resulting standings, and optionally a fresh prediction, without changing any real data.
- **Prediction staleness:** every run records the league version it was built from, so a prediction is flagged stale the moment a result changes.
- **Safe writes:** idempotency keys on the play endpoints, an optional API-key guard, and per-client rate limiting.
- **Ways to explore it:** Swagger UI at `/docs/`, a Postman collection, and a single-page web frontend at `/`.

The web frontend in `web/` is intentionally bare: one static page (plain HTML, CSS, and JS) for clicking through the API by hand. It's a placeholder for manual testing, not a real UI.

## Quick start

You need Docker and Docker Compose. Run everything from the repo root:

```bash
cp .env.example .env     # set OPENAI_API_KEY here if you want the AI predictor
make docker-up-all       # build and start MySQL + the API
make migrate-up          # apply database migrations
```

The API listens on http://localhost:8080. From there:

- Swagger UI: http://localhost:8080/docs/
- Web frontend: http://localhost:8080/
- Liveness check: http://localhost:8080/healthz

Create a league from the Swagger UI, the Postman collection, or a `POST /api/v1/leagues` request, then start playing weeks. Without an `OPENAI_API_KEY`, everything still works except the AI predictor, which returns 503.

### Running the API outside Docker

For faster rebuilds while developing, keep MySQL in Docker but run the API on your machine:

```bash
make docker-up           # MySQL only
make migrate-up
make run                 # API on :8080, reads .env automatically
```

## Configuration

The app reads its configuration once at boot from environment variables, loading `.env` if present. The defaults are tuned for local use, so the only variable you must set is `MYSQL_DSN`. Invalid numbers or durations fail fast at startup, and the loader reports every bad value at once rather than stopping at the first.

| Variable | Default | What it does |
|---|---|---|
| `APP_PORT` | `8080` | Port the HTTP server listens on. |
| `APP_ENV` | `local` | Environment label attached to logs. |
| `LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, or `error`. |
| `MYSQL_DSN` | _(required)_ | Go MySQL DSN for the app process. Boot fails if it is empty. |
| `MYSQL_MAX_OPEN_CONNS` | `20` | Maximum open connections in the pool. |
| `MYSQL_MAX_IDLE_CONNS` | `5` | Maximum idle connections kept in the pool. |
| `MYSQL_CONN_MAX_LIFETIME` | `5m` | How long a pooled connection is reused before it is recycled. |
| `RATE_LIMIT_RPS` | `20` | Per-IP requests per second. `0` disables the limiter. |
| `RATE_LIMIT_BURST` | `40` | Per-IP burst allowance on top of the steady rate. |
| `API_KEY` | _(empty)_ | If set, write endpoints require it in the `X-API-Key` header. Empty disables the guard. |
| `OPENAI_API_KEY` | _(empty)_ | Enables the AI predictor. Empty means the `ai_analyst` strategy returns 503. |
| `OPENAI_MODEL` | `gpt-4.1-mini` | Model used for AI predictions. |
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` | Base URL for the OpenAI-compatible API. |
| `OPENAI_TIMEOUT` | `20s` | Per-request timeout for OpenAI calls. |
| `MONTECARLO_SIMULATIONS` | `10000` | Simulations run per Monte Carlo prediction. Must be at least 100. |
| `SHUTDOWN_TIMEOUT` | `15s` | Grace period for in-flight requests during shutdown. Must be greater than 0. |
| `WEB_DIR` | `./web` | Static frontend directory, served at `/`. Set to empty to disable serving. |

Docker Compose uses three more variables to set up the MySQL container:

| Variable | Default | What it does |
|---|---|---|
| `MYSQL_ROOT_PASSWORD` | `root` | Root password for the MySQL container (admin use only). |
| `MYSQL_USER` | `league_api` | Database user for the app and migrations, scoped to the `league` database. |
| `MYSQL_PASSWORD` | `league_api` | Password for `MYSQL_USER`. |

The defaults above are fine for local work. Set strong values for `MYSQL_*` and a non-empty `API_KEY` before exposing the service anywhere public.

## API

All endpoints live under `/api/v1` and speak JSON. The full, authoritative reference is the Swagger UI at `/docs/` and the Postman collection in `api/postman/`. This section is the short version.

### Response envelope

Every response, success or failure, uses the same envelope. A success looks like this:

```json
{
  "data": { "league": { "id": 1, "name": "Demo", "current_week": 3 } },
  "meta": { "request_id": "c0ffee...", "league_version": 7 },
  "error": null
}
```

An error fills in `error` and leaves `data` null:

```json
{
  "data": null,
  "meta": { "request_id": "c0ffee..." },
  "error": {
    "code": "league_already_complete",
    "message": "league is already complete",
    "fields": []
  }
}
```

`meta.league_version` is present on league-scoped responses and bumps on every change, which is what predictions use to detect staleness. On validation failures, `error.fields` lists the offending fields.

### Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/healthz` | Liveness probe. |
| `GET` | `/api/v1/leagues` | List leagues. |
| `POST` | `/api/v1/leagues` | Create a league with its teams and fixtures. |
| `GET` | `/api/v1/leagues/{id}` | Get one league. |
| `DELETE` | `/api/v1/leagues/{id}` | Delete a league. |
| `POST` | `/api/v1/leagues/{id}/reset` | Clear all results, keeping teams and fixtures. |
| `GET` | `/api/v1/leagues/{id}/teams` | List the league's teams. |
| `GET` | `/api/v1/leagues/{id}/fixtures` | List fixtures grouped by week. |
| `GET` | `/api/v1/leagues/{id}/standings` | Current table, computed from results. |
| `GET` | `/api/v1/leagues/{id}/matches` | List matches (optional `?week=`). |
| `GET` | `/api/v1/leagues/{id}/matches/{matchID}` | Get one match. |
| `PATCH` | `/api/v1/leagues/{id}/matches/{matchID}` | Edit a played match's score. |
| `POST` | `/api/v1/leagues/{id}/weeks/next` | Play the next week. |
| `POST` | `/api/v1/leagues/{id}/play-all` | Play all remaining weeks. |
| `GET` | `/api/v1/leagues/{id}/predictions` | List prediction runs, newest first. |
| `POST` | `/api/v1/leagues/{id}/predictions` | Run a prediction (`?strategy=monte_carlo` or `ai_analyst`). |
| `GET` | `/api/v1/leagues/{id}/predictions/{runID}` | Get one prediction run. |
| `POST` | `/api/v1/leagues/{id}/what-if` | Hypothetical standings and prediction from match overrides. |

Two of the write endpoints, `weeks/next` and `play-all`, require an `Idempotency-Key` header so a retried request replays the original result instead of playing twice. When `API_KEY` is set, all write endpoints (`POST`, `PATCH`, `DELETE`) also require the `X-API-Key` header.

### Errors

The `error.code` is a stable string one can match on. Each maps to an HTTP status:

| Status | Example codes |
|---|---|
| 400 Bad Request | `validation_failed`, `bad_request`, `unknown_strategy` |
| 401 Unauthorized | `unauthorized` |
| 404 Not Found | `league_not_found`, `match_not_found`, `prediction_not_found` |
| 409 Conflict | `match_not_played`, `league_already_complete`, `prediction_not_available`, `idempotency_key_conflict` |
| 429 Too Many Requests | `rate_limited` |
| 500 Internal | `internal_error` |
| 502 Bad Gateway | `bad_gateway` (upstream OpenAI failure) |
| 503 Service Unavailable | `ai_disabled` (no `OPENAI_API_KEY`) |

## Predictions

Predictions open once four weeks have been played. A run projects the final table: each team's finishing position, expected points, and championship probability. Two strategies produce that same shape.

- **Monte Carlo** (`monte_carlo`, the default) simulates the remaining fixtures many times, 10,000 by default, and aggregates the outcomes. It is seeded deterministically from the league state, so identical standings always yield an identical forecast.
- **AI analyst** (`ai_analyst`) asks an OpenAI model to fill in the same table under a strict JSON schema. It requires `OPENAI_API_KEY` and returns 503 without one.

Both pass through the same validation: finishing positions form a complete permutation, championship percentages sum to 100, and each team's points agree with its wins, draws, and losses. If the AI predictor returns a table that fails, it gets one chance to correct it.

Each run records the league version it was built from. Editing a result or playing a week bumps that version, so earlier runs are flagged stale instead of quietly going out of date. The what-if endpoint runs the same predictors against a hypothetical schedule without persisting anything.

## Project layout

```
cmd/api              Entry point: config load, dependency wiring, server lifecycle.
internal/
  domain             Core types and rules: leagues, matches, standings, predictions.
  app                Use-case services that orchestrate domain logic and storage.
  predict/           Prediction strategies behind one interface:
    montecarlo         deterministic simulation
    aianalyst          OpenAI-backed predictor
    validating         decorator that enforces the output contract
  storage/
    mysql              MySQL repository implementations
    fakerepo           in-memory repositories for tests
  httpapi            Router, handlers, middleware, DTOs, response envelope.
  external/openai    OpenAI HTTP client.
  platform/
    apperror           typed errors and stable error codes
    random             deterministic seeding helpers
  config             Environment-based configuration loader.
migrations/mysql     SQL schema migrations (golang-migrate).
web                  Static single-page frontend.
api                  OpenAPI spec and Postman collection.
```

The layering runs one way: `httpapi` depends on `app`, `app` depends on `domain`, and storage plus external clients sit behind interfaces that `app` defines. `domain` depends on nothing else in the tree.

## Testing

Tests run at three levels: unit tests, integration tests against a real database, and acceptance tests against a live API. Each has a make target.

| Command | What it covers |
|---|---|
| `make test` | Unit tests across all packages. |
| `make test-race` | The same unit tests under the race detector. |
| `make cover` | Unit tests with a coverage profile, written to `coverage.out` and `coverage.html`. |
| `make test-integration` | Integration tests against a real MySQL started with testcontainers. Needs Docker. |
| `make test-acceptance` | The Postman collection run end to end with newman. Needs the API already running and `newman` installed. |