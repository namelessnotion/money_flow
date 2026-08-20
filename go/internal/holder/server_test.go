package holder

import (
	"context"
	"errors"
	"testing"

	"github.com/twitchtv/twirp"
	"google.golang.org/protobuf/proto"

	pb "github.com/namelessnotion/money_flow/go/gen/proto/holder/v1"
	"github.com/namelessnotion/money_flow/go/internal/eventstore"
)

func TestEstablishRecordsHolderEstablished(t *testing.T) {
	t.Parallel()

	store := eventstore.NewMemoryStore()
	server := NewServer(store)
	ctx := context.Background()

	req := &pb.EstablishRequest{
		Id:    "h1",
		Memos: map[string]*pb.Memo{"why": {Id: "m1", Value: "onboarding"}},
	}

	resp, err := server.Establish(ctx, req)
	if err != nil {
		t.Fatalf("Establish() error = %v", err)
	}

	if resp.GetId() != "h1" {
		t.Errorf("response Id = %q, want %q", resp.GetId(), "h1")
	}
	established := resp.GetHolderEstablished()
	if established == nil {
		t.Fatal("response result = nil, want HolderEstablished")
	}
	if established.GetId() != "h1" {
		t.Errorf("HolderEstablished.Id = %q, want %q", established.GetId(), "h1")
	}
	// The memos on the command have to survive onto the event, or the reason a
	// Holder was established is lost the moment the request goes out of scope.
	if !proto.Equal(req.GetMemos()["why"], established.GetMemos()["why"]) {
		t.Errorf("HolderEstablished.Memos = %v, want %v", established.GetMemos(), req.GetMemos())
	}

	// The event must actually be in the log, not merely echoed back.
	events, err := store.Load(ctx, "holder", "h1")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("log holds %d events, want 1", len(events))
	}
	if got, want := events[0].EventType, "holder.v1.HolderEstablished"; got != want {
		t.Errorf("recorded event type = %q, want %q", got, want)
	}

	decoded, err := events[0].Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if !proto.Equal(established, decoded) {
		t.Errorf("recorded event = %v, want %v", decoded, established)
	}
}

func TestEstablishRequiresID(t *testing.T) {
	t.Parallel()

	store := eventstore.NewMemoryStore()
	server := NewServer(store)

	_, err := server.Establish(context.Background(), &pb.EstablishRequest{})
	if err == nil {
		t.Fatal("Establish() with empty id error = nil, want an error")
	}

	var twerr twirp.Error
	if !errors.As(err, &twerr) {
		t.Fatalf("Establish() error = %T, want twirp.Error", err)
	}
	if twerr.Code() != twirp.InvalidArgument {
		t.Errorf("error code = %q, want %q", twerr.Code(), twirp.InvalidArgument)
	}
}

// Callers own the Holder id and retry after ambiguous failures, so establishing
// twice has to converge on one Holder rather than erroring or double-recording.
func TestEstablishIsIdempotent(t *testing.T) {
	t.Parallel()

	store := eventstore.NewMemoryStore()
	server := NewServer(store)
	ctx := context.Background()

	req := &pb.EstablishRequest{Id: "h1", Memos: map[string]*pb.Memo{"why": {Id: "m1", Value: "first"}}}

	first, err := server.Establish(ctx, req)
	if err != nil {
		t.Fatalf("first Establish() error = %v", err)
	}

	second, err := server.Establish(ctx, req)
	if err != nil {
		t.Fatalf("second Establish() error = %v", err)
	}

	if !proto.Equal(first, second) {
		t.Errorf("second Establish() = %v, want identical to first %v", second, first)
	}

	events, err := store.Load(ctx, "holder", "h1")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("log holds %d events, want 1 — re-establishing must not append again", len(events))
	}
}

// A retry carrying different memos must not silently rewrite history; the
// original event is what gets returned.
func TestEstablishIgnoresChangedMemosOnReplay(t *testing.T) {
	t.Parallel()

	store := eventstore.NewMemoryStore()
	server := NewServer(store)
	ctx := context.Background()

	if _, err := server.Establish(ctx, &pb.EstablishRequest{
		Id:    "h1",
		Memos: map[string]*pb.Memo{"why": {Id: "m1", Value: "original"}},
	}); err != nil {
		t.Fatalf("first Establish() error = %v", err)
	}

	resp, err := server.Establish(ctx, &pb.EstablishRequest{
		Id:    "h1",
		Memos: map[string]*pb.Memo{"why": {Id: "m1", Value: "changed"}},
	})
	if err != nil {
		t.Fatalf("second Establish() error = %v", err)
	}

	if got := resp.GetHolderEstablished().GetMemos()["why"].GetValue(); got != "original" {
		t.Errorf("memo value = %q, want %q — the recorded event is immutable", got, "original")
	}
}

