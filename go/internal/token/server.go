package token

import (
	"context"
	"errors"
	"fmt"

	"github.com/twitchtv/twirp"
	"google.golang.org/protobuf/proto"

	sharedpb "github.com/namelessnotion/money_flow/go/gen/proto/shared/v1"
	pb "github.com/namelessnotion/money_flow/go/gen/proto/token/v1"
	walletpb "github.com/namelessnotion/money_flow/go/gen/proto/wallet/v1"
	"github.com/namelessnotion/money_flow/go/internal/eventstore"
	"github.com/namelessnotion/money_flow/go/internal/id"
	"github.com/namelessnotion/money_flow/go/internal/ledger"
	"github.com/namelessnotion/money_flow/go/internal/money"
	"github.com/namelessnotion/money_flow/go/internal/wallet"
)

// AggregateType is this aggregate's stream namespace in the event log.
// Exported, like wallet's, because transfer's saga names Token streams
// directly when minting a Transfer's destination(s).
const AggregateType = "token"

var _ pb.TokenService = (*Server)(nil)

// Server implements the Token Twirp service. Minting crosses into
// TigerBeetle, so unlike Holder/Wallet's Server it also holds a
// ledger.Client.
type Server struct {
	store  eventstore.Store
	ledger ledger.Client
}

func NewServer(store eventstore.Store, lc ledger.Client) *Server {
	return &Server{store: store, ledger: lc}
}

// MintSpec is one Token to mint into a Wallet: its id (caller-generated,
// must be a valid UUID — it doubles as the TigerBeetle account id) and
// fixed capacity.
type MintSpec struct {
	TokenID  string
	Capacity *sharedpb.Money
}

// Minted returns the TokenMinted event for id, or nil when the Token has not
// been minted yet. A package function, not a method, so transfer's saga can
// check it without holding a Server.
func Minted(ctx context.Context, store eventstore.Store, tokenID string) (*pb.TokenMinted, error) {
	events, err := store.Load(ctx, AggregateType, tokenID)
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}
	if len(events) == 0 {
		return nil, nil
	}
	msg, err := events[0].Decode()
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}
	minted, ok := msg.(*pb.TokenMinted)
	if !ok {
		return nil, twirp.InternalError(fmt.Sprintf(
			"token %q: stream starts with %s, want TokenMinted", tokenID, events[0].EventType,
		))
	}
	return minted, nil
}

// MintWrites builds the StreamWrite(s) for minting one or more Tokens into
// walletID in a single TigerBeetle-linked batch: one new Token stream per
// spec (carrying TokenMinted), plus one write to the Wallet's own stream
// carrying every TokenMintedForWallet event.
//
// Unlike wallet.OpenedEvent, this is not pure: it calls
// ledger.CreateAccounts before it can honestly hand back a TokenMinted
// event, because recording one before its TigerBeetle account exists would
// be a lie the log could never take back. That's the one deliberate
// divergence from the Holder/Wallet "pure helper" precedent, forced by
// crossing into an external system rather than a taste choice.
//
// walletEvents must be the Wallet's full, already-loaded stream: its first
// event is decoded for the Wallet's Allows policy, and its length becomes
// the Wallet write's ExpectedSeq. Callers that also need the Wallet's
// existing Token ids (e.g. transfer's source-Token selection) already have
// this stream loaded for that purpose and can reuse it here for free.
func MintWrites(
	ctx context.Context, store eventstore.Store, lc ledger.Client,
	walletID string, walletEvents []eventstore.Event, specs []MintSpec,
) ([]eventstore.StreamWrite, *pb.TokenMintRejected, error) {
	if len(walletEvents) == 0 {
		return nil, &pb.TokenMintRejected{
			WalletId: walletID,
			Reason:   fmt.Sprintf("wallet %q has not been opened", walletID),
		}, nil
	}
	msg, err := walletEvents[0].Decode()
	if err != nil {
		return nil, nil, twirp.InternalErrorWith(err)
	}
	opened, ok := msg.(*walletpb.WalletOpened)
	if !ok {
		return nil, nil, twirp.InternalError(fmt.Sprintf(
			"wallet %q: stream starts with %s, want WalletOpened", walletID, walletEvents[0].EventType,
		))
	}

	flags, err := AllowsToAccountFlags(opened.GetAllows())
	if err != nil {
		return nil, nil, twirp.InternalErrorWith(fmt.Errorf("wallet %q: %w", walletID, err))
	}

	accounts := make([]ledger.Account, len(specs))
	for i, spec := range specs {
		accounts[i] = ledger.Account{
			ID: spec.TokenID, Currency: spec.Capacity.GetCurrency(), Flags: flags,
		}
		accounts[i].Flags.Linked = i < len(specs)-1
	}

	results, err := lc.CreateAccounts(ctx, accounts)
	if err != nil {
		return nil, nil, twirp.InternalErrorWith(fmt.Errorf("ledger: %w", err))
	}
	for i, r := range results {
		switch r.Result {
		case ledger.AccountResultOK, ledger.AccountResultExists:
			continue
		default:
			return nil, &pb.TokenMintRejected{
				WalletId: walletID,
				Reason:   fmt.Sprintf("tigerbeetle rejected token %q: %v", specs[i].TokenID, r.Result),
			}, nil
		}
	}

	writes := make([]eventstore.StreamWrite, 0, len(specs)+1)
	walletTokenEvents := make([]proto.Message, len(specs))
	for i, spec := range specs {
		writes = append(writes, eventstore.StreamWrite{
			AggregateType: AggregateType, AggregateID: spec.TokenID, ExpectedSeq: 0,
			Events: []proto.Message{&pb.TokenMinted{Id: spec.TokenID, WalletId: walletID, Capacity: spec.Capacity}},
		})
		walletTokenEvents[i] = wallet.TokenMintedForWalletEvent(walletID, spec.TokenID, spec.Capacity)
	}
	writes = append(writes, eventstore.StreamWrite{
		AggregateType: wallet.AggregateType, AggregateID: walletID, ExpectedSeq: int64(len(walletEvents)),
		Events: walletTokenEvents,
	})

	return writes, nil, nil
}

