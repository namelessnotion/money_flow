// Package operation implements the Operation root aggregate: one leg of a
// Transfer. Its balance-affecting activity is recorded here; Token's own
// stream only ever carries mint intent — see the token package.
package operation

import (
	"context"
	"errors"
	"fmt"

	"github.com/twitchtv/twirp"
	"google.golang.org/protobuf/proto"

	pb "github.com/namelessnotion/money_flow/go/gen/proto/operation/v1"
	sharedpb "github.com/namelessnotion/money_flow/go/gen/proto/shared/v1"
	"github.com/namelessnotion/money_flow/go/internal/eventstore"
	"github.com/namelessnotion/money_flow/go/internal/id"
	"github.com/namelessnotion/money_flow/go/internal/money"
)

// AggregateType is this aggregate's stream namespace in the event log.
// Exported, like token's, because transfer's saga names Operation streams
// directly when initiating a Transfer's legs and driving them through
// Stage/Perform/Cancel/Fail.
const AggregateType = "operation"

var _ pb.OperationService = (*Server)(nil)

// Server implements the Operation Twirp service. No ledger.Client: the
// proto is explicit that all of a Transfer's DEBIT Operations submit
// together as one linked TigerBeetle chain, so there is no meaningful
// per-Operation TigerBeetle call — that batch call lives in
// transfer/saga.go instead.
type Server struct {
	store eventstore.Store
}

func NewServer(store eventstore.Store) *Server {
	return &Server{store: store}
}

// InitiatedEvent builds the event Initiate records. Pure — no I/O — so
// transfer's saga can build every Operation's StreamWrite without a Server,
// alongside TransferRequestAccepted and any new TokenMinted events, in one
// AppendAtomic.
func InitiatedEvent(id, transferID, tokenID, counterpartyTokenID string, op pb.Operator, amount *sharedpb.Money) *pb.Initiated {
	return &pb.Initiated{
		Id: id, TransferId: transferID, TokenId: tokenID, CounterpartyTokenId: counterpartyTokenID,
		Operator: op, Amount: amount,
	}
}

// Initiated returns the Initiated event for id, or nil when the Operation
// has not been initiated yet.
func Initiated(ctx context.Context, store eventstore.Store, operationID string) (*pb.Initiated, error) {
	events, err := store.Load(ctx, AggregateType, operationID)
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
	initiated, ok := msg.(*pb.Initiated)
	if !ok {
		return nil, twirp.InternalError(fmt.Sprintf(
			"operation %q: stream starts with %s, want Initiated", operationID, events[0].EventType,
		))
	}
	return initiated, nil
}

func validateInitiate(req *pb.InitiateRequest) error {
	if err := id.Validate("id", req.GetId()); err != nil {
		return err
	}
	if err := id.Validate("transfer_id", req.GetTransferId()); err != nil {
		return err
	}
	if err := id.Validate("token_id", req.GetTokenId()); err != nil {
		return err
	}
	if req.GetOperator() == pb.Operator_OPERATOR_UNSPECIFIED {
		return twirp.InvalidArgumentError("operator", "must be set")
	}
	// Per operation.proto: counterparty_token_id is the DEBIT leg's credit
	// account (the Transfer's destination Token) and is unused for CREDIT.
	if req.GetOperator() == pb.Operator_OPERATOR_DEBIT {
		if err := id.Validate("counterparty_token_id", req.GetCounterpartyTokenId()); err != nil {
			return err
		}
	}
	return money.Validate("amount", req.GetAmount())
}

// Initiate records Initiated for req.Id — one leg of a Transfer.
//
// Idempotent, like Holder/Wallet/Token: an Operation that already exists is
// returned as-is.
func (s *Server) Initiate(ctx context.Context, req *pb.InitiateRequest) (*pb.InitiateResponse, error) {
	if err := validateInitiate(req); err != nil {
		return nil, err
	}

	existing, err := Initiated(ctx, s.store, req.GetId())
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return initiatedResponse(existing), nil
	}

	event := InitiatedEvent(req.GetId(), req.GetTransferId(), req.GetTokenId(), req.GetCounterpartyTokenId(), req.GetOperator(), req.GetAmount())
	switch err := s.store.Append(ctx, AggregateType, req.GetId(), 0, event); {
	case err == nil:
		return initiatedResponse(event), nil

	case errors.Is(err, eventstore.ErrConcurrencyConflict):
		existing, err := Initiated(ctx, s.store, req.GetId())
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, twirp.InternalError("operation: append conflicted but no operation was found")
		}
		return initiatedResponse(existing), nil

	default:
		return nil, twirp.InternalErrorWith(err)
	}
}

func initiatedResponse(e *pb.Initiated) *pb.InitiateResponse {
	return &pb.InitiateResponse{Id: e.GetId(), Result: &pb.InitiateResponse_Initiated{Initiated: e}}
}

