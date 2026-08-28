package wallet

import (
	"context"
	"errors"
	"fmt"

	"github.com/twitchtv/twirp"

	sharedpb "github.com/namelessnotion/money_flow/go/gen/proto/shared/v1"
	pb "github.com/namelessnotion/money_flow/go/gen/proto/wallet/v1"
	"github.com/namelessnotion/money_flow/go/internal/eventstore"
	"github.com/namelessnotion/money_flow/go/internal/id"
)

// AggregateType is this aggregate's stream namespace in the event log.
// Exported, unlike Holder's, because provisioning a Holder writes to Wallet
// streams and has to name them exactly as Open does.
const AggregateType = "wallet"

var _ pb.WalletService = (*Server)(nil)

// Server implements the Wallet Twirp service.
type Server struct {
	store eventstore.Store
}

func NewServer(store eventstore.Store) *Server {
	return &Server{store: store}
}

// OpenedEvent builds the event Open records. Provisioning uses it too, so a
// Wallet opened alongside its Holder writes exactly the history a direct Open
// would have produced.
func OpenedEvent(walletID, holderID, name string, allows sharedpb.Allows) *pb.WalletOpened {
	return &pb.WalletOpened{Id: walletID, HolderId: holderID, Name: name, Allows: allows}
}

// ValidateWallet checks what every caller must check about a Wallet's own id
// and access policy, independent of whose Holder it belongs to. Split out
// from ValidateOpen so AddWallet and Provision — which already validate the
// holder id themselves, against their own request's "id" field — don't also
// re-validate it here under the name "holder_id" for every Wallet.
func ValidateWallet(walletID string, allows sharedpb.Allows) error {
	if err := id.Validate("id", walletID); err != nil {
		return err
	}
	// The caller has to state the policy: an unset field is a bug, and
	// ALLOWS_NONE exists for deliberately permitting neither direction.
	if allows == sharedpb.Allows_ALLOWS_UNSPECIFIED {
		return twirp.InvalidArgumentError("allows", "must be set; use ALLOWS_NONE to permit neither onramp nor offramp")
	}
	return nil
}

// ValidateOpen checks what every caller must check before a Wallet is opened.
// Open is the only caller that hasn't already validated the holder id itself,
// so it's the only one that needs the composed check.
func ValidateOpen(walletID, holderID string, allows sharedpb.Allows) error {
	if err := ValidateWallet(walletID, allows); err != nil {
		return err
	}
	return id.Validate("holder_id", holderID)
}

// Opened returns the WalletOpened event for id, or nil when the Wallet does not
// exist yet. A package function rather than a method so provisioning can reuse
// the read-before-write without holding a Server.
func Opened(ctx context.Context, store eventstore.Store, id string) (*pb.WalletOpened, error) {
	events, err := store.Load(ctx, AggregateType, id)
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
	opened, ok := msg.(*pb.WalletOpened)
	if !ok {
		return nil, twirp.InternalError(fmt.Sprintf(
			"wallet %q: stream starts with %s, want WalletOpened", id, events[0].EventType,
		))
	}
	return opened, nil
}

// Open records WalletOpened for req.Id.
//
// Idempotent, like Holder's Establish: a Wallet that already exists is returned
// as-is, and a changed name or allows on replay is ignored rather than
// rewriting the recorded event.
func (s *Server) Open(ctx context.Context, req *pb.OpenRequest) (*pb.OpenResponse, error) {
	if err := ValidateOpen(req.GetId(), req.GetHolderId(), req.GetAllows()); err != nil {
		return nil, err
	}

	existing, err := Opened(ctx, s.store, req.GetId())
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return replayResponse(existing, req.GetHolderId())
	}

	event := OpenedEvent(req.GetId(), req.GetHolderId(), req.GetName(), req.GetAllows())

	switch err := s.store.Append(ctx, AggregateType, req.GetId(), 0, event); {
	case err == nil:
		return openedResponse(event), nil

	case errors.Is(err, eventstore.ErrConcurrencyConflict):
		// A concurrent Open for this id landed first. Idempotency means
		// returning that winner rather than surfacing the race.
		existing, err := Opened(ctx, s.store, req.GetId())
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, twirp.InternalError("wallet: append conflicted but no wallet was found")
		}
		return replayResponse(existing, req.GetHolderId())

	default:
		return nil, twirp.InternalErrorWith(err)
	}
}

// replayResponse returns an already-open Wallet, but refuses when the caller
// names a different Holder. Silently reporting success for "open wallet W under
// holder B" when W belongs to holder A would hand the caller someone else's
// Wallet — worse than an error, and the one case where idempotency must yield
// to correctness.
func replayResponse(existing *pb.WalletOpened, holderID string) (*pb.OpenResponse, error) {
	if existing.GetHolderId() != holderID {
		return rejectedResponse(existing.GetId(), holderID, fmt.Sprintf(
			"wallet already belongs to holder %q", existing.GetHolderId(),
		)), nil
	}
	return openedResponse(existing), nil
}

func openedResponse(e *pb.WalletOpened) *pb.OpenResponse {
	return &pb.OpenResponse{
		Id:     e.GetId(),
		Result: &pb.OpenResponse_WalletOpened{WalletOpened: e},
	}
}

// rejectedResponse carries a *domain* refusal — a well-formed request the
// Wallet declines. Malformed requests are twirp errors instead (see
// ValidateOpen), matching how Holder's Establish reports them.
func rejectedResponse(walletID, holderID, reason string) *pb.OpenResponse {
	return &pb.OpenResponse{
		Id: walletID,
		Result: &pb.OpenResponse_WalletRejected{
			WalletRejected: &pb.WalletRejected{Id: walletID, HolderId: holderID, Reason: reason},
		},
	}
}