// Mint records TokenMinted for req.Id, backed by a new TigerBeetle account
// under req.WalletId.
//
// Idempotent, like Holder/Wallet: a Token that already exists is returned
// as-is. Beyond that single-id convergence, Mint also retries past a
// concurrency conflict that isn't about this Token at all — two Mints for
// different Tokens racing to append to the *same Wallet's* stream — by
// reloading the Wallet's current stream and trying again with a fresh
// ExpectedSeq, since ledger.CreateAccounts is itself idempotent and safe to
// resubmit.
func (s *Server) Mint(ctx context.Context, req *pb.MintRequest) (*pb.MintResponse, error) {
	if err := id.Validate("id", req.GetId()); err != nil {
		return nil, err
	}
	if err := id.Validate("wallet_id", req.GetWalletId()); err != nil {
		return nil, err
	}
	if err := money.Validate("capacity", req.GetCapacity()); err != nil {
		return nil, err
	}

	existing, err := Minted(ctx, s.store, req.GetId())
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return mintedResponse(existing), nil
	}

	const maxAttempts = 5
	for attempt := 0; attempt < maxAttempts; attempt++ {
		walletEvents, err := s.store.Load(ctx, wallet.AggregateType, req.GetWalletId())
		if err != nil {
			return nil, twirp.InternalErrorWith(err)
		}

		writes, rejection, err := MintWrites(ctx, s.store, s.ledger, req.GetWalletId(), walletEvents,
			[]MintSpec{{TokenID: req.GetId(), Capacity: req.GetCapacity()}})
		if err != nil {
			return nil, err
		}
		if rejection != nil {
			rejection.Id = req.GetId()
			return &pb.MintResponse{
				Id:     req.GetId(),
				Result: &pb.MintResponse_TokenMintRejected{TokenMintRejected: rejection},
			}, nil
		}

		switch err := s.store.AppendAtomic(ctx, writes...); {
		case err == nil:
			return mintedResponse(&pb.TokenMinted{
				Id: req.GetId(), WalletId: req.GetWalletId(), Capacity: req.GetCapacity(),
			}), nil

		case errors.Is(err, eventstore.ErrConcurrencyConflict):
			existing, err := Minted(ctx, s.store, req.GetId())
			if err != nil {
				return nil, err
			}
			if existing != nil {
				// A concurrent Mint for this same id landed first.
				return mintedResponse(existing), nil
			}
			// Some other write advanced the Wallet's stream first (a
			// different Token being minted concurrently) — retry with a
			// fresh ExpectedSeq rather than surfacing the race.
			continue

		default:
			return nil, twirp.InternalErrorWith(err)
		}
	}
	return nil, twirp.InternalError("token: mint could not converge after repeated concurrency conflicts")
}

func mintedResponse(e *pb.TokenMinted) *pb.MintResponse {
	return &pb.MintResponse{Id: e.GetId(), Result: &pb.MintResponse_TokenMinted{TokenMinted: e}}
}
