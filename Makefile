DB_URL  ?= postgres://postgres:password@localhost:5432/worm_dev?sslmode=disable
COMPOSE ?= docker compose

test-up:
	$(COMPOSE) up -d postgres
	@until $(COMPOSE) exec -T postgres pg_isready -U postgres -d worm_dev >/dev/null 2>&1; do sleep 1; done
	psql "$(DB_URL)" -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
	psql "$(DB_URL)" -f ./querier/testdata/schema.sql

# bigger schema for exercising migrate-data/migrate-resume end to end - separate
# from test-up, which the querier/inspect unit tests assert against exactly
dev-up:
	$(COMPOSE) up -d postgres
	@until $(COMPOSE) exec -T postgres pg_isready -U postgres -d worm_dev >/dev/null 2>&1; do sleep 1; done
	psql "$(DB_URL)" -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
	psql "$(DB_URL)" -f ./scripts/dev_schema.sql

# adds a batch of rows on top of whatever's already there - run dev-up first,
# then this, then again later (post migrate-data) to give migrate-resume
# something to stream
seed:
	@until $(COMPOSE) exec -T postgres pg_isready -U postgres -d worm_dev >/dev/null 2>&1; do sleep 1; done
	psql "$(DB_URL)" -f ./scripts/seed_data.sql

# compares row counts between SOURCE_CONN_STR and TARGET_CONN_STR (from .env),
# table by table - doesn't assume any particular schema
counts:
	go run ./scripts/checkcounts


.PHONY: test-up dev-up seed counts
