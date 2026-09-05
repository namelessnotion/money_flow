package wallet

import (
	"context"

	"github.com/twitchtv/twirp"

	sharedpb "github.com/namelessnotion/money_flow/go/gen/proto/shared/v1"
	pb "github.com/namelessnotion/money_flow/go/gen/proto/wallet/v1"
	"github.com/namelessnotion/money_flow/go/internal/eventstore"
)

// TokenMintedForWalletEvent builds the event token.Mint records onto a
// Wallet's own stream, alongside the Token's own TokenMinted, in the same
// atomic append. Pure — no I/O — so transfer's saga can build it without a
// Server, the same reason wallet.OpenedEvent is a free function.
//
// transactionID tags the Token with its owning Transaction, if any (empty
// for a Token minted outside any Transaction) — see TokensOfVisibleTo.
func TokenMintedForWalletEvent(walletID, tokenID string, capacity *sharedpb.Money, transactionID string) *pb.TokenMintedForWallet {
	return &pb.TokenMintedForWallet{Id: walletID, TokenId: tokenID, Capacity: capacity, TransactionId: transactionID}
}

// TokensOf returns the ids of every Token minted for walletID, oldest first
// — the order their TokenMintedForWallet events were appended to the
// Wallet's own stream. This is what lets source-Token selection enumerate a
// Wallet's Tokens in mint order without scanning every Token stream in the
// system. A Wallet that doesn't exist, or exists but has no Tokens yet,
// returns an empty slice.
func TokensOf(ctx context.Context, store eventstore.Store, walletID string) ([]string, error) {
	events, err := store.Load(ctx, AggregateType, walletID)
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}

	var tokens []string
	for _, event := range events {
		msg, err := event.Decode()
		if err != nil {
			return nil, twirp.InternalErrorWith(err)
		}
		if minted, ok := msg.(*pb.TokenMintedForWallet); ok {
			tokens = append(tokens, minted.GetTokenId())
		}
	}
	return tokens, nil
}

// TransactionOpenChecker reports whether transactionID (as recorded on a
// TokenMintedForWallet tag) is still open — has not yet reached a terminal
// state, and so may still need to reverse the Token it tagged. wallet has no
// notion of what a "Transaction" even is; this indirection lets it ask the
// question without importing the transaction package, the same reason
// ledger.Client is an interface transfer depends on rather than a concrete
// TigerBeetle type. A nil checker means "treat every tag as already
// resolved" — i.e. behaves exactly like TokensOf, for callers that don't
// need transaction-awareness at all.
type TransactionOpenChecker func(ctx context.Context, transactionID string) (bool, error)

// TokensOfVisibleTo is TokensOf, filtered by ownership: a Token tagged with
// a DIFFERENT, still-open Transaction is skipped — hidden from FIFO source
// selection so that Transaction's own compensation can't lose a race against
// an outside caller spending the same Token down first. A Token that is
// untagged, tagged with callingTransactionID itself (required for a
// Transaction's own DAG to chain its Transfers together), or tagged with a
// Transaction that has since closed, is included.
func TokensOfVisibleTo(ctx context.Context, store eventstore.Store, walletID, callingTransactionID string, isOpen TransactionOpenChecker) ([]string, error) {
	events, err := store.Load(ctx, AggregateType, walletID)
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}

	var visible []string
	for _, event := range events {
		msg, err := event.Decode()
		if err != nil {
			return nil, twirp.InternalErrorWith(err)
		}
		minted, ok := msg.(*pb.TokenMintedForWallet)
		if !ok {
			continue
		}

		tag := minted.GetTransactionId()
		if tag == "" || tag == callingTransactionID || isOpen == nil {
			visible = append(visible, minted.GetTokenId())
			continue
		}
		open, err := isOpen(ctx, tag)
		if err != nil {
			return nil, err
		}
		if !open {
			visible = append(visible, minted.GetTokenId())
		}
	}
	return visible, nil
}
