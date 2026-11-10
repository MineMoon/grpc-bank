# grpc-bank

Демонстрационный банковский сервис на Go: HTTP-шлюз, gRPC-сервисы и PostgreSQL с отдельными базами.

## Возможности

- Создание счёта с уникальным номером вида `40817810` + 12 цифр; валюта по умолчанию `RUB`.
- Один счёт на пользователя в каждой валюте: уникальный индекс `user_id` + `currency`, повтор возвращает `409`.
- Пополнение счёта и перевод между счетами.
- Перевод выполняется в одной транзакции с блокировкой строк `FOR UPDATE`; при недостатке средств возвращается `422`.
- Идемпотентные платежи по `idempotency_key`: повтор с тем же ключом не создаёт второй перевод.
- Баланс не может быть отрицательным (`CHECK (balance >= 0)`); деньги хранятся как `NUMERIC(18,4)` и `shopspring/decimal`, без `float`.

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
| `pkg/env` | загрузка `.env` и обязательных переменных |
| `pkg/postgres` | создание пула pgx |
| `migrations/accounts_db` | миграции базы счетов |
| `migrations/payments_db` | миграции базы платежей |
| `deployments` | docker-compose и скрипт создания баз |
| `scripts` | запуск всех сервисов одной командой |
| `.env.example` | шаблон конфигурации, копируется в `.env` |
| `Makefile` | запуск, сборка, миграции и кодогенерация |
| `AGENTS.md` | краткая памятка для ИИ-агентов |

Юнит-тесты лежат рядом с пакетами в файлах `*_test.go`.

## Требования

- Go 1.26 или новее.
- Docker и Docker Compose.
- `golang-migrate` в `$(go env GOPATH)/bin/migrate`.
- `protoc`, `protoc-gen-go` и `protoc-gen-go-grpc` только для `make proto`.

Установка `golang-migrate`:

```bash
go install -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

Если `migrate` установлен не в `$(go env GOPATH)/bin`, передайте путь явно: `make start MIGRATE="$(which migrate)"`.

## Быстрый старт

1. Создать конфигурацию. `make start` создаёт `.env` из `.env.example` автоматически, если файла нет. Чтобы заранее изменить значения, скопируйте шаблон вручную:
   ```bash
   cp .env.example .env
   ```
2. Поднять базу, применить миграции, собрать бинарники и запустить все сервисы:
   ```bash
   make start
   ```
   Команда поднимает Postgres, ждёт готовности, накатывает миграции, собирает `bin/account-service`, `bin/payment-service` и `bin/api-gateway`, затем запускает их по порядку. `Ctrl+C` останавливает сервисы; контейнер Postgres продолжает работать до `make down`.
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

Суммы передаются строками, например `"1000.00"`. Внутри используется `shopspring/decimal` и `NUMERIC(18,4)`.

Тела запросов и ответы:

- `POST /api/v1/accounts` принимает `{"user_id":"<UUID>","currency":"RUB"}` и возвращает `201` с `{"account_id":"...","account_number":"..."}`. Пустая валюта заменяется на `RUB`. Повторный счёт для той же пары пользователь + валюта даёт `409`.
- `GET /api/v1/accounts/{id}` возвращает `{"account_id","user_id","account_number","balance","currency"}` или `404`.
- `POST /api/v1/accounts/deposit` принимает `{"account_id":"<UUID>","amount":"1000.00"}` и возвращает `{"status":"SUCCESS","balance_after":"..."}`.
- `POST /api/v1/transfers` принимает `{"idempotency_key":"...","from_account_id":"<UUID>","to_account_id":"<UUID>","amount":"10.00"}` и возвращает `{"payment_id","status","created_at"}`; `created_at` в формате RFC3339.

Коды ошибок шлюза: `400` для некорректного тела или аргументов, `404` для ненайденного счёта, `409` для существующего счёта, `422` для отклонённого перевода (в том числе недостатка средств и повторного использования ключа идемпотентности в статусе `PENDING` или `FAILED`), `500` для внутренних ошибок.

Повторный перевод с уже успешным `idempotency_key` возвращает существующий платёж со статусом `SUCCESS` и не переводит деньги ещё раз.

## Конфигурация

Все настройки читаются из переменных окружения. Каждый сервис при старте загружает `.env` из текущего каталога; путь можно переопределить через `ENV_FILE`. Уже заданные переменные окружения имеют приоритет над файлом. Значений по умолчанию в коде нет: обязательные переменные перечислены в `.env.example`.

| Переменная | Кто читает | Назначение |
|---|---|---|
| `POSTGRES_USER`, `POSTGRES_PASSWORD` | Postgres, сервисы, миграции | пользователь и пароль |
| `POSTGRES_HOST`, `POSTGRES_PORT` | сервисы, миграции | адрес базы |
| `POSTGRES_SSLMODE` | сервисы, миграции | режим SSL |
| `ACCOUNTS_DB_NAME`, `PAYMENTS_DB_NAME` | Postgres, сервисы | имена баз |
| `ACCOUNT_SERVICE_GRPC_PORT` | account-service | gRPC-порт |
| `PAYMENT_SERVICE_GRPC_PORT` | payment-service | gRPC-порт |
| `GATEWAY_HTTP_PORT` | api-gateway | HTTP-порт |
| `ACCOUNT_SERVICE_ADDR` | payment-service, api-gateway | адрес account-service |
| `PAYMENT_SERVICE_ADDR` | api-gateway | адрес payment-service |
| `ACCOUNT_SERVICE_DB_URL`, `PAYMENT_SERVICE_DB_URL` | сервисы, миграции | полный DSN вместо сборки из `POSTGRES_*` (необязательно) |

`Makefile` собирает DSN из `POSTGRES_*` и имён баз, поэтому смена пароля или порта в `.env` не требует правок в других файлах.

`Makefile` подключает `.env` как фрагмент make-синтаксиса, поэтому значения должны быть без пробелов и символа `#`. Если файла нет, он создаётся из `.env.example` при любом запуске `make`.

