<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/logo-dark.svg">
  <img alt="MoneyFlow" src="assets/logo-light.svg" height="72">
</picture>

Event-sourced money movement system: a Go event store and ledger backend, a
Ruby/GraphQL business layer on top of it, and a Vue client.

## Status

Early scaffolding. End-to-end today: onboarding an `Entity` in Ruby
provisions a `Holder` and `Wallet` in Go over Twirp, both are recorded as
immutable events, and the Vue client lists entities via GraphQL.

## Structure

```
proto/    Protobuf domain messages and Twirp service definitions, shared by go/ and ruby/
go/       Event-sourced ledger backend (Go + Twirp), backed by TigerBeetle for token accounting
ruby/     Business backend (Ruby + GraphQL), orchestrates money flow via the go/ backend
client/   Frontend (Vue 3 + Apollo Client 4 + TailwindCSS), talks to ruby/ over GraphQL
docker/   Dockerfiles and entrypoints for each service, wired together by docker-compose.yml
```

### `proto/`

Domain command/event messages and Twirp service definitions
(`holder`, `wallet`, `token`, `transaction`, `transfer`, `operation`,
`shared`), compiled to both Go and Ruby with `bin/generate_protos.sh`.

### `go/`

Event-sourced backend. Commands are validated and recorded as domain events
in an append-only event log (`internal/eventstore`, PostgreSQL-backed, one
table); the event log is the source of truth for intent. TigerBeetle handles
low-level token accounting. Exposed as Twirp RPC services (`internal/holder`,
`internal/wallet`) reachable directly on `:8080` or via the proxy at
`https://rpc.local.namelessnotion.com`.

### `ruby/`

Business backend. Exposes GraphQL (`app/graphql`) backed by Sequel models
(`app/models`) over PostgreSQL, and orchestrates money-flow operations
(`app/services`) by calling the `go/` backend over Twirp. Runs on Falcon,
reachable directly on `:9292` or via the proxy at
`https://graphql.local.namelessnotion.com`.

### `client/`

Vue 3 + TypeScript SPA. Apollo Client (via `@vue/apollo-composable`) queries
the Ruby GraphQL API; TailwindCSS for styling. Served by Vite, reachable
directly on `:5173` or via the proxy at `https://app.local.namelessnotion.com`.

## Running locally

Everything runs via Docker Compose: PostgreSQL, TigerBeetle, the Go and Ruby
backends, the Vite dev server, and an nginx reverse proxy that fronts all
three under `*.local.namelessnotion.com`.

```bash
make up        # docker compose up -d
```

This starts:

| Service | Direct port | Proxied at |
|---|---|---|
| `client` (Vite) | `:5173` | `https://app.local.namelessnotion.com` |
| `ruby` (GraphQL) | `:9292` | `https://graphql.local.namelessnotion.com` |
| `go` (Twirp/RPC) | `:8080` | `https://rpc.local.namelessnotion.com` |

The Go and Ruby services each run pending migrations on boot. To add the
proxy subdomains, point them at `127.0.0.1` in `/etc/hosts`, then generate a
browser-trusted local cert:

```bash
make ssl       # bin/setup_local_ssl.sh, requires mkcert; then restarts the proxy
```

Until `make ssl` has been run, the proxy serves a self-signed placeholder, so
HTTPS still works, just with a browser warning.

Other Makefile targets:

```bash
make down      # docker compose down
make restart   # docker compose restart
make migrate   # run both migrators out-of-band, without restarting go/ruby
```

### Running services outside Docker

```bash
# Go backend
cd go && go run ./cmd/server

# Ruby backend (expects the Go backend and Postgres reachable)
cd ruby && bin/server

# Client (expects the Ruby backend reachable, see client/README.md)
cd client && npm install && npm run dev
```

### Regenerating protobuf code

After changing anything under `proto/`:

```bash
bin/generate_protos.sh
```

## Testing & linting

Tests are written first, then code to pass them. Run these before pushing:

```bash
# Go
cd go && go test ./...
cd go && golangci-lint run

# Ruby
cd ruby && bundle exec rspec
cd ruby && bundle exec rubocop
cd ruby && bundle exec srb tc       # Sorbet type check

# Client
cd client && npm run build          # vue-tsc type check + Vite build
```

CI (`.github/workflows/`) runs Go tests, Ruby specs, Rubocop, and Sorbet's
`srb tc` against every push/PR to `main`.

## Conventions

See [CLAUDE.md](CLAUDE.md) for coding conventions: test-first development,
no linter-disable comments, avoiding `T.untyped` in Sorbet, and the
`# typed: strict` default for new Ruby files.
