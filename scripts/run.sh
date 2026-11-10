#!/usr/bin/env bash
# Запускает сервисы из bin/ по порядку и останавливает их вместе.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

if [[ -f ${ENV_FILE:-.env} ]]; then
	set -a
	# shellcheck disable=SC1090
	source "${ENV_FILE:-.env}"
	set +a
fi

pids=()
last_pid=

cleanup() {
	trap - EXIT
	if ((${#pids[@]} > 0)); then
		kill "${pids[@]}" 2>/dev/null || true
		wait 2>/dev/null || true
	fi
}
trap cleanup EXIT

start() {
	local binary=$1
	./bin/"$binary" &
	last_pid=$!
	pids+=("$last_pid")
}

wait_port() {
	local binary=$1 port=$2 attempt
	for ((attempt = 0; attempt < 100; attempt++)); do
		if (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null; then
			exec 3>&-
			echo ">>> $binary готов, порт $port"
			return 0
		fi
		if ! kill -0 "$last_pid" 2>/dev/null; then
			echo ">>> $binary завершился, не открыв порт $port" >&2
			return 1
		fi
		sleep 0.1
	done
	echo ">>> $binary не открыл порт $port за 10 с" >&2
	return 1
}

for key in ACCOUNT_SERVICE_GRPC_PORT PAYMENT_SERVICE_GRPC_PORT GATEWAY_HTTP_PORT; do
	if [[ -z "${!key:-}" ]]; then
		echo ">>> Не задана переменная $key (см. .env.example)" >&2
		exit 1
	fi
done

start account-service
wait_port account-service "$ACCOUNT_SERVICE_GRPC_PORT"

start payment-service
wait_port payment-service "$PAYMENT_SERVICE_GRPC_PORT"

start api-gateway
wait_port api-gateway "$GATEWAY_HTTP_PORT"

echo ">>> Все сервисы запущены. Ctrl+C останавливает их."

status=0
wait -n "${pids[@]}" || status=$?
if ((status == 130 || status == 143)); then
	status=0
fi
exit "$status"