## База данных и миграции

- `deployments/init.sh` создаёт базы из `ACCOUNTS_DB_NAME` и `PAYMENTS_DB_NAME` только при первой инициализации тома `pgdata`.
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
| `make start` | поднять Postgres, применить миграции, собрать бинарники и запустить все сервисы |
| `make run` | запустить собранные сервисы без миграций и пересборки |
| `make infra` | поднять только Postgres и дождаться готовности |
| `make build` | собрать три бинарника в `bin/` |
| `make down` | остановить контейнеры |
| `make reset` | перезапустить Postgres без потери данных и пересобрать бинарники |
| `make clean` | удалить `bin/` (то же, что `clean-bin`) |
| `make clean-bin` | удалить `bin/` |
| `make clean-db` | удалить том `pgdata` |
| `make clean-all` | удалить `bin/` и том `pgdata` |
| `make migrate-up` | применить миграции |
| `make migrate-down` | откатить все миграции |
| `make proto` | перегенерировать Go-код из `.proto` |
| `make proto-clean` | удалить сгенерированный код |

## Недавние изменения

- Конфигурация перенесена в `.env`: сервисы загружают файл через `pkg/env`, `ENV_FILE` указывает другой путь, экспортированные переменные окружения имеют приоритет над файлом.
- `make start` теперь создаёт `.env` из `.env.example`, поднимает Postgres, применяет миграции, собирает бинарники и запускает все три сервиса по порядку через `scripts/run.sh`.
- `deployments/init.sh` создаёт `accounts_db` и `payments_db` из переменных `ACCOUNTS_DB_NAME` и `PAYMENTS_DB_NAME` при первой инициализации тома.

## Проверка

Юнит-тесты не требуют Postgres, Docker и запущенных сервисов.

```bash
go build ./...
go vet ./...
go test ./...
```

Один пакет или один тест:

```bash
go test ./internal/payment/service
go test ./internal/payment/service -run TestPaymentServiceIdempotency -v
```

Покрытие:

```bash
go test -cover ./...
```

Тесты покрывают сервисы, gRPC-обработчики, HTTP-шлюз, конфигурацию и разбор DSN. SQL-запросы, транзакции и блокировки в репозиториях не покрыты: для них нужен живой Postgres. Ручная проверка описана в разделе «Быстрый старт».

## Известные ограничения

- Перевод между сервисами не является распределённой транзакцией: `payment-service` пишет строку `PENDING`, вызывает `account-service`, затем обновляет статус. Если обновление статуса упадёт, деньги уже будут переведены.
- gRPC reflection включён в обоих сервисах для отладки.
- Пароль из `.env.example` (`postgresadmin`) подходит только для локальной разработки.
- CI отсутствует. Есть только юнит-тесты без Postgres; интеграционных тестов нет.

## Устранение неполадок

| Проблема | Решение |
|---|---|
| `make start` падает с `migrate: command not found` | установить `golang-migrate` или передать путь: `make start MIGRATE="$(which migrate)"` |
| `database ... does not exist` | проверить имена баз в `.env`, выполнить `make clean-db`, затем `make start` |
| `connection refused` на порту 50051 или 50052 | запустить сервисы: `make run` |
| Порт занят | изменить порт в `.env` |
