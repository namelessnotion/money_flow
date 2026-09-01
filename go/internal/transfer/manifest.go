package transfer

import (
	"context"
	"fmt"

	"uuid"

	"github.com/twitchtv/twirp"

	sharedpb "github.com/namelessnotion/money_flow/go/gen/proto/shared/v1"
	pb "github.com/namelessnotion/money_flow/go/gen/proto/transfer/v1"
	"github.com/namelessnotion/money_flow/go/internal/eventstore"
	"github.com/namelessnotion/money_flow/go/internal/ledger"
	"github.com/namelessnotion/money_flow/go/internal/token"
	"github.com/namelessnotion/money_flow/go/internal/wallet"
)

// Leg is one DEBIT/CREDIT pairing within a Transfer: an amount moving from
// SourceTokenID to DestTokenID. selectSourceTokens fills in only
// SourceTokenID and Amount — DestTokenID is joined in once prepare() has
// decided (or, for a forward Transfer, minted) the destination(s).
type Leg struct {
	SourceTokenID string
	DestTokenID   string
	Amount        *sharedpb.Money
}

// selectSourceTokens picks existing Token(s) belonging to walletID, oldest-
// minted-first (FIFO), accumulating until amount is covered. This is what
// produces the many-to-one case when a single Token's balance can't cover
// the full amount, and the one-to-one case when it can.
//
// ASSUMPTION — source Token selection strategy: oldest-minted-first. This is
// a documented, swappable choice, isolated to this one function; an
// alternative (e.g. largest-balance-first, to minimize the number of legs)
// is easy to substitute here without touching anything else.
//
// Returns a domain rejection, not an error, when the Wallet's total existing
// Token capacity can't cover amount.
func selectSourceTokens(
	ctx context.Context, store eventstore.Store, lc ledger.Client, walletID string, amount *sharedpb.Money,
) ([]Leg, *pb.TransferRequestRejected, error) {
	tokenIDs, err := wallet.TokensOf(ctx, store, walletID)
	if err != nil {
		return nil, nil, err
	}

	var legs []Leg
	remaining := amount.GetMinorUnits()
	for _, tokenID := range tokenIDs {
		if remaining == 0 {
			break
		}
		balance, found, err := lc.AccountBalance(ctx, tokenID)
		if err != nil {
			return nil, nil, twirp.InternalErrorWith(fmt.Errorf("ledger: AccountBalance: %w", err))
		}
		if !found || balance <= 0 {
			continue
		}
		take := uint64(balance)
		if take > remaining {
			take = remaining
		}
		legs = append(legs, Leg{
			SourceTokenID: tokenID,
			Amount:        &sharedpb.Money{MinorUnits: take, Currency: amount.GetCurrency()},
		})
		remaining -= take
	}

	if remaining > 0 {
		return nil, &pb.TransferRequestRejected{
			Reason: fmt.Sprintf(
				"wallet %q has insufficient Token capacity: %d %s short",
				walletID, remaining, amount.GetCurrency(),
			),
		}, nil
	}
	return legs, nil, nil
}

// planDestinations decides how many new destination Tokens to mint for a
// forward Transfer and how the amount splits across them, generating a
// fresh id for each.
//
// PROPOSED ASSUMPTION, per decision #2: v1 always mints exactly one
// destination Token for the full amount. One-to-many is satisfied entirely
// by reversing a many-to-one Transfer instead — see reversalManifest — so
// no "max capacity per Token" splitting rule is needed here.
func planDestinations(amount *sharedpb.Money) []token.MintSpec {
	return []token.MintSpec{{TokenID: uuid.NewV7().String(), Capacity: amount}}
}

// reversalManifest derives a reversal's legs from the original Transfer's
// own recorded TransferPrepared.legs: original DEBIT (source→dest, amt)
// becomes reversal DEBIT (dest→source, amt), one leg per original leg. No
// Token minting, no independent selection — the destinations are always the
// original Transfer's own source Token(s), which already exist by
// construction. This is why decision #1's repeated-destination shape and
// decision #4's reversal-as-new-Transfer compose for free: an original
// many-to-one forward Transfer (N sources → 1 destination) reverses into a
// one-to-many reversal (1 source → N destinations).
//
// Returns a domain rejection, not an error, when the original Transfer
// doesn't exist or was never committed.
func reversalManifest(ctx context.Context, store eventstore.Store, originalTransferID string) ([]Leg, *pb.ReversalRequestRejected, error) {
	events, err := store.Load(ctx, AggregateType, originalTransferID)
	if err != nil {
		return nil, nil, twirp.InternalErrorWith(err)
	}
	if len(events) == 0 {
		return nil, &pb.ReversalRequestRejected{
			TransferId: originalTransferID, Reason: "original transfer not found",
		}, nil
	}
	if state := currentState(events); state != stateCommitted {
		return nil, &pb.ReversalRequestRejected{
			TransferId: originalTransferID,
			Reason:     fmt.Sprintf("original transfer is %s, not committed", state),
		}, nil
	}

	var legs []Leg
	for _, e := range events {
		if e.EventType != eventstore.EventType(&pb.TransferPrepared{}) {
			continue
		}
		msg, err := e.Decode()
		if err != nil {
			return nil, nil, twirp.InternalErrorWith(err)
		}
		prepared, ok := msg.(*pb.TransferPrepared)
		if !ok {
			return nil, nil, twirp.InternalError(fmt.Sprintf(
				"transfer %q: event typed %s did not decode as TransferPrepared", originalTransferID, e.EventType,
			))
		}
		for _, l := range prepared.GetLegs() {
			legs = append(legs, Leg{
				SourceTokenID: l.GetDestTokenId(),
				DestTokenID:   l.GetSourceTokenId(),
				Amount:        l.GetAmount(),
			})
		}
	}
	if len(legs) == 0 {
		return nil, nil, twirp.InternalError(fmt.Sprintf(
			"transfer %q: committed but has no recorded legs", originalTransferID,
		))
	}
	return legs, nil, nil
}
