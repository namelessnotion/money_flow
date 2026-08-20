package eventstore_test

import (
	"context"
	"errors"
	"testing"
	"uuid"

	"google.golang.org/protobuf/proto"

	pb "github.com/namelessnotion/money_flow/go/gen/proto/holder/v1"
	"github.com/namelessnotion/money_flow/go/internal/eventstore"
)

func TestEventType(t *testing.T) {
	t.Parallel()

	got := eventstore.EventType(&pb.HolderEstablished{Id: "h1"})
	want := "holder.v1.HolderEstablished"
	if got != want {
		t.Fatalf("EventType() = %q, want %q", got, want)
	}
}

func TestEventDecode(t *testing.T) {
	t.Parallel()

	original := &pb.HolderEstablished{
		Id:    "h1",
		Memos: map[string]*pb.Memo{"why": {Id: "m1", Value: "onboarding"}},
	}
	payload, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	event := eventstore.Event{EventType: eventstore.EventType(original), Payload: payload}

	msg, err := event.Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	decoded, ok := msg.(*pb.HolderEstablished)
	if !ok {
		t.Fatalf("Decode() returned %T, want *pb.HolderEstablished", msg)
	}
	if !proto.Equal(original, decoded) {
		t.Errorf("round-trip mismatch:\n got: %v\nwant: %v", decoded, original)
	}
}

func TestEventDecodeUnknownType(t *testing.T) {
	t.Parallel()

	event := eventstore.Event{EventType: "holder.v1.NoSuchEvent", Payload: []byte{}}

	if _, err := event.Decode(); err == nil {
		t.Fatal("Decode() error = nil, want an error for an unregistered event type")
	}
}

// A payload that can't be unmarshalled must surface as an error rather than a
// zero-valued event: silently returning empty domain events would be worse than
// failing loudly.
func TestEventDecodeCorruptPayload(t *testing.T) {
	t.Parallel()

	event := eventstore.Event{
		EventType: eventstore.EventType(&pb.HolderEstablished{}),
		Payload:   []byte{0xff, 0xff, 0xff, 0xff},
	}

	if _, err := event.Decode(); err == nil {
		t.Fatal("Decode() error = nil, want an error for a corrupt payload")
	}
}

func TestMemoryStore(t *testing.T) {
	t.Parallel()

	runStoreContract(t, func(t *testing.T) eventstore.Store {
		t.Helper()
		return eventstore.NewMemoryStore()
	})
}