// The Holder id comes from the caller, so two concurrent onboarding attempts
// can race. The loser must be handed the winner's Holder, not an error.
func TestEstablishResolvesConcurrencyConflict(t *testing.T) {
	t.Parallel()

	store := eventstore.NewMemoryStore()
	ctx := context.Background()

	// Simulate losing the race: the event is already in the log by the time
	// this server's Append runs.
	winner := &pb.HolderEstablished{Id: "h1", Memos: map[string]*pb.Memo{"why": {Id: "m1", Value: "winner"}}}
	server := NewServer(&conflictOnceStore{Store: store, onConflict: func() {
		if err := store.Append(ctx, "holder", "h1", 0, winner); err != nil {
			t.Errorf("seeding winner: %v", err)
		}
	}})

	resp, err := server.Establish(ctx, &pb.EstablishRequest{
		Id:    "h1",
		Memos: map[string]*pb.Memo{"why": {Id: "m2", Value: "loser"}},
	})
	if err != nil {
		t.Fatalf("Establish() error = %v, want the winner's holder", err)
	}
	if got := resp.GetHolderEstablished().GetMemos()["why"].GetValue(); got != "winner" {
		t.Errorf("memo value = %q, want %q", got, "winner")
	}
}

// A Holder stream must open with HolderEstablished. If it doesn't, the log is
// corrupt or mis-sequenced and Establish must refuse rather than quietly treat
// the aggregate as new and append a second origin event.
func TestEstablishRejectsStreamNotStartingWithHolderEstablished(t *testing.T) {
	t.Parallel()

	store := eventstore.NewMemoryStore()
	ctx := context.Background()

	if err := store.Append(ctx, "holder", "h1", 0, &pb.HolderAddedWallet{Id: "h1", WalletId: "w1"}); err != nil {
		t.Fatalf("seeding stream: %v", err)
	}

	server := NewServer(store)
	_, err := server.Establish(ctx, &pb.EstablishRequest{Id: "h1"})
	if err == nil {
		t.Fatal("Establish() error = nil, want a refusal for a malformed stream")
	}

	var twerr twirp.Error
	if !errors.As(err, &twerr) {
		t.Fatalf("Establish() error = %T, want twirp.Error", err)
	}
	if twerr.Code() != twirp.Internal {
		t.Errorf("error code = %q, want %q", twerr.Code(), twirp.Internal)
	}

	events, err := store.Load(ctx, "holder", "h1")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(events) != 1 {
		t.Errorf("log holds %d events, want 1 — nothing may be appended to a malformed stream", len(events))
	}
}

func TestEstablishSurfacesStoreFailure(t *testing.T) {
	t.Parallel()

	boom := errors.New("database is on fire")
	server := NewServer(&failingStore{err: boom})

	_, err := server.Establish(context.Background(), &pb.EstablishRequest{Id: "h1"})
	if err == nil {
		t.Fatal("Establish() error = nil, want the store failure surfaced")
	}

	var twerr twirp.Error
	if !errors.As(err, &twerr) {
		t.Fatalf("Establish() error = %T, want twirp.Error", err)
	}
	if twerr.Code() != twirp.Internal {
		t.Errorf("error code = %q, want %q", twerr.Code(), twirp.Internal)
	}
}

func TestAddWalletIsUnimplemented(t *testing.T) {
	t.Parallel()

	server := NewServer(eventstore.NewMemoryStore())

	_, err := server.AddWallet(context.Background(), &pb.AddWalletRequest{Id: "h1", WalletId: "w1"})

	var twerr twirp.Error
	if !errors.As(err, &twerr) {
		t.Fatalf("AddWallet() error = %T, want twirp.Error", err)
	}
	if twerr.Code() != twirp.Unimplemented {
		t.Errorf("error code = %q, want %q", twerr.Code(), twirp.Unimplemented)
	}
}

// conflictOnceStore forces the first Append to lose an optimistic-concurrency
// race, running onConflict first so the winning event is already in the log.
type conflictOnceStore struct {
	eventstore.Store
	onConflict func()
	conflicted bool
}

func (s *conflictOnceStore) Append(ctx context.Context, aggregateType, aggregateID string, expectedSeq int64, events ...proto.Message) error {
	if !s.conflicted {
		s.conflicted = true
		s.onConflict()
		return eventstore.ErrConcurrencyConflict
	}
	return s.Store.Append(ctx, aggregateType, aggregateID, expectedSeq, events...)
}

type failingStore struct{ err error }

func (s *failingStore) Append(context.Context, string, string, int64, ...proto.Message) error {
	return s.err
}

func (s *failingStore) Load(context.Context, string, string) ([]eventstore.Event, error) {
	return nil, s.err
}
