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
	event := TokenMintedForWalletEvent(testutil.ID("w1"), testutil.ID("t1"), capacity)

	if event.GetId() != testutil.ID("w1") {
		t.Errorf("Id = %q, want %q", event.GetId(), testutil.ID("w1"))
	}
	if event.GetTokenId() != testutil.ID("t1") {
		t.Errorf("TokenId = %q, want %q", event.GetTokenId(), testutil.ID("t1"))
	}
	if event.GetCapacity().GetMinorUnits() != 500 || event.GetCapacity().GetCurrency() != "USD" {
		t.Errorf("Capacity = %+v, want {500 USD}", event.GetCapacity())
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
		TokenMintedForWalletEvent(testutil.ID("w1"), t1, capacity)); err != nil {
		t.Fatalf("Append(t1) error = %v", err)
	}
	if err := store.Append(ctx, AggregateType, testutil.ID("w1"), 2,
		TokenMintedForWalletEvent(testutil.ID("w1"), t2, capacity)); err != nil {
		t.Fatalf("Append(t2) error = %v", err)
	}
	if err := store.Append(ctx, AggregateType, testutil.ID("w1"), 3,
		TokenMintedForWalletEvent(testutil.ID("w1"), t3, capacity)); err != nil {
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
