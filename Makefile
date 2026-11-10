# Конфигурация лежит в .env: при первом запуске он создаётся из .env.example.
ifeq (,$(wildcard .env))
$(shell cp .env.example .env)
$(info >>> .env не найден, создан из .env.example)
endif
-include .env

MIGRATE ?= $(shell go env GOPATH)/bin/migrate
COMPOSE_FILE ?= deployments/docker-compose.yml

# Пароль, хост и порт берутся из одного места, полный DSN можно задать явно.
ACCOUNT_SERVICE_DB_URL ?= postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@$(POSTGRES_HOST):$(POSTGRES_PORT)/$(ACCOUNTS_DB_NAME)?sslmode=$(POSTGRES_SSLMODE)
PAYMENT_SERVICE_DB_URL ?= postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@$(POSTGRES_HOST):$(POSTGRES_PORT)/$(PAYMENTS_DB_NAME)?sslmode=$(POSTGRES_SSLMODE)

export

.PHONY: start run infra down reset build clean clean-bin clean-db clean-all proto proto-clean migrate-up migrate-down

# 1. Запуск: поднимает Docker, накатывает миграции, собирает бинарники и запускает все сервисы.
#    Ctrl+C останавливает сервисы, контейнер Postgres продолжает работать (make down).
start: infra migrate-up build
	@./scripts/run.sh

# 2. Запуск уже собранных сервисов без миграций и пересборки
run:
	@./scripts/run.sh

# 3. Инфраструктура: Postgres и ожидание готовности
infra:
	docker compose --env-file .env -f $(COMPOSE_FILE) up -d --wait

# 4. Остановка базы
down:
	docker compose --env-file .env -f $(COMPOSE_FILE) down

# 5. Перезапуск базы (данные сохраняются) и ПЕРЕСБОРКА бинарников
reset:
	docker compose --env-file .env -f $(COMPOSE_FILE) restart
	@echo ">>> Ожидание готовности PostgreSQL после перезапуска..."
	@until docker exec bank_postgres pg_isready -U "$$POSTGRES_USER" > /dev/null 2>&1; do sleep 1; done
	@$(MAKE) build
	@echo ">>> Контейнеры перезапущены (данные сохранены), бинарники пересобраны в bin/!"

# 6. Сборка всех бинарников в bin/
build:
	@mkdir -p bin
	go build -o bin/account-service ./cmd/account-service
	go build -o bin/payment-service ./cmd/payment-service
	go build -o bin/api-gateway ./cmd/api-gateway

# 7. Очистка бинарников (proto НЕ ТРОГАЕТ)
clean: clean-bin

clean-bin:
	rm -rf bin/

# 8. Очистка данных базы (удаляет том pgdata, образы НЕ трогает)
clean-db:
	docker compose --env-file .env -f $(COMPOSE_FILE) down -v

# 9. Полная очистка: бинарники + данные базы (proto и Docker-образы НЕ трогает)
clean-all: clean-bin clean-db
	@echo ">>> Бинарники и данные баз данных очищены."

# ==============================================================================
# МИГРАЦИИ
# ==============================================================================

migrate-up:
	$(MIGRATE) -path migrations/accounts_db -database "$(ACCOUNT_SERVICE_DB_URL)" up
	$(MIGRATE) -path migrations/payments_db -database "$(PAYMENT_SERVICE_DB_URL)" up

migrate-down:
	$(MIGRATE) -path migrations/payments_db -database "$(PAYMENT_SERVICE_DB_URL)" down -all
	$(MIGRATE) -path migrations/accounts_db -database "$(ACCOUNT_SERVICE_DB_URL)" down -all

# ==============================================================================
# PROTOBUF
# ==============================================================================

proto:
	protoc --proto_path=api/proto \
	       --go_out=. --go_opt=module=grpc-bank \
	       --go-grpc_out=. --go-grpc_opt=module=grpc-bank \
	       api/proto/account/account.proto \
	       api/proto/payment/payment.proto

proto-clean:
	rm -rf pkg/api/*
