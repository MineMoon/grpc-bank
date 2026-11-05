# grpc-bank

Демонстрационный банковский сервис на Go: HTTP-шлюз, gRPC-сервисы и PostgreSQL с отдельными базами.

## Архитектура

```
Клиент
  │ HTTP :8080
  ▼
api-gateway
  ├── gRPC :50051 ──▶ account-service ──▶ PostgreSQL (accounts_db)
  └── gRPC :50052 ──▶ payment-service ──▶ PostgreSQL (payments_db)
                           │
                           └── gRPC :50051 ──▶ account-service
```

- `account-service`: счета, пополнение и переводы.
- `payment-service`: идемпотентные платежи; вызывает `account-service` для перевода.
- `api-gateway`: REST поверх обоих gRPC-сервисов.
- Один экземпляр PostgreSQL хранит две отдельные базы.

## Структура репозитория

| Путь | Что внутри |
|---|---|
| `cmd/account-service` | точка входа account-service |
| `cmd/payment-service` | точка входа payment-service |
| `cmd/api-gateway` | точка входа HTTP-шлюза |
| `internal/account` | config, domain, repository, service и handler сервиса счетов |
| `internal/payment` | те же слои для сервиса платежей |
| `internal/gateway` | конфиг, DTO, обработчики и роутер шлюза |
| `api/proto` | исходные описания gRPC |
| `pkg/api` | сгенерированный Go-код (не редактировать вручную) |
| `pkg/postgres` | создание пула pgx |
| `migrations/accounts_db` | миграции базы счетов |
| `migrations/payments_db` | миграции базы платежей |
| `deployments` | docker-compose и init.sql |
| `Makefile` | запуск, сборка, миграции и кодогенерация |
| `AGENTS.md` | краткая памятка для ИИ-агентов |

## Требования

- Go 1.26 или новее.
- Docker и Docker Compose.
- `golang-migrate` в `$(go env GOPATH)/bin/migrate`.
- `protoc`, `protoc-gen-go` и `protoc-gen-go-grpc` только для `make proto`.

Установка `golang-migrate`:

```bash
go install -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

## Быстрый старт

1. Поднять базу, применить миграции и собрать бинарники:
   ```bash
   make start
   ```
   Результат: `bin/account-service`, `bin/payment-service`, `bin/api-gateway`.
2. Запустить сервисы в отдельных терминалах (порядок важен):
   ```bash
   ./bin/account-service
   ./bin/payment-service
   ./bin/api-gateway
   ```
3. Создать два счёта. `user_id` должен быть UUID:
   ```bash
   curl -X POST http://localhost:8080/api/v1/accounts \
     -H 'Content-Type: application/json' \
     -d '{"user_id":"11111111-1111-1111-1111-111111111111","currency":"RUB"}'
   ```
   Ответ содержит `account_id`; его нужно использовать в следующих запросах.
4. Пополнить счёт:
   ```bash
   curl -X POST http://localhost:8080/api/v1/accounts/deposit \
     -H 'Content-Type: application/json' \
     -d '{"account_id":"<UUID>","amount":"1000.00"}'
   ```
5. Перевести деньги:
   ```bash
   curl -X POST http://localhost:8080/api/v1/transfers \
     -H 'Content-Type: application/json' \
     -d '{"idempotency_key":"demo-1","from_account_id":"<UUID>","to_account_id":"<UUID>","amount":"10.00"}'
   ```

## HTTP API

| Метод | Путь | Назначение |
|---|---|---|
| `POST` | `/api/v1/accounts` | создать счёт |
| `GET` | `/api/v1/accounts/{id}` | получить счёт |
| `POST` | `/api/v1/accounts/deposit` | пополнить счёт |
| `POST` | `/api/v1/transfers` | создать перевод |

Суммы передаются строками, например `"1000.00"`. Внутри используется `shopspring/decimal` и `NUMERIC(18,4)`.

## Конфигурация

Все настройки читаются из переменных окружения. Файл `.env` не подхватывается автоматически.

| Сервис | Переменная | Значение по умолчанию |
|---|---|---|
| account-service | `ACCOUNT_SERVICE_GRPC_PORT` | `50051` |
| account-service | `ACCOUNT_SERVICE_DB_URL` | `postgres://postgres:postgresadmin@127.0.0.1:5432/accounts_db?sslmode=disable` |
| payment-service | `PAYMENT_SERVICE_GRPC_PORT` | `50052` |
| payment-service | `PAYMENT_SERVICE_DB_URL` | `postgres://postgres:postgresadmin@127.0.0.1:5432/payments_db?sslmode=disable` |
| payment-service | `ACCOUNT_SERVICE_ADDR` | `127.0.0.1:50051` |
| api-gateway | `GATEWAY_HTTP_PORT` | `8080` |
| api-gateway | `ACCOUNT_SERVICE_ADDR` | `localhost:50051` |
| api-gateway | `PAYMENT_SERVICE_ADDR` | `localhost:50052` |

