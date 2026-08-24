package holder

import (
	"context"
	"errors"
	"testing"

	"github.com/twitchtv/twirp"
	"google.golang.org/protobuf/proto"

	pb "github.com/namelessnotion/money_flow/go/gen/proto/holder/v1"
	sharedpb "github.com/namelessnotion/money_flow/go/gen/proto/shared/v1"
	walletpb "github.com/namelessnotion/money_flow/go/gen/proto/wallet/v1"
	"github.com/namelessnotion/money_flow/go/internal/eventstore"
	"github.com/namelessnotion/money_flow/go/internal/wallet"
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

// provisionRequest builds a Holder plus three Wallets, the shape onboarding
// sends (one Wallet per account type).
func provisionRequest() *pb.ProvisionRequest {
	return &pb.ProvisionRequest{
		Id: "h1",
		Wallets: []*pb.WalletSpec{
			{WalletId: "w-bank", Name: "bank", Allows: sharedpb.Allows_ALLOWS_ONRAMP_AND_OFFRAMP},
			{WalletId: "w-debit", Name: "debit_card", Allows: sharedpb.Allows_ALLOWS_ONRAMP},
			{WalletId: "w-cash", Name: "cash", Allows: sharedpb.Allows_ALLOWS_NONE},
		},
	}
}

func TestProvisionWritesHolderAndEveryWallet(t *testing.T) {
	t.Parallel()

	store := eventstore.NewMemoryStore()
	ctx := context.Background()

	resp, err := NewServer(store).Provision(ctx, provisionRequest())
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}

	provisioned := resp.GetHolderProvisioned()
	if provisioned == nil {
		t.Fatal("response result = nil, want HolderProvisioned")
	}
	if len(provisioned.GetWalletIds()) != 3 {
		t.Errorf("HolderProvisioned.WalletIds = %v, want 3 ids", provisioned.GetWalletIds())
	}

	// The Holder's stream: HolderEstablished then one HolderAddedWallet each.
	holderEvents, err := store.Load(ctx, aggregateType, "h1")
	if err != nil {
		t.Fatalf("Load(holder) error = %v", err)
	}
	if len(holderEvents) != 4 {
		t.Fatalf("holder stream holds %d events, want 4", len(holderEvents))
	}
	if got, want := holderEvents[0].EventType, "holder.v1.HolderEstablished"; got != want {
		t.Errorf("holder stream starts with %q, want %q", got, want)
	}
	for i, event := range holderEvents[1:] {
		if got, want := event.EventType, "holder.v1.HolderAddedWallet"; got != want {
			t.Errorf("holder stream event %d = %q, want %q", i+1, got, want)
		}
	}

	// Each Wallet gets its own stream with exactly one WalletOpened.
	for _, spec := range provisionRequest().GetWallets() {
		events, err := store.Load(ctx, wallet.AggregateType, spec.GetWalletId())
		if err != nil {
			t.Fatalf("Load(wallet %s) error = %v", spec.GetWalletId(), err)
		}
		if len(events) != 1 {
			t.Fatalf("wallet %s holds %d events, want 1", spec.GetWalletId(), len(events))
		}
		msg, err := events[0].Decode()
		if err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		opened, ok := msg.(*walletpb.WalletOpened)
		if !ok {
			t.Fatalf("wallet %s recorded %T, want *WalletOpened", spec.GetWalletId(), msg)
		}
		if opened.GetHolderId() != "h1" || opened.GetName() != spec.GetName() {
			t.Errorf("WalletOpened = (holder %q, name %q), want (h1, %q)",
				opened.GetHolderId(), opened.GetName(), spec.GetName())
		}
		if opened.GetAllows() != spec.GetAllows() {
			t.Errorf("wallet %s allows = %v, want %v", spec.GetWalletId(), opened.GetAllows(), spec.GetAllows())
		}
	}
}

