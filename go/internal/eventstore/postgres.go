package eventstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
)

const uniqueViolation = "23505"

// PostgresStore is a Store backed by the events table (see
// go/db/migrations/00001_create_events.up.sql). Optimistic concurrency is
// enforced entirely by that table's UNIQUE(aggregate_type, aggregate_id,
// sequence) constraint: Append computes each event's sequence number from
// expectedSeq up front, so a stale expectedSeq collides with rows another
// writer already inserted instead of racing a separate check.
type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) Append(ctx context.Context, aggregateType, aggregateID string, expectedSeq int64, events ...proto.Message) error {
	if len(events) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for i, evt := range events {
		payload, err := proto.Marshal(evt)
		if err != nil {
			return fmt.Errorf("eventstore: marshal %T: %w", evt, err)
		}
		batch.Queue(
			`INSERT INTO events (aggregate_type, aggregate_id, sequence, event_type, payload)
			 VALUES ($1, $2, $3, $4, $5)`,
			aggregateType, aggregateID, expectedSeq+int64(i)+1, EventType(evt), payload,
		)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("eventstore: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	br := tx.SendBatch(ctx, batch)
	for range events {
		if _, err := br.Exec(); err != nil {
			br.Close()
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
				return ErrConcurrencyConflict
			}
			return fmt.Errorf("eventstore: insert: %w", err)
		}
	}
	if err := br.Close(); err != nil {
		return fmt.Errorf("eventstore: close batch: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("eventstore: commit: %w", err)
	}
	return nil
}

func (s *PostgresStore) Load(ctx context.Context, aggregateType, aggregateID string) ([]Event, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT global_seq, aggregate_type, aggregate_id, sequence, event_type, payload, occurred_at
		 FROM events
		 WHERE aggregate_type = $1 AND aggregate_id = $2
		 ORDER BY sequence ASC`,
		aggregateType, aggregateID,
	)
	if err != nil {
		return nil, fmt.Errorf("eventstore: query: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.GlobalSeq, &e.AggregateType, &e.AggregateID, &e.Sequence, &e.EventType, &e.Payload, &e.OccurredAt); err != nil {
			return nil, fmt.Errorf("eventstore: scan: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