`POSTGRES_PASSWORD` влияет только на контейнер Postgres. URL миграций в `Makefile` заданы жёстко, поэтому при смене пароля их нужно обновить вручную.

## База данных и миграции

- `deployments/init.sql` создаёт `accounts_db` и `payments_db` только при первой инициализации тома `pgdata`.
- Если база потерялась, пересоздайте том: `make clean-db`, затем `make start`.
- Миграции лежат отдельно для каждой базы: `migrations/accounts_db` и `migrations/payments_db`.
- Новая миграция: пара файлов `<номер>_<имя>.up.sql` и `<номер>_<имя>.down.sql` в нужном каталоге.
- `make migrate-up` применяет обе базы, `make migrate-down` откатывает обе.
- Идентификаторы хранятся как `UUID`, суммы как `NUMERIC(18,4)`.
- Идемпотентность платежей обеспечивает уникальный `transactions.idempotency_key`. Статусы: `PENDING`, `SUCCESS`, `FAILED`.

## Генерация protobuf

Исходники лежат в `api/proto`, сгенерированный код в `pkg/api` уже закоммичен. Меняйте `.proto` и выполняйте:

```bash
make proto
```

`make proto-clean` удаляет сгенерированный код.

## Команды Makefile

| Команда | Действие |
|---|---|
| `make start` | поднять Postgres, применить миграции и собрать бинарники |
| `make build` | собрать три бинарника в `bin/` |
| `make down` | остановить контейнеры |
| `make reset` | перезапустить Postgres без потери данных и пересобрать бинарники |
| `make clean-bin` | удалить `bin/` |
| `make clean-db` | удалить том `pgdata` |
| `make clean-all` | удалить `bin/` и том `pgdata` |
| `make migrate-up` | применить миграции |
| `make migrate-down` | откатить все миграции |
| `make proto` | перегенерировать Go-код из `.proto` |
| `make proto-clean` | удалить сгенерированный код |

## Проверка

Автоматических тестов в репозитории пока нет. Базовая проверка сборки:

```bash
go build ./...
go vet ./...
```

`go vet` сейчас сообщает о двух ошибках форматирования в `pkg/postgres/postgres.go` (аргументы без директив). Их стоит исправить перед добавлением CI.

Ручная проверка: запустить сервисы и выполнить запросы из раздела «Быстрый старт».

## Известные ограничения

- Перевод между сервисами не является распределённой транзакцией: `payment-service` пишет строку `PENDING`, вызывает `account-service`, затем обновляет статус. Если обновление статуса упадёт, деньги уже будут переведены.
- gRPC reflection включён в обоих сервисах для отладки.
- Пароль Postgres по умолчанию (`postgresadmin`) подходит только для локальной разработки.
- CI и автотесты отсутствуют.

## Устранение неполадок

| Проблема | Решение |
|---|---|
| `make start` падает с `migrate: command not found` | установить `golang-migrate` по инструкции выше |
| `database ... does not exist` | выполнить `make clean-db`, затем `make start` |
| `connection refused` на порту 50051 или 50052 | запустить `account-service` и `payment-service` |
| Порт занят | изменить переменную окружения с портом |
