package token

import (
	"testing"

	sharedpb "github.com/namelessnotion/money_flow/go/gen/proto/shared/v1"
	"github.com/namelessnotion/money_flow/go/internal/ledger"
)

func TestAllowsToAccountFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		allows sharedpb.Allows
		want   ledger.AccountFlags
	}{
		{
			name:   "onramp and offramp gets no flag",
			allows: sharedpb.Allows_ALLOWS_ONRAMP_AND_OFFRAMP,
			want:   ledger.AccountFlags{},
		},
		{
			name:   "onramp only (e.g. Debit Card) gets credits_must_not_exceed_debits",
			allows: sharedpb.Allows_ALLOWS_ONRAMP,
			want:   ledger.AccountFlags{CreditsMustNotExceedDebits: true},
		},
		{
			name:   "offramp only (e.g. ACH BankAccount) gets debits_must_not_exceed_credits",
			allows: sharedpb.Allows_ALLOWS_OFFRAMP,
			want:   ledger.AccountFlags{DebitsMustNotExceedCredits: true},
		},
		{
			name:   "neither direction also gets debits_must_not_exceed_credits",
			allows: sharedpb.Allows_ALLOWS_NONE,
			want:   ledger.AccountFlags{DebitsMustNotExceedCredits: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := AllowsToAccountFlags(tt.allows)
			if err != nil {
				t.Fatalf("AllowsToAccountFlags(%v) error = %v", tt.allows, err)
			}
			if got != tt.want {
				t.Errorf("AllowsToAccountFlags(%v) = %+v, want %+v", tt.allows, got, tt.want)
			}
		})
	}
}

func TestAllowsToAccountFlags_UnspecifiedIsAnError(t *testing.T) {
	t.Parallel()

	if _, err := AllowsToAccountFlags(sharedpb.Allows_ALLOWS_UNSPECIFIED); err == nil {
		t.Error("AllowsToAccountFlags(ALLOWS_UNSPECIFIED) error = nil, want an error")
	}
}
