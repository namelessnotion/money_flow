# Money Flow repository instructions

## Commands

Run commands from the component directory unless noted otherwise.

| Purpose | Command |
| --- | --- |
| Start local dependencies | `docker compose up -d` |
| Apply Go event-store migrations | `cd go && DATABASE_URL=postgres://money_flow:money_flow@localhost:5432/money_flow_dev?sslmode=disable go run ./cmd/migrate up` |
| Roll back the latest Go migration | `cd go && DATABASE_URL=postgres://money_flow:money_flow@localhost:5432/money_flow_dev?sslmode=disable go run ./cmd/migrate down` |
| Run the Go server | `cd go && DATABASE_URL=postgres://money_flow:money_flow@localhost:5432/money_flow_dev?sslmode=disable go run ./cmd/server` |
| Run all Go tests | `cd go && go test ./...` |
| Run one Go package or test | `cd go && go test ./internal/wallet -run '^TestOpenRecordsWalletOpened$'` |
| Apply Ruby migrations | `cd ruby && DATABASE_URL=postgres://money_flow:money_flow@localhost:5432/money_flow_dev?sslmode=disable bin/migrate up` |
| Run all Ruby specs | `cd ruby && bundle exec rspec` |
| Run one Ruby spec or example | `cd ruby && bundle exec rspec spec/app/services/onboard_entity_spec.rb:26` |
| Lint Ruby | `cd ruby && bundle exec rubocop` |
| Type-check Ruby | `cd ruby && bundle exec srb tc` |
| Regenerate Protobuf/Twirp bindings | `bin/generate_protos.sh` |
| Regenerate Sequel/Sorbet DSL RBIs | `cd ruby && DATABASE_URL=postgres://money_flow:money_flow@localhost:5432/money_flow_dev?sslmode=disable bundle exec tapioca dsl` |

`bin/generate_protos.sh` deletes and recreates both `go/gen/proto` and
`ruby/gen/proto`; edit only `proto/**/*.proto`, then regenerate and commit the
resulting bindings. It builds the required Go protoc plugins into the ignored
`go/.tools/` directory. `protoc` must be installed locally.

Ruby's eager app loading and `tapioca dsl` query the database when Sequel model
classes are defined, so run Ruby migrations first. The two migration systems
are separate: Go records versions in `schema_migrations`; Ruby uses
`ruby_schema_migrations`.

## Architecture

This is a financial-system monorepo with a Go command/API side and a Ruby
application side, coupled through versioned Protobuf contracts:

- `proto/` is the source of truth for domain messages and Twirp service
  interfaces. `holder.v1` and `wallet.v1` are currently implemented by Go;
  the other contracts define the wider domain vocabulary.
- The Go HTTP server (`go/cmd/server`) mounts generated Twirp handlers at their
  generated `/twirp/<package>.<Service>/` prefixes and exposes `/healthz`.
  It uses a Postgres-backed event store and currently wires the Holder and
  Wallet services.
- Go aggregates are event sourced. `internal/eventstore` is an append-only
  stream store; `events` is immutable at the database layer, and
  `UNIQUE(aggregate_type, aggregate_id, sequence)` enforces optimistic
  concurrency. Aggregate handlers rebuild state by decoding their event stream.
  TigerBeetle is the future/source-of-truth balance system, not the event log.
- `Holder` provisioning creates a holder plus its wallets using
  `AppendAtomic`, while standalone wallet opening writes one wallet stream.
  The Ruby `Services::OnboardEntity` calls the Holder Twirp client first, then
  persists its local `Entity` and one `Account` per account type in a Sequel
  transaction. Ruby GraphQL mutations call these services.
- The Ruby app is deliberately framework-light: `lib/boot.rb` owns the Sequel
  connection and generated-code load path, and `lib/environment.rb` eager-loads
  `app/types`, `models`, `services`, and `graphql`. RSpec wraps every example
  in a rollback-only database transaction.

## Conventions

- Preserve the event-sourcing invariants when adding Go commands: validate
  malformed inputs with Twirp argument errors; represent valid domain refusals
  in response `oneof` variants; append events rather than updating state; and
  treat `ErrConcurrencyConflict` as a reread/retry or idempotent convergence,
  not a lost update.
- A stream's first event establishes its aggregate (`HolderEstablished` or
  `WalletOpened`). Event payload types are fully qualified proto names, so any
  event message that must be decoded must have its generated Go package linked
  into the binary.
- A wallet policy must explicitly set `shared.v1.Allows`; use
  `ALLOWS_NONE` for neither direction, never the unspecified zero value.
  Shared money values use integer `minor_units`, never floating point.
- Keep Ruby account-type serialized strings explicit. They are simultaneously
  persisted in `accounts.type`, sent as wallet names, and constrained by the
  migration's fixed check values. If an account type changes, update the enum,
  migration strategy, and onboarding expectations together.
- `OnboardEntity` generates holder and wallet UUIDs once per call and reuses
  them for all retry attempts. Provisioning occurs before the local database
  transaction; do not hold a Ruby database transaction across the Twirp call.
- Ruby uses `# typed: strict` for app code and Sorbet signatures. Generated
  protobuf constants are runtime descriptor-pool assignments, so use
  `T.untyped`/`T.unsafe` only at the generated-code boundary as existing code
  does. Do not add `T::Generic` to Sequel model source classes; the custom
  Tapioca compiler supplies the static-only generic declarations.
- When a schema change affects a Sequel model, regenerate the committed
  `ruby/sorbet/rbi/dsl/models/` RBIs after migrating the database. Tapioca is
  configured with one worker because it loads a live Postgres connection.
