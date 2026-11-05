MIGRATE=$(shell go env GOPATH)/bin/migrate

ACCOUNTS_DB_URL=postgres://postgres:postgresadmin@127.0.0.1:5432/accounts_db?sslmode=disable
PAYMENTS_DB_URL=postgres://postgres:postgresadmin@127.0.0.1:5432/payments_db?sslmode=disable
COMPOSE_FILE=deployments/docker-compose.yml

.PHONY: start down reset build clean clean-bin clean-db clean-all proto proto-clean migrate-up migrate-down

# 1. Запуск: поднимает Docker, ждет готовности, накатывает миграции и ПЕРЕСОБИРАЕТ бинарники
start:
	docker compose -f $(COMPOSE_FILE) up -d --wait
	@$(MAKE) migrate-up
	@$(MAKE) build
	@echo ">>> Инфраструктура готова, бинарники пересобраны в bin/!"

# 2. Остановка базы
down:
	docker compose -f $(COMPOSE_FILE) down

# 3. Перезапуск базы (данные сохраняются) и ПЕРЕСБОРКА бинарников
reset:
	docker compose -f $(COMPOSE_FILE) restart
	@echo ">>> Ожидание готовности PostgreSQL после перезапуска..."
	@until docker exec bank_postgres pg_isready -U postgres > /dev/null 2>&1; do sleep 1; done
	@$(MAKE) build
	@echo ">>> Контейнеры перезапущены (данные сохранены), бинарники пересобраны в bin/!"

# 4. Сборка всех бинарников в bin/
build:
	@mkdir -p bin
	go build -o bin/account-service ./cmd/account-service
	go build -o bin/payment-service ./cmd/payment-service
	go build -o bin/api-gateway ./cmd/api-gateway

# 5. Очистка бинарников (proto НЕ ТРОГАЕТ)
clean: clean-bin

clean-bin:
	rm -rf bin/

# 6. Очистка данных базы (удаляет том pgdata, образы НЕ трогает)
clean-db:
	docker compose -f $(COMPOSE_FILE) down -v

# 7. Полная очистка: бинарники + данные базы (proto и Docker-образы НЕ трогает)
clean-all: clean-bin clean-db
	@echo ">>> Бинарники и данные баз данных очищены."

# ==============================================================================
# МИГРАЦИИ
# ==============================================================================

migrate-up:
	$(MIGRATE) -path migrations/accounts_db -database "$(ACCOUNTS_DB_URL)" up
	$(MIGRATE) -path migrations/payments_db -database "$(PAYMENTS_DB_URL)" up

migrate-down:
	$(MIGRATE) -path migrations/payments_db -database "$(PAYMENTS_DB_URL)" down -all
	$(MIGRATE) -path migrations/accounts_db -database "$(ACCOUNTS_DB_URL)" down -all

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