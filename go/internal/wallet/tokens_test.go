package wallet

import (
	"context"
	"testing"

	sharedpb "github.com/namelessnotion/money_flow/go/gen/proto/shared/v1"
	"github.com/namelessnotion/money_flow/go/internal/eventstore"
	"github.com/namelessnotion/money_flow/go/internal/testutil"
)

func TestTokenMintedForWalletEvent(t *testing.T) {
	t.Parallel()

	capacity := &sharedpb.Money{MinorUnits: 500, Currency: "USD"}
	event := TokenMintedForWalletEvent(testutil.ID("w1"), testutil.ID("t1"), capacity, testutil.ID("txn1"))

	if event.GetId() != testutil.ID("w1") {
		t.Errorf("Id = %q, want %q", event.GetId(), testutil.ID("w1"))
	}
	if event.GetTokenId() != testutil.ID("t1") {
		t.Errorf("TokenId = %q, want %q", event.GetTokenId(), testutil.ID("t1"))
	}
	if event.GetCapacity().GetMinorUnits() != 500 || event.GetCapacity().GetCurrency() != "USD" {
		t.Errorf("Capacity = %+v, want {500 USD}", event.GetCapacity())
	}
	if event.GetTransactionId() != testutil.ID("txn1") {
		t.Errorf("TransactionId = %q, want %q", event.GetTransactionId(), testutil.ID("txn1"))
	}
}

func TestTokensOf_NoWallet(t *testing.T) {
	t.Parallel()

	store := eventstore.NewMemoryStore()
	tokens, err := TokensOf(context.Background(), store, testutil.ID("w1"))
	if err != nil {
		t.Fatalf("TokensOf() error = %v", err)
	}
	if len(tokens) != 0 {
		t.Errorf("TokensOf() = %v, want empty", tokens)
	}
}

func TestTokensOf_NoTokensYet(t *testing.T) {
	t.Parallel()

	store := eventstore.NewMemoryStore()
	ctx := context.Background()
	if _, err := NewServer(store).Open(ctx, openRequest()); err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	tokens, err := TokensOf(ctx, store, testutil.ID("w1"))
	if err != nil {
		t.Fatalf("TokensOf() error = %v", err)
	}
	if len(tokens) != 0 {
		t.Errorf("TokensOf() = %v, want empty", tokens)
	}
}

func TestTokensOf_ReturnsIdsInMintOrder(t *testing.T) {
	t.Parallel()

	store := eventstore.NewMemoryStore()
	ctx := context.Background()
	if _, err := NewServer(store).Open(ctx, openRequest()); err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	capacity := &sharedpb.Money{MinorUnits: 100, Currency: "USD"}
	t1, t2, t3 := testutil.ID("t1"), testutil.ID("t2"), testutil.ID("t3")
	if err := store.Append(ctx, AggregateType, testutil.ID("w1"), 1,
		TokenMintedForWalletEvent(testutil.ID("w1"), t1, capacity, "")); err != nil {
		t.Fatalf("Append(t1) error = %v", err)
	}
	if err := store.Append(ctx, AggregateType, testutil.ID("w1"), 2,
		TokenMintedForWalletEvent(testutil.ID("w1"), t2, capacity, "")); err != nil {
		t.Fatalf("Append(t2) error = %v", err)
	}
	if err := store.Append(ctx, AggregateType, testutil.ID("w1"), 3,
		TokenMintedForWalletEvent(testutil.ID("w1"), t3, capacity, "")); err != nil {
		t.Fatalf("Append(t3) error = %v", err)
	}

	tokens, err := TokensOf(ctx, store, testutil.ID("w1"))
	if err != nil {
		t.Fatalf("TokensOf() error = %v", err)
	}
	want := []string{t1, t2, t3}
	if len(tokens) != len(want) {
		t.Fatalf("TokensOf() = %v, want %v", tokens, want)
	}
	for i := range want {
		if tokens[i] != want[i] {
			t.Errorf("TokensOf()[%d] = %q, want %q", i, tokens[i], want[i])
		}
	}
}

func TestTokensOfVisibleTo_NilCheckerDegradesToTokensOf(t *testing.T) {
	t.Parallel()

	store := eventstore.NewMemoryStore()
	ctx := context.Background()
	if _, err := NewServer(store).Open(ctx, openRequest()); err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	capacity := &sharedpb.Money{MinorUnits: 100, Currency: "USD"}
	t1, t2 := testutil.ID("t1"), testutil.ID("t2")
	if err := store.Append(ctx, AggregateType, testutil.ID("w1"), 1,
		TokenMintedForWalletEvent(testutil.ID("w1"), t1, capacity, "")); err != nil {
		t.Fatalf("Append(t1) error = %v", err)
	}
	// Even a Token tagged with some Transaction is visible when isOpen is
	// nil — the caller doesn't need transaction-awareness at all.
	if err := store.Append(ctx, AggregateType, testutil.ID("w1"), 2,
		TokenMintedForWalletEvent(testutil.ID("w1"), t2, capacity, testutil.ID("txn1"))); err != nil {
		t.Fatalf("Append(t2) error = %v", err)
	}

	tokens, err := TokensOfVisibleTo(ctx, store, testutil.ID("w1"), "", nil)
	if err != nil {
		t.Fatalf("TokensOfVisibleTo() error = %v", err)
	}
	want := []string{t1, t2}
	if len(tokens) != len(want) || tokens[0] != want[0] || tokens[1] != want[1] {
		t.Fatalf("TokensOfVisibleTo() = %v, want %v", tokens, want)
	}
}

