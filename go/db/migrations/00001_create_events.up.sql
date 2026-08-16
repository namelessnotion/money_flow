CREATE TABLE events (
    global_seq      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    aggregate_type  TEXT NOT NULL,
    aggregate_id    TEXT NOT NULL,
    sequence        BIGINT NOT NULL,
    event_type      TEXT NOT NULL,
    payload         BYTEA NOT NULL,
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (aggregate_type, aggregate_id, sequence)
);

-- Every projection/process manager reads forward from a bookmark on this
-- index, so new events across all aggregates are visible in a single
-- total order without touching per-aggregate rows.
CREATE INDEX events_global_seq_idx ON events (global_seq);

-- Aggregates replay their own stream in sequence order.
CREATE INDEX events_aggregate_idx ON events (aggregate_type, aggregate_id, sequence);

-- The event log is append-only and immutable by design: enforce that at
-- the database layer so it holds regardless of which role connects or
-- what future code does, not just application discipline.
CREATE FUNCTION forbid_event_mutation() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'events is append-only: % is not permitted', TG_OP;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER events_no_update
  BEFORE UPDATE ON events
  FOR EACH ROW EXECUTE FUNCTION forbid_event_mutation();

CREATE TRIGGER events_no_delete
  BEFORE DELETE ON events
  FOR EACH ROW EXECUTE FUNCTION forbid_event_mutation();
