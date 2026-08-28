package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	holderpb "github.com/namelessnotion/money_flow/go/gen/proto/holder/v1"
	sharedpb "github.com/namelessnotion/money_flow/go/gen/proto/shared/v1"
	walletpb "github.com/namelessnotion/money_flow/go/gen/proto/wallet/v1"
	"github.com/namelessnotion/money_flow/go/internal/eventstore"
	"github.com/namelessnotion/money_flow/go/internal/testutil"
)

type okPinger struct{}

func (okPinger) Ping(context.Context) error { return nil }

// Drives the real mux over real HTTP with the generated clients. Everything
// else in the suite calls the servers directly, so this is the only place the
// wire itself — routing, path prefixes, protobuf content negotiation — is
// exercised.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(newMux(eventstore.NewMemoryStore(), okPinger{}))
	t.Cleanup(srv.Close)
	return srv
}

func TestHolderServiceOverHTTP(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	client := holderpb.NewHolderServiceProtobufClient(srv.URL, srv.Client())

	resp, err := client.Provision(context.Background(), &holderpb.ProvisionRequest{
		Id: testutil.ID("h1"),
		Wallets: []*holderpb.WalletSpec{
			{WalletId: testutil.ID("w1"), Name: "bank", Allows: sharedpb.Allows_ALLOWS_ONRAMP_AND_OFFRAMP},
			{WalletId: testutil.ID("w2"), Name: "cash", Allows: sharedpb.Allows_ALLOWS_NONE},
		},
	})
	if err != nil {
		t.Fatalf("Provision() over HTTP error = %v", err)
	}
	if got := resp.GetHolderProvisioned().GetWalletIds(); len(got) != 2 {
		t.Errorf("WalletIds = %v, want 2", got)
	}
	// Enum values must survive the round trip; a mis-encoded enum would come
	// back as UNSPECIFIED without any transport error.
	if resp.GetId() != testutil.ID("h1") {
		t.Errorf("response Id = %q, want %s", resp.GetId(), testutil.ID("h1"))
	}
}

func TestWalletServiceOverHTTP(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	client := walletpb.NewWalletServiceProtobufClient(srv.URL, srv.Client())

	resp, err := client.Open(context.Background(), &walletpb.OpenRequest{
		Id: testutil.ID("w1"), HolderId: testutil.ID("h1"), Name: "bank", Allows: sharedpb.Allows_ALLOWS_OFFRAMP,
	})
	if err != nil {
		t.Fatalf("Open() over HTTP error = %v", err)
	}
	if got := resp.GetWalletOpened().GetAllows(); got != sharedpb.Allows_ALLOWS_OFFRAMP {
		t.Errorf("Allows round-tripped as %v, want ALLOWS_OFFRAMP", got)
	}
}

// The path prefix is the whole reason this test exists: a client pointed at the
// bare host rather than /twirp gets a 404, and nothing in either language's
// unit tests would catch it.
func TestServicesMountUnderTwirpPrefix(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	for _, tc := range []struct {
		name, path string
		want       int
	}{
		{"under the twirp prefix", "/twirp/holder.v1.HolderService/Establish", http.StatusBadRequest},
		{"without the prefix", "/holder.v1.HolderService/Establish", http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// An empty body is a malformed request, not a routing failure — so
			// 400 means "routed and rejected" while 404 means "never reached".
			resp, err := srv.Client().Post(srv.URL+tc.path, "application/protobuf", http.NoBody)
			if err != nil {
				t.Fatalf("POST %s error = %v", tc.path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("POST %s = %d, want %d", tc.path, resp.StatusCode, tc.want)
			}
		})
	}
}

func TestHealthz(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	resp, err := srv.Client().Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /healthz = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}