// Cancel records Cancelled for req.Id: an *external* factor stopped this
// Operation — the caller decided not to proceed with a not-yet-committed
// Transfer, or ruby reported a staged Transfer's settlement was cancelled.
// Distinct from Fail, which is for our own ledger's invariant rejecting a
// submission — see decision #13 in the Transfer plan.
func (s *Server) Cancel(ctx context.Context, req *pb.CancelRequest) (*pb.CancelResponse, error) {
	if err := id.Validate("id", req.GetId()); err != nil {
		return nil, err
	}
	cancelled, err := Cancel(ctx, s.store, req.GetId(), req.GetReason())
	if err != nil {
		return nil, err
	}
	return &pb.CancelResponse{Id: cancelled.GetId(), Result: &pb.CancelResponse_Cancelled{Cancelled: cancelled}}, nil
}

// Stage records Staged for operationID once its leg has been submitted to
// TigerBeetle as flags.pending but not yet posted or voided. Saga-internal
// — not exposed as its own RPC, only ever called by transfer/saga.go.
func Stage(ctx context.Context, store eventstore.Store, operationID string) (*pb.Staged, error) {
	msg, err := advance(ctx, store, operationID, &pb.Staged{Id: operationID})
	if err != nil {
		return nil, err
	}
	return msg.(*pb.Staged), nil
}

// Perform records Performed for operationID: for DEBIT, once its TigerBeetle
// transfer (or, for a staged leg, its post_pending_transfer) has committed;
// for CREDIT, once every DEBIT Operation on the Transfer has performed.
// Saga-internal, like Stage.
func Perform(ctx context.Context, store eventstore.Store, operationID string) (*pb.Performed, error) {
	msg, err := advance(ctx, store, operationID, &pb.Performed{Id: operationID})
	if err != nil {
		return nil, err
	}
	return msg.(*pb.Performed), nil
}

// Cancel records Cancelled for operationID directly on the store, without an
// RPC round trip — the package function transfer's saga calls for every leg
// of a Transfer being abandoned (before commit) or a staged settlement
// being reported cancelled (an external factor, per decision #13).
func Cancel(ctx context.Context, store eventstore.Store, operationID, reason string) (*pb.Cancelled, error) {
	msg, err := advance(ctx, store, operationID, &pb.Cancelled{Id: operationID, Reason: reason})
	if err != nil {
		return nil, err
	}
	return msg.(*pb.Cancelled), nil
}

// Fail records Failed for operationID: our own ledger rejected a submission
// we made (at stage or commit time) — an internal invariant violation, not
// an external factor. Distinct from Cancel; see decision #13.
func Fail(ctx context.Context, store eventstore.Store, operationID, reason string) (*pb.Failed, error) {
	msg, err := advance(ctx, store, operationID, &pb.Failed{Id: operationID, Reason: reason})
	if err != nil {
		return nil, err
	}
	return msg.(*pb.Failed), nil
}

// advance appends event as the next fact in operationID's stream, unless
// the stream's last event is already this same fact — idempotent
// convergence, the same idiom Holder/Wallet/Token use for their own
// creation-time replays — or already a *different* terminal fact
// (Performed, Cancelled, or Failed), which is a saga contradiction and a
// bug rather than a race, and is refused rather than silently overwritten.
func advance(ctx context.Context, store eventstore.Store, operationID string, event proto.Message) (proto.Message, error) {
	events, err := store.Load(ctx, AggregateType, operationID)
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}
	if len(events) == 0 {
		return nil, twirp.NewError(twirp.FailedPrecondition,
			fmt.Sprintf("operation %q has not been initiated", operationID))
	}

	wantType := eventstore.EventType(event)
	last := events[len(events)-1]
	if last.EventType == wantType {
		return last.Decode()
	}
	if isTerminalType(last.EventType) {
		return nil, twirp.InternalError(fmt.Sprintf(
			"operation %q: already %s, cannot also become %s", operationID, last.EventType, wantType))
	}

	switch err := store.Append(ctx, AggregateType, operationID, int64(len(events)), event); {
	case err == nil:
		return event, nil
	case errors.Is(err, eventstore.ErrConcurrencyConflict):
		// A sibling call (a retry of this exact step, run concurrently)
		// landed first — reload and converge rather than surfacing the race.
		return advance(ctx, store, operationID, event)
	default:
		return nil, twirp.InternalErrorWith(err)
	}
}

func isTerminalType(eventType string) bool {
	switch eventType {
	case eventstore.EventType(&pb.Performed{}), eventstore.EventType(&pb.Cancelled{}), eventstore.EventType(&pb.Failed{}):
		return true
	default:
		return false
	}
}
