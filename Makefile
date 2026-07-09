.PHONY: db-up db-down migrate run test build

VSRV_DATABASE_URL ?= postgres://vsrv:vsrv@localhost:5433/vsrv?sslmode=disable

db-up:
	docker compose up -d postgres

db-down:
	docker compose down

migrate:
	@test -n "$(VSRV_DATABASE_URL)"
	for file in migrations/*.sql; do \
		echo "applying $$file"; \
		psql "$(VSRV_DATABASE_URL)" -v ON_ERROR_STOP=1 -f "$$file"; \
	done

migrate-docker: db-up
	@until docker compose exec -T postgres pg_isready -U vsrv -d vsrv >/dev/null 2>&1; do sleep 1; done
	for file in migrations/*.sql; do \
		echo "applying $$file"; \
		docker compose exec -T postgres psql -U vsrv -d vsrv -v ON_ERROR_STOP=1 < "$$file"; \
	done

run:
	@set -a; [ -f .env ] && . ./.env; set +a; go run ./cmd/vsrv

build:
	go build -o bin/vsrv ./cmd/vsrv

test:
	go test ./...