// runStoreContract exercises the behaviour every Store implementation must
// provide. It runs against MemoryStore always, and against PostgresStore when a
// database is reachable (see postgres_test.go), so the in-memory double can't
// silently drift from the real thing.
func runStoreContract(t *testing.T, newStore func(*testing.T) eventstore.Store) {
	t.Helper()

	ctx := context.Background()

	t.Run("append then load round-trips events in order", func(t *testing.T) {
		store := newStore(t)
		id := uniqueID(t)

		first := &pb.HolderEstablished{Id: id}
		second := &pb.HolderAddedWallet{Id: id, WalletId: "w1", Name: "primary"}

		if err := store.Append(ctx, "holder", id, 0, first, second); err != nil {
			t.Fatalf("Append() error = %v", err)
		}

		events, err := store.Load(ctx, "holder", id)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if len(events) != 2 {
			t.Fatalf("Load() returned %d events, want 2", len(events))
		}

		// Sequences are 1-based and contiguous within the stream.
		for i, event := range events {
			wantSeq := int64(i + 1)
			if event.Sequence != wantSeq {
				t.Errorf("events[%d].Sequence = %d, want %d", i, event.Sequence, wantSeq)
			}
			if event.AggregateType != "holder" || event.AggregateID != id {
				t.Errorf("events[%d] = (%q, %q), want (%q, %q)",
					i, event.AggregateType, event.AggregateID, "holder", id)
			}
			if event.OccurredAt.IsZero() {
				t.Errorf("events[%d].OccurredAt is zero", i)
			}
		}

		if got, want := events[0].EventType, eventstore.EventType(first); got != want {
			t.Errorf("events[0].EventType = %q, want %q", got, want)
		}

		decoded, err := events[1].Decode()
		if err != nil {
			t.Fatalf("events[1].Decode() error = %v", err)
		}
		if !proto.Equal(second, decoded) {
			t.Errorf("events[1] decoded to %v, want %v", decoded, second)
		}
	})

	t.Run("load returns nothing for an unknown aggregate", func(t *testing.T) {
		store := newStore(t)

		events, err := store.Load(ctx, "holder", uniqueID(t))
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if len(events) != 0 {
			t.Fatalf("Load() returned %d events, want 0", len(events))
		}
	})

	t.Run("appending no events is a no-op", func(t *testing.T) {
		store := newStore(t)
		id := uniqueID(t)

		if err := store.Append(ctx, "holder", id, 0); err != nil {
			t.Fatalf("Append() with no events error = %v", err)
		}

		events, err := store.Load(ctx, "holder", id)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if len(events) != 0 {
			t.Fatalf("Load() returned %d events, want 0", len(events))
		}
	})

	t.Run("a stale expectedSeq conflicts", func(t *testing.T) {
		store := newStore(t)
		id := uniqueID(t)

		if err := store.Append(ctx, "holder", id, 0, &pb.HolderEstablished{Id: id}); err != nil {
			t.Fatalf("first Append() error = %v", err)
		}

		// Second writer still believes the stream is empty.
		err := store.Append(ctx, "holder", id, 0, &pb.HolderEstablished{Id: id})
		if !errors.Is(err, eventstore.ErrConcurrencyConflict) {
			t.Fatalf("Append() with stale expectedSeq error = %v, want ErrConcurrencyConflict", err)
		}

		events, err := store.Load(ctx, "holder", id)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("Load() returned %d events, want 1 — the losing append must not be recorded", len(events))
		}
	})

	t.Run("appending continues from the current sequence", func(t *testing.T) {
		store := newStore(t)
		id := uniqueID(t)

		if err := store.Append(ctx, "holder", id, 0, &pb.HolderEstablished{Id: id}); err != nil {
			t.Fatalf("first Append() error = %v", err)
		}
		if err := store.Append(ctx, "holder", id, 1, &pb.HolderAddedWallet{Id: id, WalletId: "w1"}); err != nil {
			t.Fatalf("second Append() error = %v", err)
		}

		events, err := store.Load(ctx, "holder", id)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if len(events) != 2 {
			t.Fatalf("Load() returned %d events, want 2", len(events))
		}
		if events[1].Sequence != 2 {
			t.Errorf("events[1].Sequence = %d, want 2", events[1].Sequence)
		}
	})

	// A conflicting batch must land atomically or not at all — a half-applied
	// batch would leave the aggregate in a state no command ever produced.
	t.Run("a conflicting batch is applied atomically", func(t *testing.T) {
		store := newStore(t)
		id := uniqueID(t)

		if err := store.Append(ctx, "holder", id, 0, &pb.HolderEstablished{Id: id}); err != nil {
			t.Fatalf("first Append() error = %v", err)
		}

		// Sequence 1 collides; sequence 2 would be free on its own.
		err := store.Append(ctx, "holder", id, 0,
			&pb.HolderEstablished{Id: id},
			&pb.HolderAddedWallet{Id: id, WalletId: "w1"},
		)
		if !errors.Is(err, eventstore.ErrConcurrencyConflict) {
			t.Fatalf("Append() error = %v, want ErrConcurrencyConflict", err)
		}

		events, err := store.Load(ctx, "holder", id)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("Load() returned %d events, want 1 — no part of the conflicting batch may persist", len(events))
		}
	})

	t.Run("streams are isolated by aggregate id and type", func(t *testing.T) {
		store := newStore(t)
		idA, idB := uniqueID(t), uniqueID(t)

		if err := store.Append(ctx, "holder", idA, 0, &pb.HolderEstablished{Id: idA}); err != nil {
			t.Fatalf("Append(idA) error = %v", err)
		}
		if err := store.Append(ctx, "holder", idB, 0, &pb.HolderEstablished{Id: idB}); err != nil {
			t.Fatalf("Append(idB) error = %v", err)
		}
		// Same id under a different aggregate type is a different stream, and
		// so starts its own sequence at 1 rather than conflicting.
		if err := store.Append(ctx, "wallet", idA, 0, &pb.HolderEstablished{Id: idA}); err != nil {
			t.Fatalf("Append(wallet/idA) error = %v", err)
		}

		for _, tc := range []struct{ aggType, id string }{
			{"holder", idA}, {"holder", idB}, {"wallet", idA},
		} {
			events, err := store.Load(ctx, tc.aggType, tc.id)
			if err != nil {
				t.Fatalf("Load(%s/%s) error = %v", tc.aggType, tc.id, err)
			}
			if len(events) != 1 {
				t.Errorf("Load(%s/%s) returned %d events, want 1", tc.aggType, tc.id, len(events))
			}
			if len(events) > 0 && events[0].Sequence != 1 {
				t.Errorf("Load(%s/%s) first sequence = %d, want 1", tc.aggType, tc.id, events[0].Sequence)
			}
		}
	})
}

// uniqueID keeps every test case in its own stream. The events table is
// append-only (DELETE is blocked by a trigger), so the Postgres run of this
// suite cannot clean up after itself and relies on never colliding instead —
// including across repeated runs against the same database.
//
// UUIDv7 rather than v4: its leading millisecond timestamp makes the ids sort
// in creation order, which keeps the index on an append-only table healthy and
// lets leftover rows be dated.
func uniqueID(t *testing.T) string {
	t.Helper()
	return uuid.NewV7().String()
}
