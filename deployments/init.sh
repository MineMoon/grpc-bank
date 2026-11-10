#!/bin/sh
# Создаёт обе базы при первой инициализации тома pgdata.
set -eu

psql --username "$POSTGRES_USER" --dbname postgres --set ON_ERROR_STOP=1 \
	-c "CREATE DATABASE \"$ACCOUNTS_DB_NAME\"" \
	-c "CREATE DATABASE \"$PAYMENTS_DB_NAME\""