// Provisioning nothing partially is the whole point: if any stream in the batch
// conflicts, none of the others may be written either.
func TestProvisionIsAllOrNothing(t *testing.T) {
	t.Parallel()

	store := eventstore.NewMemoryStore()
	ctx := context.Background()

	// A store that fails the atomic append the way a real conflict would,
	// without writing anything.
	server := NewServer(&failingStore{err: errors.New("append refused")})
	if _, err := server.Provision(ctx, provisionRequest()); err == nil {
		t.Fatal("Provision() error = nil, want the store failure surfaced")
	}

	// Nothing may have been written to any stream.
	for _, tc := range []struct{ aggType, id string }{
		{aggregateType, "h1"}, {wallet.AggregateType, "w-bank"},
		{wallet.AggregateType, "w-debit"}, {wallet.AggregateType, "w-cash"},
	} {
		events, err := store.Load(ctx, tc.aggType, tc.id)
		if err != nil {
			t.Fatalf("Load(%s/%s) error = %v", tc.aggType, tc.id, err)
		}
		if len(events) != 0 {
			t.Errorf("Load(%s/%s) holds %d events, want 0", tc.aggType, tc.id, len(events))
		}
	}
}

func TestProvisionIsIdempotent(t *testing.T) {
	t.Parallel()

	store := eventstore.NewMemoryStore()
	server := NewServer(store)
	ctx := context.Background()

	first, err := server.Provision(ctx, provisionRequest())
	if err != nil {
		t.Fatalf("first Provision() error = %v", err)
	}
	second, err := server.Provision(ctx, provisionRequest())
	if err != nil {
		t.Fatalf("second Provision() error = %v", err)
	}
	if !proto.Equal(first, second) {
		t.Errorf("second Provision() = %v, want identical to first %v", second, first)
	}

	holderEvents, _ := store.Load(ctx, aggregateType, "h1")
	if len(holderEvents) != 4 {
		t.Errorf("holder stream holds %d events, want 4 — re-provisioning must append nothing", len(holderEvents))
	}
	for _, spec := range provisionRequest().GetWallets() {
		events, _ := store.Load(ctx, wallet.AggregateType, spec.GetWalletId())
		if len(events) != 1 {
			t.Errorf("wallet %s holds %d events, want 1", spec.GetWalletId(), len(events))
		}
	}
}

// A retry after a partial failure — or provisioning on top of a Holder that was
// established directly — must fill in only what is missing.
func TestProvisionConvergesOnPartialState(t *testing.T) {
	t.Parallel()

	store := eventstore.NewMemoryStore()
	server := NewServer(store)
	ctx := context.Background()

	// The Holder and one of its Wallets already exist.
	if _, err := server.Establish(ctx, &pb.EstablishRequest{Id: "h1"}); err != nil {
		t.Fatalf("Establish() error = %v", err)
	}
	if err := store.Append(ctx, wallet.AggregateType, "w-bank", 0,
		wallet.OpenedEvent("w-bank", "h1", "bank", sharedpb.Allows_ALLOWS_ONRAMP_AND_OFFRAMP),
	); err != nil {
		t.Fatalf("seeding wallet: %v", err)
	}

	if _, err := server.Provision(ctx, provisionRequest()); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}

	holderEvents, _ := store.Load(ctx, aggregateType, "h1")
	if len(holderEvents) != 4 {
		t.Errorf("holder stream holds %d events, want 4", len(holderEvents))
	}
	// The pre-existing wallet keeps its single event; the missing two are opened.
	for _, spec := range provisionRequest().GetWallets() {
		events, _ := store.Load(ctx, wallet.AggregateType, spec.GetWalletId())
		if len(events) != 1 {
			t.Errorf("wallet %s holds %d events, want 1", spec.GetWalletId(), len(events))
		}
	}
}

func TestProvisionValidatesRequest(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		req  *pb.ProvisionRequest
	}{
		{"missing id", &pb.ProvisionRequest{Wallets: provisionRequest().GetWallets()}},
		{"no wallets", &pb.ProvisionRequest{Id: "h1"}},
		{"wallet without allows", &pb.ProvisionRequest{Id: "h1", Wallets: []*pb.WalletSpec{{WalletId: "w1"}}}},
		{"duplicate wallet id", &pb.ProvisionRequest{Id: "h1", Wallets: []*pb.WalletSpec{
			{WalletId: "w1", Allows: sharedpb.Allows_ALLOWS_NONE},
			{WalletId: "w1", Allows: sharedpb.Allows_ALLOWS_NONE},
		}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := NewServer(eventstore.NewMemoryStore())

			_, err := server.Provision(context.Background(), tc.req)
			if err == nil {
				t.Fatal("Provision() error = nil, want a rejection")
			}
			var twerr twirp.Error
			if !errors.As(err, &twerr) || twerr.Code() != twirp.InvalidArgument {
				t.Errorf("Provision() error = %v, want twirp invalid_argument", err)
			}
		})
	}
}

