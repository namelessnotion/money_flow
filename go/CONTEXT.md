# Tokenized Transaction System

This is a tokenized transaction system.

Event Sourced, CQRS, and DDD are used to implement the system.

Event store is stored in a single Postgres table, as an append-only log. Other datastores maybe implemented in the future to hold the event store, but for now, Postgres is the only supported event store.

Domain commands and events are defined in the `proto/` directory using twirp RPC to implement the services.
