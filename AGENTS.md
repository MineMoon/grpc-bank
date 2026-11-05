# AGENTS.md

Go gRPC banking demo. Module is `grpc-bank` (directory name is `grps-bank`). Human-facing Russian runbook: `README.md`.

## Verification
- `go build ./...` and `make build` pass.
- `go test ./...` finds no test files.
- `go vet ./...` currently fails at `pkg/postgres/postgres.go:14,24`: `fmt.Errorf` receives arguments without formatting directives. Fix those instead of treating the output as a regression from your change.

## Services and runtime dependencies
- `cmd/account-service` — gRPC `:50051`, database `accounts_db`.
- `cmd/payment-service` — gRPC `:50052`, database `payments_db`; calls account-service over gRPC.
- `cmd/api-gateway` — HTTP REST `:8080`, calls both gRPC services.
- Start order: account-service -> payment-service -> api-gateway. All three need Postgres up and migrated first.
- One Postgres 16 instance holds two separate databases (`accounts_db`, `payments_db`).

## Commands
- `make start`: starts Docker Postgres, waits, runs migrations, then runs `make build`, which writes all three binaries to `bin/`.
- Build without touching the DB: `make build`, or `go build -o bin/account-service ./cmd/account-service` (same for `payment-service`, `api-gateway`).
- `make down` / `make reset` (restart, keeps data, does not re-run migrations) / `make clean-db` (deletes `pgdata` volume) / `make clean-all`.
- `bin/` is gitignored; do not commit build artifacts.
- `make migrate-up` / `make migrate-down`: needs `golang-migrate` at `$(go env GOPATH)/bin/migrate` (install command in `README.md`). DB URLs are hardcoded in the Makefile.
- `make proto` — needs `protoc`, `protoc-gen-go`, `protoc-gen-go-grpc`. Regenerates `pkg/api/` from `api/proto/`. `make proto-clean` deletes generated code.

## Configuration (env vars with defaults)
- account: `ACCOUNT_SERVICE_GRPC_PORT` (`50051`), `ACCOUNT_SERVICE_DB_URL` (`postgres://postgres:postgresadmin@127.0.0.1:5432/accounts_db?sslmode=disable`)
- payment: `PAYMENT_SERVICE_GRPC_PORT` (`50052`), `PAYMENT_SERVICE_DB_URL`, `ACCOUNT_SERVICE_ADDR` (`127.0.0.1:50051`)
- gateway: `GATEWAY_HTTP_PORT` (`8080`), `ACCOUNT_SERVICE_ADDR` (`localhost:50051`), `PAYMENT_SERVICE_ADDR` (`localhost:50052`)
- No `.env` loading. Compose honors `POSTGRES_PASSWORD` (default `postgresadmin`) but the Makefile migration URLs do not — overriding the password breaks migrations unless the URLs are changed too.

## Schema and migrations
- `deployments/init.sql` only creates the two databases on the **first** initialization of the `pgdata` volume; it does not re-run. If a database is missing, recreate the volume (`make clean-db`) or create it manually.
- Migrations are split per database: `migrations/accounts_db/`, `migrations/payments_db/`; add numbered `.up.sql`/`.down.sql` pairs to the correct directory.
- IDs are `UUID` in Postgres and strings in Go/proto; `user_id` must be a valid UUID or inserts fail. Money is `NUMERIC(18,4)` + `shopspring/decimal`; pass amounts as strings over gRPC/JSON. Do not introduce `float`.
- Payment idempotency: `transactions.idempotency_key` is unique; statuses are `PENDING`/`SUCCESS`/`FAILED`.

## Layout conventions
- Per-service layers: `internal/<svc>/{config,domain,repository,service,handler}`. gRPC adapters live in files named `grps.go`; payment config is `internal/payment/config/gonfig.go` (filename typo). Search by package, not filename.
- Generated protobufs in `pkg/api/` are committed; edit `api/proto/` and run `make proto` instead of editing them.
- Gateway routes under `/api/v1`: `POST /accounts`, `GET /accounts/{id}`, `POST /accounts/deposit`, `POST /transfers`.
- Payment flow is not a distributed transaction: payment-service writes a `PENDING` row, calls account-service `ExecuteTransfer`, then updates status. The account transfer is not rolled back if the status update fails.
