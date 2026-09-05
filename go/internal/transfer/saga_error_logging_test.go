package transfer

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	sharedpb "github.com/namelessnotion/money_flow/go/gen/proto/shared/v1"
	"github.com/namelessnotion/money_flow/go/internal/eventstore"
	"github.com/namelessnotion/money_flow/go/internal/ledger"
	"github.com/namelessnotion/money_flow/go/internal/testutil"
)

// failingAppendAtomicStore lets Append and Load through to the real store
// (so RequestTransfer's own accept step succeeds normally) but makes every
// AppendAtomic call fail with a plain, non-domain error — simulating the
// store becoming unavailable partway through prepare(), which is the first
// thing runSaga calls after accept.
type failingAppendAtomicStore struct {
	eventstore.Store
	err error
}

func (s *failingAppendAtomicStore) AppendAtomic(context.Context, ...eventstore.StreamWrite) error {
	return s.err
}

// A runSaga failure here is exactly the case this test guards: not a
// TigerBeetle rejection (those become a durable TransferFailed event and
// runSaga returns nil), but a genuine internal problem — the kind that used
// to vanish with `_ = s.runSaga(...)` and no logging at all.
func TestRequestTransfer_LogsSwallowedSagaErrors(t *testing.T) {
	store := eventstore.NewMemoryStore()
	lc := ledger.NewFakeClient()
	openWallet(t, store, testutil.ID("w1"), sharedpb.Allows_ALLOWS_ONRAMP_AND_OFFRAMP)
	openWallet(t, store, testutil.ID("w2"), sharedpb.Allows_ALLOWS_ONRAMP_AND_OFFRAMP)
	mintToken(t, store, lc, testutil.ID("w1"), testutil.ID("t1"), usd(1000))
	fundToken(t, lc, testutil.ID("t1"), 1000)

	failing := &failingAppendAtomicStore{Store: store, err: errors.New("boom: store unavailable")}
	server := NewServer(failing, lc, nil, nil)

	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(logDefaultOutput)

	resp, err := server.RequestTransfer(context.Background(),
		transferRequest(testutil.ID("xfer1"), testutil.ID("w1"), testutil.ID("w2"), usd(400), false))
	if err != nil {
		t.Fatalf("RequestTransfer() error = %v, want the Accepted response even though the saga failed (decision #7)", err)
	}
	if resp.GetTransferRequestAccepted() == nil {
		t.Fatalf("result = %v, want Accepted", resp.GetResult())
	}

	if got := logBuf.String(); !strings.Contains(got, "boom: store unavailable") {
		t.Errorf("log output = %q, want it to mention the swallowed saga error", got)
	}
	if got := logBuf.String(); !strings.Contains(got, testutil.ID("xfer1")) {
		t.Errorf("log output = %q, want it to name the stuck transfer id", got)
	}
}

// logDefaultOutput restores the stdlib logger to its normal destination
// after a test redirects it. Captured once at package init, before any test
// has a chance to redirect it.
var logDefaultOutput = log.Writer()
