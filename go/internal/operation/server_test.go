package operation

import (
	"context"
	"testing"

	pb "github.com/namelessnotion/money_flow/go/gen/proto/operation/v1"
	sharedpb "github.com/namelessnotion/money_flow/go/gen/proto/shared/v1"
	"github.com/namelessnotion/money_flow/go/internal/eventstore"
	"github.com/namelessnotion/money_flow/go/internal/testutil"
)

func amount() *sharedpb.Money { return &sharedpb.Money{MinorUnits: 500, Currency: "USD"} }

func debitRequest() *pb.InitiateRequest {
	return &pb.InitiateRequest{
		Id: testutil.ID("op1"), TransferId: testutil.ID("xfer1"),
		TokenId: testutil.ID("t-src"), CounterpartyTokenId: testutil.ID("t-dst"),
		Operator: pb.Operator_OPERATOR_DEBIT, Amount: amount(),
	}
}

func TestInitiatedEvent(t *testing.T) {
	t.Parallel()
	event := InitiatedEvent(testutil.ID("op1"), testutil.ID("xfer1"), testutil.ID("t-src"), testutil.ID("t-dst"), pb.Operator_OPERATOR_DEBIT, amount())
	if event.GetId() != testutil.ID("op1") || event.GetTransferId() != testutil.ID("xfer1") {
		t.Errorf("event = %+v", event)
	}
	if event.GetOperator() != pb.Operator_OPERATOR_DEBIT {
		t.Errorf("Operator = %v, want DEBIT", event.GetOperator())
	}
}

func TestInitiate_Succeeds(t *testing.T) {
	t.Parallel()
	store := eventstore.NewMemoryStore()
	server := NewServer(store)

	resp, err := server.Initiate(context.Background(), debitRequest())
	if err != nil {
		t.Fatalf("Initiate() error = %v", err)
	}
	initiated := resp.GetInitiated()
	if initiated == nil {
		t.Fatalf("result = %v, want Initiated", resp.GetResult())
	}
	if initiated.GetId() != testutil.ID("op1") {
		t.Errorf("Id = %q, want %q", initiated.GetId(), testutil.ID("op1"))
	}
}

