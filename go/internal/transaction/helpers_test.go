package transaction

import (
	"context"
	"testing"

	sharedpb "github.com/namelessnotion/money_flow/go/gen/proto/shared/v1"
	tokenpb "github.com/namelessnotion/money_flow/go/gen/proto/token/v1"
	walletpb "github.com/namelessnotion/money_flow/go/gen/proto/wallet/v1"
	"github.com/namelessnotion/money_flow/go/internal/eventstore"
	"github.com/namelessnotion/money_flow/go/internal/ledger"
	"github.com/namelessnotion/money_flow/go/internal/testutil"
	"github.com/namelessnotion/money_flow/go/internal/token"
	"github.com/namelessnotion/money_flow/go/internal/transfer"
	"github.com/namelessnotion/money_flow/go/internal/wallet"
)

// newTransferServer wires a real *transfer.Server to this package's own
// IsOpen/Exists — the same wiring cmd/server/main.go does — so end-to-end
// tests exercise the real cross-transaction Token-reservation and
// mint_source-guarantee mechanisms, not a fake.
func newTransferServer(store eventstore.Store, lc ledger.Client) *transfer.Server {
	isOpen := func(ctx context.Context, transactionID string) (bool, error) { return IsOpen(ctx, store, transactionID) }
	exists := func(ctx context.Context, transactionID string) (bool, error) { return Exists(ctx, store, transactionID) }
	return transfer.NewServer(store, lc, isOpen, exists)
}

func openWallet(t *testing.T, store eventstore.Store, walletID string, allows sharedpb.Allows) {
	t.Helper()
	if _, err := wallet.NewServer(store).Open(context.Background(), &walletpb.OpenRequest{
		Id: walletID, HolderId: testutil.ID("h1"), Name: "test wallet", Allows: allows,
	}); err != nil {
		t.Fatalf("open wallet: %v", err)
	}
}

// mintAndFundToken opens (if needed) and mints a Token of the given
// capacity into walletID, then gives it a matching posted TigerBeetle
// balance by transferring in from a throwaway external account — for
// wallets that need ordinary, FIFO-selectable pre-existing balance (Cash,
// Uncleared, Cleared), unlike the mint_source wallets (Bank Account, Bank
// Control) whose Tokens are minted fresh by the Transfer saga itself.
func mintAndFundToken(t *testing.T, store eventstore.Store, lc ledger.Client, walletID, tokenID string, capacity *sharedpb.Money) {
	t.Helper()
	ctx := context.Background()
	ts := token.NewServer(store, lc)
	resp, err := ts.Mint(ctx, &tokenpb.MintRequest{Id: tokenID, WalletId: walletID, Capacity: capacity})
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}
	if resp.GetTokenMintRejected() != nil {
		t.Fatalf("mint token rejected: %v", resp.GetTokenMintRejected())
	}

	source := ledger.Account{ID: testutil.ID("external-funding-" + tokenID), Currency: capacity.GetCurrency()}
	if _, err := lc.CreateAccounts(ctx, []ledger.Account{source}); err != nil {
		t.Fatalf("fund token: create external account: %v", err)
	}
	xfer := ledger.Transfer{
		ID: testutil.ID("fund-" + tokenID), DebitAccountID: source.ID, CreditAccountID: tokenID,
		MinorUnits: capacity.GetMinorUnits(), Currency: capacity.GetCurrency(), Kind: ledger.TransferKindRegular,
	}
	results, err := lc.CreateTransfers(ctx, []ledger.Transfer{xfer})
	if err != nil {
		t.Fatalf("fund token: transfer: %v", err)
	}
	if results[0].Result != ledger.TransferResultOK {
		t.Fatalf("fund token: transfer result = %v, want OK", results[0].Result)
	}
}
