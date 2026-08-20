package holder

import (
	"context"
	"errors"
	"fmt"

	"github.com/twitchtv/twirp"

	pb "github.com/namelessnotion/money_flow/go/gen/proto/holder/v1"
	"github.com/namelessnotion/money_flow/go/internal/eventstore"
)

// aggregateType is this aggregate's stream namespace in the event log.
const aggregateType = "holder"

var _ pb.HolderService = (*Server)(nil)

// Server implements the Holder Twirp service. Commands are validated and then
// recorded as Domain Events; the event log is the source of truth for intent.
type Server struct {
	store eventstore.Store
}

func NewServer(store eventstore.Store) *Server {
	return &Server{store: store}
}

// Establish records HolderEstablished for req.Id.
//
// It is idempotent: a Holder that already exists is returned as-is rather than
// re-established or reported as an error. Callers generate the id themselves
// (Ruby's OnboardEntity does, before it ever calls here), so a retry after an
// ambiguous failure has to converge on one Holder rather than conflict.
func (s *Server) Establish(ctx context.Context, req *pb.EstablishRequest) (*pb.EstablishResponse, error) {
	if req.GetId() == "" {
		return nil, twirp.RequiredArgumentError("id")
	}

	existing, err := s.established(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return establishResponse(existing), nil
	}

	event := &pb.HolderEstablished{Id: req.GetId(), Memos: req.GetMemos()}

	switch err := s.store.Append(ctx, aggregateType, req.GetId(), 0, event); {
	case err == nil:
		return establishResponse(event), nil

	case errors.Is(err, eventstore.ErrConcurrencyConflict):
		// A concurrent Establish for this id landed first. Idempotency means
		// returning that winner, not surfacing the race to the caller.
		existing, err := s.established(ctx, req.GetId())
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, twirp.InternalError("holder: append conflicted but no holder was found")
		}
		return establishResponse(existing), nil

	default:
		return nil, twirp.InternalErrorWith(err)
	}
}

// AddWallet is not implemented yet. Adding a Wallet to a Holder is a
// three-step saga (OpenWallet -> WalletOpened, then AddWalletToHolder ->
// HolderAddedWallet), which needs the Wallet aggregate to exist first.
func (s *Server) AddWallet(ctx context.Context, req *pb.AddWalletRequest) (*pb.AddWalletResponse, error) {
	return nil, twirp.NewError(twirp.Unimplemented, "holder: AddWallet is not implemented")
}

// established returns the HolderEstablished event for id, or nil when the
// Holder does not exist yet.
func (s *Server) established(ctx context.Context, id string) (*pb.HolderEstablished, error) {
	events, err := s.store.Load(ctx, aggregateType, id)
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
	established, ok := msg.(*pb.HolderEstablished)
	if !ok {
		return nil, twirp.InternalError(fmt.Sprintf(
			"holder %q: stream starts with %s, want HolderEstablished", id, events[0].EventType,
		))
	}
	return established, nil
}

func establishResponse(e *pb.HolderEstablished) *pb.EstablishResponse {
	return &pb.EstablishResponse{
		Id:     e.GetId(),
		Result: &pb.EstablishResponse_HolderEstablished{HolderEstablished: e},
	}
}