func TestInitiate_IsIdempotent(t *testing.T) {
	t.Parallel()
	store := eventstore.NewMemoryStore()
	server := NewServer(store)
	ctx := context.Background()

	first, err := server.Initiate(ctx, debitRequest())
	if err != nil {
		t.Fatalf("first Initiate() error = %v", err)
	}
	second, err := server.Initiate(ctx, debitRequest())
	if err != nil {
		t.Fatalf("second Initiate() error = %v", err)
	}
	if first.GetInitiated().GetId() != second.GetInitiated().GetId() {
		t.Errorf("replay differs: %v vs %v", first, second)
	}

	events, err := store.Load(ctx, AggregateType, testutil.ID("op1"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(events) != 1 {
		t.Errorf("log holds %d events, want 1 (no duplicate Initiated)", len(events))
	}
}

func TestInitiate_ValidatesRequest(t *testing.T) {
	t.Parallel()
	store := eventstore.NewMemoryStore()
	server := NewServer(store)
	ctx := context.Background()

	base := debitRequest
	tests := []struct {
		name string
		req  *pb.InitiateRequest
	}{
		{"missing id", func() *pb.InitiateRequest { r := base(); r.Id = ""; return r }()},
		{"missing transfer_id", func() *pb.InitiateRequest { r := base(); r.TransferId = ""; return r }()},
		{"missing token_id", func() *pb.InitiateRequest { r := base(); r.TokenId = ""; return r }()},
		{"debit missing counterparty", func() *pb.InitiateRequest { r := base(); r.CounterpartyTokenId = ""; return r }()},
		{"missing operator", func() *pb.InitiateRequest { r := base(); r.Operator = pb.Operator_OPERATOR_UNSPECIFIED; return r }()},
		{"missing amount", func() *pb.InitiateRequest { r := base(); r.Amount = nil; return r }()},
		{"zero amount", func() *pb.InitiateRequest { r := base(); r.Amount = &sharedpb.Money{Currency: "USD"}; return r }()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := server.Initiate(ctx, tt.req); err == nil {
				t.Errorf("Initiate(%+v) error = nil, want a validation error", tt.req)
			}
		})
	}
}

func TestInitiate_CreditDoesNotRequireCounterparty(t *testing.T) {
	t.Parallel()
	store := eventstore.NewMemoryStore()
	server := NewServer(store)

	req := &pb.InitiateRequest{
		Id: testutil.ID("op-credit"), TransferId: testutil.ID("xfer1"),
		TokenId: testutil.ID("t-dst"), Operator: pb.Operator_OPERATOR_CREDIT, Amount: amount(),
	}
	if _, err := server.Initiate(context.Background(), req); err != nil {
		t.Fatalf("Initiate() error = %v, want CREDIT to be valid without counterparty_token_id", err)
	}
}

func TestServerCancel_Succeeds(t *testing.T) {
	t.Parallel()
	store := eventstore.NewMemoryStore()
	server := NewServer(store)
	ctx := context.Background()
	if _, err := server.Initiate(ctx, debitRequest()); err != nil {
		t.Fatalf("Initiate() error = %v", err)
	}

	resp, err := server.Cancel(ctx, &pb.CancelRequest{Id: testutil.ID("op1"), Reason: "user cancelled"})
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if resp.GetCancelled().GetReason() != "user cancelled" {
		t.Errorf("Cancelled.Reason = %q, want %q", resp.GetCancelled().GetReason(), "user cancelled")
	}
}

func TestStagePerformCancelFail_Lifecycle(t *testing.T) {
	t.Parallel()
	store := eventstore.NewMemoryStore()
	server := NewServer(store)
	ctx := context.Background()

	// Immediate path: Initiated -> Performed directly.
	if _, err := server.Initiate(ctx, debitRequest()); err != nil {
		t.Fatalf("Initiate() error = %v", err)
	}
	performed, err := Perform(ctx, store, testutil.ID("op1"))
	if err != nil {
		t.Fatalf("Perform() error = %v", err)
	}
	if performed.GetId() != testutil.ID("op1") {
		t.Errorf("Performed.Id = %q, want %q", performed.GetId(), testutil.ID("op1"))
	}
	// Idempotent retry.
	if _, err := Perform(ctx, store, testutil.ID("op1")); err != nil {
		t.Fatalf("Perform() retry error = %v", err)
	}
	// Contradiction: can't also Cancel something already Performed.
	if _, err := Cancel(ctx, store, testutil.ID("op1"), "too late"); err == nil {
		t.Error("Cancel() after Performed error = nil, want a contradiction error")
	}
}

func TestStage_ThenPerform(t *testing.T) {
	t.Parallel()
	store := eventstore.NewMemoryStore()
	server := NewServer(store)
	ctx := context.Background()

	op2 := &pb.InitiateRequest{
		Id: testutil.ID("op2"), TransferId: testutil.ID("xfer1"),
		TokenId: testutil.ID("t-src"), CounterpartyTokenId: testutil.ID("t-dst"),
		Operator: pb.Operator_OPERATOR_DEBIT, Amount: amount(),
	}
	if _, err := server.Initiate(ctx, op2); err != nil {
		t.Fatalf("Initiate() error = %v", err)
	}

	staged, err := Stage(ctx, store, testutil.ID("op2"))
	if err != nil {
		t.Fatalf("Stage() error = %v", err)
	}
	if staged.GetId() != testutil.ID("op2") {
		t.Errorf("Staged.Id = %q, want %q", staged.GetId(), testutil.ID("op2"))
	}
	// Idempotent retry.
	if _, err := Stage(ctx, store, testutil.ID("op2")); err != nil {
		t.Fatalf("Stage() retry error = %v", err)
	}

	if _, err := Perform(ctx, store, testutil.ID("op2")); err != nil {
		t.Fatalf("Perform() after Stage error = %v", err)
	}

	events, err := store.Load(ctx, AggregateType, testutil.ID("op2"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	wantTypes := []string{"operation.v1.Initiated", "operation.v1.Staged", "operation.v1.Performed"}
	if len(events) != len(wantTypes) {
		t.Fatalf("log holds %d events, want %d: %v", len(events), len(wantTypes), events)
	}
	for i, want := range wantTypes {
		if events[i].EventType != want {
			t.Errorf("events[%d].EventType = %q, want %q", i, events[i].EventType, want)
		}
	}
}

func TestStage_FromPendingCanBeVoidedViaCancel(t *testing.T) {
	t.Parallel()
	store := eventstore.NewMemoryStore()
	server := NewServer(store)
	ctx := context.Background()
	if _, err := server.Initiate(ctx, debitRequest()); err != nil {
		t.Fatalf("Initiate() error = %v", err)
	}
	if _, err := Stage(ctx, store, testutil.ID("op1")); err != nil {
		t.Fatalf("Stage() error = %v", err)
	}

	cancelled, err := Cancel(ctx, store, testutil.ID("op1"), "provider returned it")
	if err != nil {
		t.Fatalf("Cancel() after Stage error = %v", err)
	}
	if cancelled.GetReason() != "provider returned it" {
		t.Errorf("Cancelled.Reason = %q, want %q", cancelled.GetReason(), "provider returned it")
	}
}

func TestFail_FromInitiated(t *testing.T) {
	t.Parallel()
	store := eventstore.NewMemoryStore()
	server := NewServer(store)
	ctx := context.Background()
	if _, err := server.Initiate(ctx, debitRequest()); err != nil {
		t.Fatalf("Initiate() error = %v", err)
	}

	failed, err := Fail(ctx, store, testutil.ID("op1"), "tigerbeetle rejected the batch")
	if err != nil {
		t.Fatalf("Fail() error = %v", err)
	}
	if failed.GetReason() != "tigerbeetle rejected the batch" {
		t.Errorf("Failed.Reason = %q, want %q", failed.GetReason(), "tigerbeetle rejected the batch")
	}
	// Idempotent retry, ignoring a changed reason on replay.
	replay, err := Fail(ctx, store, testutil.ID("op1"), "different reason")
	if err != nil {
		t.Fatalf("Fail() retry error = %v", err)
	}
	if replay.GetReason() != "tigerbeetle rejected the batch" {
		t.Errorf("replay Reason = %q, want the original reason preserved", replay.GetReason())
	}
}

func TestOperationLifecycleFunctions_RequireInitiatedFirst(t *testing.T) {
	t.Parallel()
	store := eventstore.NewMemoryStore()
	ctx := context.Background()

	if _, err := Perform(ctx, store, testutil.ID("never-initiated")); err == nil {
		t.Error("Perform() on an uninitiated operation error = nil, want an error")
	}
}
