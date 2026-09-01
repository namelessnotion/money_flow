package money

import (
	"errors"
	"testing"

	"github.com/twitchtv/twirp"

	sharedpb "github.com/namelessnotion/money_flow/go/gen/proto/shared/v1"
)

func TestValidateAcceptsAWellFormedMoney(t *testing.T) {
	t.Parallel()

	if err := Validate("amount", &sharedpb.Money{MinorUnits: 500, Currency: "USD"}); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func wantInvalidArgument(t *testing.T, err error, wantArgument string) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want an invalid-argument error")
	}
	var twerr twirp.Error
	if !errors.As(err, &twerr) {
		t.Fatalf("error = %T, want twirp.Error", err)
	}
	if twerr.Code() != twirp.InvalidArgument {
		t.Errorf("error code = %q, want %q", twerr.Code(), twirp.InvalidArgument)
	}
	if got := twerr.Meta("argument"); got != wantArgument {
		t.Errorf("error argument = %q, want %q", got, wantArgument)
	}
}

func TestValidateRejectsANilMoney(t *testing.T) {
	t.Parallel()
	wantInvalidArgument(t, Validate("capacity", nil), "capacity")
}

func TestValidateRejectsAnEmptyCurrency(t *testing.T) {
	t.Parallel()
	wantInvalidArgument(t, Validate("capacity", &sharedpb.Money{MinorUnits: 500}), "capacity.currency")
}

func TestValidateRejectsZeroMinorUnits(t *testing.T) {
	t.Parallel()
	wantInvalidArgument(t, Validate("amount", &sharedpb.Money{Currency: "USD"}), "amount.minor_units")
}

// The field name is the one thing every caller supplies differently
// (token's "capacity" vs. transfer/operation's "amount") — confirm it
// actually threads through to the error rather than being hardcoded.
func TestValidateNamesTheGivenFieldInEveryError(t *testing.T) {
	t.Parallel()

	wantInvalidArgument(t, Validate("wallet_capacity", nil), "wallet_capacity")
	wantInvalidArgument(t, Validate("leg_amount", &sharedpb.Money{MinorUnits: 1}), "leg_amount.currency")
	wantInvalidArgument(t, Validate("leg_amount", &sharedpb.Money{Currency: "USD"}), "leg_amount.minor_units")
}
