.PHONY: up down restart migrate ssl

up:
	docker compose up -d

down:
	docker compose down

restart:
	docker compose restart

# Runs both migrators against the postgres container out-of-band, e.g. after
# adding a new migration file without wanting to restart ruby/go (which also
# migrate automatically on boot, see docker/ruby/entrypoint.sh and
# docker/go/entrypoint.sh).
migrate:
	docker compose exec go go run ./cmd/migrate up
	docker compose exec ruby bin/migrate up

# Generates a browser-trusted cert for *.local.namelessnotion.com (see
# bin/setup_local_ssl.sh) and restarts the proxy container to pick it up.
ssl:
	bin/setup_local_ssl.sh
	-docker compose restart proxy
