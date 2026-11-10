# AGENTS.md

Go gRPC banking demo. Module `grpc-bank`; human-facing Russian runbook: `README.md`.

## Verification
- `go build ./...`, `make build`, `go vet ./...`, and `go test ./...` pass on the current tree. Keep all four green.
- `go test ./...` runs unit tests only: fake repositories and fake gRPC clients, no Postgres, no network, no running services.
- Single package: `go test ./internal/payment/service`. Single case: `go test ./internal/payment/service -run TestPaymentServiceIdempotency -v`.
- Tests use the standard library only (`testing`, `net/http/httptest`). Do not make the default suite depend on a database, a mocking library, or a live service.
- Repository SQL and transaction paths are intentionally uncovered. Runtime verification of those requires `make start` plus the services.

## Tests
- Tests sit beside their packages: `*_test.go` in `service`, `handler`, `config`, `repository`, `pkg/env`, and `pkg/postgres`.
- Service and gRPC handler tests use a real service with a fake repository. Gateway tests use fake generated gRPC clients with `httptest` and the real chi router.
- Config tests use `t.Setenv`; each subtest sets the variables it needs. Config loading has no built-in defaults.
- `pkg/env` tests cover `.env` parsing, required variables, and DSN composition; `pkg/postgres` is tested only for invalid DSN parsing, before any connection attempt.

## Services and runtime dependencies
- `cmd/account-service`: gRPC `:50051`, database `accounts_db`.
- `cmd/payment-service`: gRPC `:50052`, database `payments_db`; calls account-service over gRPC.
- `cmd/api-gateway`: HTTP REST `:8080`, calls both gRPC services.
- Start order: account-service -> payment-service -> api-gateway. All three need Postgres up and migrated first. `scripts/run.sh` enforces the order and stops all three together.
- One Postgres 16 instance holds two separate databases (`accounts_db`, `payments_db`).

## Commands
- `make start`: creates `.env` from `.env.example` when missing, starts Docker Postgres, waits, runs migrations, runs `make build` into `bin/`, then launches all three services in order via `scripts/run.sh`. It blocks until Ctrl+C; Postgres keeps running until `make down`.
- `make run`: launches the already built services only (no migrations, no rebuild).
- If `migrate` is not at `$(go env GOPATH)/bin/migrate` (for example, installed by mise), pass it explicitly: `make start MIGRATE="$(which migrate)"`.
- Build without touching the DB: `make build`, or `go build -o bin/account-service ./cmd/account-service` (same for `payment-service`, `api-gateway`).
- `make down` / `make reset` (restart, keeps data, does not re-run migrations) / `make clean-db` (deletes `pgdata` volume) / `make clean-all`.
- `bin/` is gitignored; do not commit build artifacts.
- `make migrate-up` / `make migrate-down`: needs `golang-migrate`; the same `MIGRATE=` override applies. URLs are assembled in the Makefile from `.env`.

## Configuration
- `.env` is the single source of settings; `.env.example` is committed, `.env` is gitignored. The Makefile copies the example when `.env` is missing.
- Services load `.env` from the working directory at startup (`pkg/env`); `ENV_FILE` points to another file. Exported variables override file values, and there are no defaults in the config code.
- Required variables: `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_SSLMODE`, `ACCOUNTS_DB_NAME`, `PAYMENTS_DB_NAME`, `ACCOUNT_SERVICE_GRPC_PORT`, `PAYMENT_SERVICE_GRPC_PORT`, `GATEWAY_HTTP_PORT`, `ACCOUNT_SERVICE_ADDR`, `PAYMENT_SERVICE_ADDR`.
- DSNs are built from `POSTGRES_*` plus the database name; `ACCOUNT_SERVICE_DB_URL` and `PAYMENT_SERVICE_DB_URL` override them. Changing the password or port in `.env` is enough for Postgres, migrations, and services.
- Makefile `.env` values must stay free of spaces and `#`, because the file is included as Make syntax.

## Schema and migrations
- `deployments/init.sh` creates the databases from `ACCOUNTS_DB_NAME` and `PAYMENTS_DB_NAME` on the **first** initialization of the `pgdata` volume; it does not re-run. If a database is missing, recreate the volume (`make clean-db`) or create it manually.
- Migrations are split per database: `migrations/accounts_db/`, `migrations/payments_db/`; add numbered `.up.sql`/`.down.sql` pairs to the correct directory.
- IDs are `UUID` in Postgres and strings in Go/proto; `user_id` must be a valid UUID or inserts fail. Money is `NUMERIC(18,4)` plus `shopspring/decimal`; pass amounts as strings over gRPC/JSON. Do not introduce `float`.
- Payment idempotency: `transactions.idempotency_key` is unique; statuses are `PENDING`/`SUCCESS`/`FAILED`.

## Layout conventions
- Per-service layers: `internal/<svc>/{config,domain,repository,service,handler}`. gRPC adapters live in files named `grps.go`; payment config is `internal/payment/config/gonfig.go` (filename typo). Search by package, not filename.
- Generated protobufs in `pkg/api/` are committed; edit `api/proto/` and run `make proto` instead of editing them.
- Gateway routes under `/api/v1`: `POST /accounts`, `GET /accounts/{id}`, `POST /accounts/deposit`, `POST /transfers`.
- Payment flow is not a distributed transaction: payment-service writes a `PENDING` row, calls account-service `ExecuteTransfer`, then updates status. The account transfer is not rolled back if the status update fails.
