package transaction

import (
	"context"
	"testing"

	sharedpb "github.com/namelessnotion/money_flow/go/gen/proto/shared/v1"
	pb "github.com/namelessnotion/money_flow/go/gen/proto/transaction/v1"
	"github.com/namelessnotion/money_flow/go/internal/eventstore"
	"github.com/namelessnotion/money_flow/go/internal/testutil"
)

func usd(minorUnits uint64) *sharedpb.Money {
	return &sharedpb.Money{MinorUnits: minorUnits, Currency: "USD"}
}

// gatedChildDAG builds a single-root, auto_process=false DAG — enough to
// exercise StartInitializingTransaction's own accept/reject/idempotency
// without needing a working transferClient, since a Gated child never
// reaches into it.
func gatedChildDAG(childID string) map[string]*pb.Transfer {
	return map[string]*pb.Transfer{
		childID: {
			Id: childID, Amount: usd(400),
			FromWalletId: testutil.ID("w1"), ToWalletId: testutil.ID("w2"),
			AutoProcess: false,
		},
	}
}

func TestStartInitializingTransaction_Accepts(t *testing.T) {
	t.Parallel()
	store := eventstore.NewMemoryStore()
	server := NewServer(store, nil)
	ctx := context.Background()

	txnID := testutil.ID("txn1")
	childID := testutil.ID("xfer1")
	resp, err := server.StartInitializingTransaction(ctx, &pb.StartInitializingTransactionRequest{
		Id: txnID, Transfers: gatedChildDAG(childID),
	})
	if err != nil {
		t.Fatalf("StartInitializingTransaction() error = %v", err)
	}
	if resp.GetTransactionInitialized() == nil {
		t.Fatalf("result = %v, want TransactionInitialized", resp.GetResult())
	}

	events, err := store.Load(ctx, AggregateType, txnID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if topLevelState(events) != stateStarted {
		t.Fatalf("state = %v, want started (runSaga should have moved Initialized -> Started -> gated child)", topLevelState(events))
	}
	children, err := foldChildStates(events)
	if err != nil {
		t.Fatalf("foldChildStates() error = %v", err)
	}
	if children[childID] != childGated {
		t.Errorf("child state = %v, want gated (auto_process=false)", children[childID])
	}
}

func TestStartInitializingTransaction_RejectsCyclicDAG(t *testing.T) {
	t.Parallel()
	store := eventstore.NewMemoryStore()
	server := NewServer(store, nil)
	ctx := context.Background()

	childID := testutil.ID("xfer1")
	spec := gatedChildDAG(childID)[childID]
	txnID := testutil.ID("txn1")
	resp, err := server.StartInitializingTransaction(ctx, &pb.StartInitializingTransactionRequest{
		Id:        txnID,
		Transfers: map[string]*pb.Transfer{childID: spec},
		TransferDependency: map[string]*pb.TransferIdList{
			childID: {TransferId: []string{childID}}, // self-dependency
		},
	})
	if err != nil {
		t.Fatalf("StartInitializingTransaction() error = %v", err)
	}
	if resp.GetTransactionRejected() == nil {
		t.Fatalf("result = %v, want TransactionRejected", resp.GetResult())
	}

	events, err := store.Load(ctx, AggregateType, txnID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("stream holds %d events, want 1 (the rejection itself)", len(events))
	}
	if events[0].EventType != "transaction.v1.TransactionRejected" {
		t.Errorf("recorded event type = %q, want TransactionRejected", events[0].EventType)
	}
}

func TestStartInitializingTransaction_RejectsEmptyTransfers(t *testing.T) {
	t.Parallel()
	store := eventstore.NewMemoryStore()
	server := NewServer(store, nil)
	ctx := context.Background()

	resp, err := server.StartInitializingTransaction(ctx, &pb.StartInitializingTransactionRequest{Id: testutil.ID("txn1")})
	if err != nil {
		t.Fatalf("StartInitializingTransaction() error = %v", err)
	}
	if resp.GetTransactionRejected() == nil {
		t.Fatalf("result = %v, want TransactionRejected", resp.GetResult())
	}
}

func TestStartInitializingTransaction_IsIdempotent(t *testing.T) {
	t.Parallel()
	store := eventstore.NewMemoryStore()
	server := NewServer(store, nil)
	ctx := context.Background()

	txnID := testutil.ID("txn1")
	req := &pb.StartInitializingTransactionRequest{Id: txnID, Transfers: gatedChildDAG(testutil.ID("xfer1"))}
	first, err := server.StartInitializingTransaction(ctx, req)
	if err != nil {
		t.Fatalf("first StartInitializingTransaction() error = %v", err)
	}
	second, err := server.StartInitializingTransaction(ctx, req)
	if err != nil {
		t.Fatalf("second StartInitializingTransaction() error = %v", err)
	}
	if first.GetTransactionInitialized().GetId() != second.GetTransactionInitialized().GetId() {
		t.Errorf("replay result = %v, want the same recorded outcome", second.GetResult())
	}

	events, err := store.Load(ctx, AggregateType, txnID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	// TransactionInitialized, TransactionStarted, TransferGatedWithinTransaction — no
	// duplicates from the second call.
	if len(events) != 3 {
		t.Errorf("stream holds %d events, want 3 (no duplicate work on replay); types = %v", len(events), eventTypesOf(events))
	}
}

func TestStartInitializingTransaction_RejectedRequestIsIdempotent(t *testing.T) {
	t.Parallel()
	store := eventstore.NewMemoryStore()
	server := NewServer(store, nil)
	ctx := context.Background()

	txnID := testutil.ID("txn1")
	req := &pb.StartInitializingTransactionRequest{Id: txnID} // empty transfers -> rejected
	if _, err := server.StartInitializingTransaction(ctx, req); err != nil {
		t.Fatalf("first StartInitializingTransaction() error = %v", err)
	}
	second, err := server.StartInitializingTransaction(ctx, req)
	if err != nil {
		t.Fatalf("second StartInitializingTransaction() error = %v", err)
	}
	if second.GetTransactionRejected() == nil {
		t.Fatalf("result = %v, want TransactionRejected on replay", second.GetResult())
	}

	events, err := store.Load(ctx, AggregateType, txnID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(events) != 1 {
		t.Errorf("stream holds %d events, want 1 (no duplicate rejection on replay)", len(events))
	}
}

func TestIsOpen_InitializedAndStartedAreOpen(t *testing.T) {
	t.Parallel()
	store := eventstore.NewMemoryStore()
	server := NewServer(store, nil)
	ctx := context.Background()

	txnID := testutil.ID("txn1")
	if _, err := server.StartInitializingTransaction(ctx, &pb.StartInitializingTransactionRequest{
		Id: txnID, Transfers: gatedChildDAG(testutil.ID("xfer1")),
	}); err != nil {
		t.Fatalf("StartInitializingTransaction() error = %v", err)
	}

	open, err := IsOpen(ctx, store, txnID)
	if err != nil {
		t.Fatalf("IsOpen() error = %v", err)
	}
	if !open {
		t.Error("IsOpen() = false, want true (Started is an open state)")
	}
}

func TestIsOpen_UnknownTransactionDefaultsOpen(t *testing.T) {
	t.Parallel()
	store := eventstore.NewMemoryStore()

	open, err := IsOpen(context.Background(), store, testutil.ID("never-existed"))
	if err != nil {
		t.Fatalf("IsOpen() error = %v", err)
	}
	if !open {
		t.Error("IsOpen() = false, want true (defensive default: fail toward blocking on a not-found stream)")
	}
}

func TestExists_UnknownTransactionIsFalse(t *testing.T) {
	t.Parallel()
	store := eventstore.NewMemoryStore()

	exists, err := Exists(context.Background(), store, testutil.ID("never-existed"))
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if exists {
		t.Error("Exists() = true, want false (opposite default from IsOpen: a claimed id must resolve to a real stream)")
	}
}

func TestExists_InitializedTransactionIsTrue(t *testing.T) {
	t.Parallel()
	store := eventstore.NewMemoryStore()
	server := NewServer(store, nil)
	ctx := context.Background()

	txnID := testutil.ID("txn1")
	if _, err := server.StartInitializingTransaction(ctx, &pb.StartInitializingTransactionRequest{
		Id: txnID, Transfers: gatedChildDAG(testutil.ID("xfer1")),
	}); err != nil {
		t.Fatalf("StartInitializingTransaction() error = %v", err)
	}

	exists, err := Exists(ctx, store, txnID)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("Exists() = false, want true")
	}
}

func eventTypesOf(events []eventstore.Event) []string {
	types := make([]string, len(events))
	for i, e := range events {
		types[i] = e.EventType
	}
	return types
}
