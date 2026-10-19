#!/usr/bin/env bash
set -euo pipefail

requests=${REQUESTS:-200000}
clients=${CLIENTS:-50}
work=$(mktemp -d)
trap 'kill $(jobs -p) 2>/dev/null; rm -rf "$work"' EXIT

go build -o "$work/clio" ./src

redis-server --port 6379 --dir "$work" --appendonly yes --appendfsync everysec \
	--save "" --daemonize no >"$work/redis.log" 2>&1 &
"$work/clio" -addr localhost:6380 -aof "$work/clio.aof" >"$work/clio.log" 2>&1 &

for port in 6379 6380; do
	until redis-cli -p "$port" PING >/dev/null 2>&1; do sleep 0.1; done
done

printf "%-6s %-9s %-5s %12s %9s\n" server pipeline test "requests/s" "p50 ms"
for pipeline in 1 16; do
	for port in 6379 6380; do
		name=$([ "$port" = 6379 ] && echo redis || echo clio)
		redis-benchmark -p "$port" -t set,get -n "$requests" -c "$clients" -P "$pipeline" --csv 2>/dev/null |
			tail -n +2 | tr -d '"' |
			while IFS=, read -r test rps avg min p50 rest; do
				printf "%-6s %-9s %-5s %12.0f %9s\n" "$name" "$pipeline" "$test" "$rps" "$p50"
			done
	done
done
