package holder

import (
	"context"
	"errors"
	"fmt"

	"github.com/twitchtv/twirp"
	"google.golang.org/protobuf/proto"

	pb "github.com/namelessnotion/money_flow/go/gen/proto/holder/v1"
	"github.com/namelessnotion/money_flow/go/internal/eventstore"
	"github.com/namelessnotion/money_flow/go/internal/id"
	"github.com/namelessnotion/money_flow/go/internal/wallet"
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
	if err := id.Validate("id", req.GetId()); err != nil {
		return nil, err
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

// AddWallet opens one Wallet and records it against an existing Holder.
//
// Both events land in a single atomic append, so a Holder can never reference a
// Wallet whose own stream was never written. Idempotent per wallet_id.
func (s *Server) AddWallet(ctx context.Context, req *pb.AddWalletRequest) (*pb.AddWalletResponse, error) {
	if err := id.Validate("id", req.GetId()); err != nil {
		return nil, err
	}
	if err := wallet.ValidateWallet(req.GetWalletId(), req.GetAllows()); err != nil {
		return nil, err
	}

	events, err := s.stream(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, twirp.NewError(twirp.FailedPrecondition,
			fmt.Sprintf("holder %q has not been established", req.GetId()))
	}

	spec := &pb.WalletSpec{WalletId: req.GetWalletId(), Name: req.GetName(), Allows: req.GetAllows()}
	writes, rejection, err := s.walletWrites(ctx, req.GetId(), events, []*pb.WalletSpec{spec})
	if err != nil {
		return nil, err
	}
	if rejection != nil {
		return &pb.AddWalletResponse{
			Id:     req.GetId(),
			Result: &pb.AddWalletResponse_HolderRejectedWallet{HolderRejectedWallet: rejection},
		}, nil
	}

	if len(writes) > 0 {
		if err := s.appendConverging(ctx, writes); err != nil {
			return nil, err
		}
	}

	return &pb.AddWalletResponse{
		Id: req.GetId(),
		Result: &pb.AddWalletResponse_HolderAddedWallet{
			HolderAddedWallet: &pb.HolderAddedWallet{
				Id: req.GetId(), WalletId: req.GetWalletId(), Name: req.GetName(), Allows: req.GetAllows(),
			},
		},
	}, nil
}

// Provision establishes a Holder and opens its Wallets as one all-or-nothing
// unit, so onboarding can never leave a half-provisioned Holder behind.
//
// Idempotent, including partially: it computes what is missing and appends only
// that, so a retry after an ambiguous failure converges rather than duplicating
// or conflicting. The read-before-write below is not in the same transaction as
// the append and does not need to be — every sequence is re-validated by the
// UNIQUE constraint at INSERT time, so a writer landing in between turns into a
// conflict rather than a lost write.
func (s *Server) Provision(ctx context.Context, req *pb.ProvisionRequest) (*pb.ProvisionResponse, error) {
	if err := id.Validate("id", req.GetId()); err != nil {
		return nil, err
	}
	if len(req.GetWallets()) == 0 {
		return nil, twirp.RequiredArgumentError("wallets")
	}

	seen := make(map[string]struct{}, len(req.GetWallets()))
	for _, spec := range req.GetWallets() {
		if err := wallet.ValidateWallet(spec.GetWalletId(), spec.GetAllows()); err != nil {
			return nil, err
		}
		if _, dup := seen[spec.GetWalletId()]; dup {
			return nil, twirp.InvalidArgumentError("wallets",
				fmt.Sprintf("wallet_id %q appears more than once", spec.GetWalletId()))
		}
		seen[spec.GetWalletId()] = struct{}{}
	}

	events, err := s.stream(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	var writes []eventstore.StreamWrite
	holderEvents := []proto.Message{}
	if len(events) == 0 {
		holderEvents = append(holderEvents, &pb.HolderEstablished{Id: req.GetId(), Memos: req.GetMemos()})
	} else if _, err := decodeEstablished(req.GetId(), events); err != nil {
		return nil, err
	}

	walletWrites, rejection, err := s.walletWrites(ctx, req.GetId(), events, req.GetWallets())
	if err != nil {
		return nil, err
	}
	if rejection != nil {
		return &pb.ProvisionResponse{
			Id: req.GetId(),
			Result: &pb.ProvisionResponse_HolderProvisionRejected{
				HolderProvisionRejected: &pb.HolderProvisionRejected{Id: req.GetId(), Reason: rejection.GetReason()},
			},
		}, nil
	}

	// The Holder's own stream carries HolderEstablished plus one
	// HolderAddedWallet per Wallet being added, all at one ExpectedSeq —
	// AppendAtomic rejects naming the same stream twice.
	for _, w := range walletWrites {
		if w.AggregateType == aggregateType {
			holderEvents = append(holderEvents, w.Events...)
			continue
		}
		writes = append(writes, w)
	}
	if len(holderEvents) > 0 {
		writes = append(writes, eventstore.StreamWrite{
			AggregateType: aggregateType,
			AggregateID:   req.GetId(),
			ExpectedSeq:   int64(len(events)),
			Events:        holderEvents,
		})
	}

	if len(writes) > 0 {
		if err := s.appendConverging(ctx, writes); err != nil {
			return nil, err
		}
	}

	walletIDs := make([]string, 0, len(req.GetWallets()))
	for _, spec := range req.GetWallets() {
		walletIDs = append(walletIDs, spec.GetWalletId())
	}
	return &pb.ProvisionResponse{
		Id: req.GetId(),
		Result: &pb.ProvisionResponse_HolderProvisioned{
			HolderProvisioned: &pb.HolderProvisioned{Id: req.GetId(), WalletIds: walletIDs},
		},
	}, nil
}

// walletWrites computes the delta for a set of Wallets: the streams that still
// need writing, skipping Wallets already open and those already recorded
// against the Holder. Returns a rejection when a requested Wallet exists but
// belongs to a different Holder.
func (s *Server) walletWrites(
	ctx context.Context, holderID string, holderStream []eventstore.Event, specs []*pb.WalletSpec,
) ([]eventstore.StreamWrite, *pb.HolderRejectedWallet, error) {
	recorded, err := recordedWallets(holderID, holderStream)
	if err != nil {
		return nil, nil, err
	}

	var writes []eventstore.StreamWrite
	var holderEvents []proto.Message

	for _, spec := range specs {
		opened, err := wallet.Opened(ctx, s.store, spec.GetWalletId())
		if err != nil {
			return nil, nil, err
		}
		switch {
		case opened == nil:
			writes = append(writes, eventstore.StreamWrite{
				AggregateType: wallet.AggregateType,
				AggregateID:   spec.GetWalletId(),
				ExpectedSeq:   0,
				Events: []proto.Message{
					wallet.OpenedEvent(spec.GetWalletId(), holderID, spec.GetName(), spec.GetAllows()),
				},
			})
		case opened.GetHolderId() != holderID:
			return nil, &pb.HolderRejectedWallet{
				Id: holderID, WalletId: spec.GetWalletId(),
				Reason: fmt.Sprintf("wallet already belongs to holder %q", opened.GetHolderId()),
			}, nil
		}

		if _, already := recorded[spec.GetWalletId()]; !already {
			holderEvents = append(holderEvents, &pb.HolderAddedWallet{
				Id: holderID, WalletId: spec.GetWalletId(), Name: spec.GetName(), Allows: spec.GetAllows(),
			})
		}
	}

	if len(holderEvents) > 0 {
		writes = append(writes, eventstore.StreamWrite{
			AggregateType: aggregateType,
			AggregateID:   holderID,
			ExpectedSeq:   int64(len(holderStream)),
			Events:        holderEvents,
		})
	}
	return writes, nil, nil
}

// appendConverging treats a concurrency conflict as convergence rather than
// failure: another caller provisioning the same ids landed first, and the
// result they wrote is the one both callers wanted.
func (s *Server) appendConverging(ctx context.Context, writes []eventstore.StreamWrite) error {
	switch err := s.store.AppendAtomic(ctx, writes...); {
	case err == nil, errors.Is(err, eventstore.ErrConcurrencyConflict):
		return nil
	default:
		return twirp.InternalErrorWith(err)
	}
}

// recordedWallets is the set of wallet ids already added to the Holder, folded
// from its stream.
func recordedWallets(id string, events []eventstore.Event) (map[string]struct{}, error) {
	recorded := make(map[string]struct{})
	for _, event := range events {
		msg, err := event.Decode()
		if err != nil {
			return nil, twirp.InternalErrorWith(err)
		}
		if added, ok := msg.(*pb.HolderAddedWallet); ok {
			recorded[added.GetWalletId()] = struct{}{}
		}
	}
	return recorded, nil
}

// stream loads the Holder's events, mapping a store failure to a twirp error.
func (s *Server) stream(ctx context.Context, id string) ([]eventstore.Event, error) {
	events, err := s.store.Load(ctx, aggregateType, id)
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}
	return events, nil
}

// established returns the HolderEstablished event for id, or nil when the
// Holder does not exist yet.
func (s *Server) established(ctx context.Context, id string) (*pb.HolderEstablished, error) {
	events, err := s.stream(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, nil
	}
	return decodeEstablished(id, events)
}

// decodeEstablished reads the origin event off a non-empty Holder stream. A
// stream that does not open with HolderEstablished is corrupt or mis-sequenced,
// and must be refused rather than treated as a new aggregate.
func decodeEstablished(id string, events []eventstore.Event) (*pb.HolderEstablished, error) {
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