// A wallet id already owned by someone else must not be silently absorbed.
func TestProvisionRejectsWalletOwnedByAnotherHolder(t *testing.T) {
	t.Parallel()

	store := eventstore.NewMemoryStore()
	ctx := context.Background()

	if err := store.Append(ctx, wallet.AggregateType, "w-bank", 0,
		wallet.OpenedEvent("w-bank", "someone-else", "bank", sharedpb.Allows_ALLOWS_ONRAMP),
	); err != nil {
		t.Fatalf("seeding wallet: %v", err)
	}

	resp, err := NewServer(store).Provision(ctx, provisionRequest())
	if err != nil {
		t.Fatalf("Provision() error = %v, want a domain rejection", err)
	}
	if resp.GetHolderProvisionRejected() == nil {
		t.Fatal("response result = nil, want HolderProvisionRejected")
	}

	// The rejected request must not have written anything.
	holderEvents, _ := store.Load(ctx, aggregateType, "h1")
	if len(holderEvents) != 0 {
		t.Errorf("holder stream holds %d events, want 0", len(holderEvents))
	}
}

func TestAddWalletOpensAndRecordsTheWallet(t *testing.T) {
	t.Parallel()

	store := eventstore.NewMemoryStore()
	server := NewServer(store)
	ctx := context.Background()

	if _, err := server.Establish(ctx, &pb.EstablishRequest{Id: "h1"}); err != nil {
		t.Fatalf("Establish() error = %v", err)
	}

	resp, err := server.AddWallet(ctx, &pb.AddWalletRequest{
		Id: "h1", WalletId: "w1", Name: "bank", Allows: sharedpb.Allows_ALLOWS_ONRAMP,
	})
	if err != nil {
		t.Fatalf("AddWallet() error = %v", err)
	}
	if resp.GetHolderAddedWallet() == nil {
		t.Fatal("response result = nil, want HolderAddedWallet")
	}

	holderEvents, _ := store.Load(ctx, aggregateType, "h1")
	if len(holderEvents) != 2 {
		t.Errorf("holder stream holds %d events, want 2", len(holderEvents))
	}
	walletEvents, _ := store.Load(ctx, wallet.AggregateType, "w1")
	if len(walletEvents) != 1 {
		t.Errorf("wallet stream holds %d events, want 1", len(walletEvents))
	}
}

func TestAddWalletRequiresAnEstablishedHolder(t *testing.T) {
	t.Parallel()

	server := NewServer(eventstore.NewMemoryStore())

	_, err := server.AddWallet(context.Background(), &pb.AddWalletRequest{
		Id: "nope", WalletId: "w1", Allows: sharedpb.Allows_ALLOWS_NONE,
	})
	if err == nil {
		t.Fatal("AddWallet() error = nil, want a rejection for an unknown holder")
	}
	var twerr twirp.Error
	if !errors.As(err, &twerr) || twerr.Code() != twirp.FailedPrecondition {
		t.Errorf("AddWallet() error = %v, want twirp failed_precondition", err)
	}
}

// conflictOnceStore forces the first append to lose an optimistic-concurrency
// race, running onConflict first so the winning event is already in the log.
// Both append methods are overridden: Establish goes through Append, and
// anything provisioning several streams goes through AppendAtomic, so
// overriding only one would silently stop exercising the other.
type conflictOnceStore struct {
	eventstore.Store
	onConflict func()
	conflicted bool
}

func (s *conflictOnceStore) Append(ctx context.Context, aggregateType, aggregateID string, expectedSeq int64, events ...proto.Message) error {
	return s.AppendAtomic(ctx, eventstore.StreamWrite{
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		ExpectedSeq:   expectedSeq,
		Events:        events,
	})
}

func (s *conflictOnceStore) AppendAtomic(ctx context.Context, writes ...eventstore.StreamWrite) error {
	if !s.conflicted {
		s.conflicted = true
		s.onConflict()
		return eventstore.ErrConcurrencyConflict
	}
	return s.Store.AppendAtomic(ctx, writes...)
}

type failingStore struct{ err error }

func (s *failingStore) Append(context.Context, string, string, int64, ...proto.Message) error {
	return s.err
}

func (s *failingStore) AppendAtomic(context.Context, ...eventstore.StreamWrite) error {
	return s.err
}

func (s *failingStore) Load(context.Context, string, string) ([]eventstore.Event, error) {
	return nil, s.err
}
