package eventstore

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
)

// MemoryStore is an in-process Store intended for tests: it lets a service be
// exercised without a database while still enforcing the same sequencing and
// optimistic-concurrency rules as PostgresStore. Nothing here is durable.
type MemoryStore struct {
	mu        sync.Mutex
	streams   map[string][]Event
	globalSeq int64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{streams: make(map[string][]Event)}
}

func (s *MemoryStore) Append(ctx context.Context, aggregateType, aggregateID string, expectedSeq int64, events ...proto.Message) error {
	if len(events) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := streamKey(aggregateType, aggregateID)
	stream := s.streams[key]
	// Strict control to prevent the expectedSeq from skipping ahead of the last sequence, thus preventing a gap in the sequence.
	// This is a stronger guarantee than PostgresStore, which only enforces that the expectedSeq is UNIQUE
	if int64(len(stream)) != expectedSeq {
		return ErrConcurrencyConflict
	}

	// Marshal everything before mutating, so a failure part-way through can't
	// leave a partially-appended stream behind.
	appended := make([]Event, 0, len(events))
	for i, evt := range events {
		payload, err := proto.Marshal(evt)
		if err != nil {
			return fmt.Errorf("eventstore: marshal %T: %w", evt, err)
		}
		appended = append(appended, Event{
			AggregateType: aggregateType,
			AggregateID:   aggregateID,
			Sequence:      expectedSeq + int64(i) + 1,
			EventType:     EventType(evt),
			Payload:       payload,
			OccurredAt:    time.Now().UTC(),
		})
	}

	for i := range appended {
		s.globalSeq++
		appended[i].GlobalSeq = s.globalSeq
	}
	s.streams[key] = append(stream, appended...)
	return nil
}

func (s *MemoryStore) Load(ctx context.Context, aggregateType, aggregateID string) ([]Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	stream := s.streams[streamKey(aggregateType, aggregateID)]
	if len(stream) == 0 {
		return nil, nil
	}
	return append([]Event(nil), stream...), nil
}

// streamKey uses a NUL separator so it can't collide with an aggregateType
// containing the separator character.
func streamKey(aggregateType, aggregateID string) string {
	return aggregateType + "\x00" + aggregateID
}