func TestTokensOfVisibleTo_HidesTokenTaggedByAnotherOpenTransaction(t *testing.T) {
	t.Parallel()

	store := eventstore.NewMemoryStore()
	ctx := context.Background()
	if _, err := NewServer(store).Open(ctx, openRequest()); err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	capacity := &sharedpb.Money{MinorUnits: 100, Currency: "USD"}
	t1, t2 := testutil.ID("t1"), testutil.ID("t2")
	otherTxn := testutil.ID("txn-other")
	if err := store.Append(ctx, AggregateType, testutil.ID("w1"), 1,
		TokenMintedForWalletEvent(testutil.ID("w1"), t1, capacity, "")); err != nil {
		t.Fatalf("Append(t1) error = %v", err)
	}
	if err := store.Append(ctx, AggregateType, testutil.ID("w1"), 2,
		TokenMintedForWalletEvent(testutil.ID("w1"), t2, capacity, otherTxn)); err != nil {
		t.Fatalf("Append(t2) error = %v", err)
	}

	stillOpen := func(context.Context, string) (bool, error) { return true, nil }
	tokens, err := TokensOfVisibleTo(ctx, store, testutil.ID("w1"), "", stillOpen)
	if err != nil {
		t.Fatalf("TokensOfVisibleTo() error = %v", err)
	}
	if len(tokens) != 1 || tokens[0] != t1 {
		t.Fatalf("TokensOfVisibleTo() = %v, want only the untagged t1 (t2 hidden, tagged by a still-open Transaction)", tokens)
	}
}

func TestTokensOfVisibleTo_RevealsTokenOnceOwningTransactionCloses(t *testing.T) {
	t.Parallel()

	store := eventstore.NewMemoryStore()
	ctx := context.Background()
	if _, err := NewServer(store).Open(ctx, openRequest()); err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	capacity := &sharedpb.Money{MinorUnits: 100, Currency: "USD"}
	t1 := testutil.ID("t1")
	otherTxn := testutil.ID("txn-other")
	if err := store.Append(ctx, AggregateType, testutil.ID("w1"), 1,
		TokenMintedForWalletEvent(testutil.ID("w1"), t1, capacity, otherTxn)); err != nil {
		t.Fatalf("Append(t1) error = %v", err)
	}

	closed := func(context.Context, string) (bool, error) { return false, nil }
	tokens, err := TokensOfVisibleTo(ctx, store, testutil.ID("w1"), "", closed)
	if err != nil {
		t.Fatalf("TokensOfVisibleTo() error = %v", err)
	}
	if len(tokens) != 1 || tokens[0] != t1 {
		t.Fatalf("TokensOfVisibleTo() = %v, want t1 visible once its owning Transaction has closed", tokens)
	}
}

func TestTokensOfVisibleTo_OwnTransactionSeesItsOwnTaggedToken(t *testing.T) {
	t.Parallel()

	store := eventstore.NewMemoryStore()
	ctx := context.Background()
	if _, err := NewServer(store).Open(ctx, openRequest()); err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	capacity := &sharedpb.Money{MinorUnits: 100, Currency: "USD"}
	t1 := testutil.ID("t1")
	ownTxn := testutil.ID("txn-own")
	if err := store.Append(ctx, AggregateType, testutil.ID("w1"), 1,
		TokenMintedForWalletEvent(testutil.ID("w1"), t1, capacity, ownTxn)); err != nil {
		t.Fatalf("Append(t1) error = %v", err)
	}

	// isOpen would say "still open" if called, but a Transaction's own later
	// child must see its own tagged output regardless — required for the
	// DAG to chain its Transfers together. Assert isOpen is never even
	// called for this case.
	isOpenCalled := false
	stillOpen := func(context.Context, string) (bool, error) { isOpenCalled = true; return true, nil }
	tokens, err := TokensOfVisibleTo(ctx, store, testutil.ID("w1"), ownTxn, stillOpen)
	if err != nil {
		t.Fatalf("TokensOfVisibleTo() error = %v", err)
	}
	if len(tokens) != 1 || tokens[0] != t1 {
		t.Fatalf("TokensOfVisibleTo() = %v, want t1 visible to its own owning Transaction", tokens)
	}
	if isOpenCalled {
		t.Error("isOpen was called for the caller's own transaction_id tag, want it skipped entirely")
	}
}
