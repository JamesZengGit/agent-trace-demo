.PHONY: up down build dev dev-stop bench chaos test tidy

# One command -> live dashboard on http://localhost:3000
# `down` first: containers half-created by an earlier failed start (e.g. a
# port conflict with the native dev stack) keep broken network config and
# must be recreated, not reused.
up:
	@./scripts/dev.sh stop >/dev/null 2>&1 || true
	docker compose down --remove-orphans 2>/dev/null || true
	docker compose up --build -d
	@echo "dashboard: http://localhost:3000   api: http://localhost:7000/healthz"

down:
	docker compose down -v

build:
	go build ./...
	cd dashboard && npm run build

test:
	go test ./...

# Native dev mode (no Docker): local Postgres + nats-server + all services.
dev:
	./scripts/dev.sh start

dev-stop:
	./scripts/dev.sh stop

# Measured throughput of the full capture path (loadgen -> proxy -> collector
# -> transport -> processor -> Postgres). Results + graph in docs/bench/.
bench:
	./scripts/bench.sh

# Reproduces the 87% orphan incident: kills the collector under load with
# reattachment off, then again with it on. Results + graph in docs/chaos/.
chaos:
	./scripts/chaos.sh
