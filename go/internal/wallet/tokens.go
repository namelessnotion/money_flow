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
func TokenMintedForWalletEvent(walletID, tokenID string, capacity *sharedpb.Money) *pb.TokenMintedForWallet {
	return &pb.TokenMintedForWallet{Id: walletID, TokenId: tokenID, Capacity: capacity}
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
