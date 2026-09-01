DB_URL  ?= postgres://postgres:password@localhost:5432/worm_dev?sslmode=disable
COMPOSE ?= docker compose

test-up:
	@until $(COMPOSE) exec -T postgres pg_isready -U postgres -d worm_dev >/dev/null 2>&1; do sleep 1; done
	psql "$(DB_URL)" -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
	psql "$(DB_URL)" -f ./querier/testdata/schema.sql


.PHONY: test-up
