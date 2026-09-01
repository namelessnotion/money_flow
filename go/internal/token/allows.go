// Package token implements the Token root aggregate: minting a fixed-
// capacity Token into a Wallet, backed 1:1 by a TigerBeetle account.
// Balance-affecting activity lives on Operation's event stream instead —
// see the operation package — so Token only ever owns its own mint intent.
package token

import (
	"fmt"

	sharedpb "github.com/namelessnotion/money_flow/go/gen/proto/shared/v1"
	"github.com/namelessnotion/money_flow/go/internal/ledger"
)

// AllowsToAccountFlags maps a Wallet's Allows policy onto the TigerBeetle
// account flags a Token minted into that Wallet gets at creation time:
//
//   - ALLOWS_ONRAMP_AND_OFFRAMP: no flag — the Wallet can move money freely
//     in either direction, so its Tokens can too.
//   - ALLOWS_ONRAMP (e.g. a Debit Card wallet): credits_must_not_exceed_debits
//     — money can only enter the platform through it.
//   - ALLOWS_OFFRAMP (e.g. an ACH BankAccount wallet): debits_must_not_exceed_credits
//     — money can only leave the platform through it, and the account is
//     allowed to go negative, which is what lets a reversal legally pull
//     money back out of it later.
//   - ALLOWS_NONE: also debits_must_not_exceed_credits — neither direction
//     is permitted, so the account must never go positive beyond what was
//     explicitly credited into it.
func AllowsToAccountFlags(allows sharedpb.Allows) (ledger.AccountFlags, error) {
	switch allows {
	case sharedpb.Allows_ALLOWS_ONRAMP_AND_OFFRAMP:
		return ledger.AccountFlags{}, nil
	case sharedpb.Allows_ALLOWS_ONRAMP:
		return ledger.AccountFlags{CreditsMustNotExceedDebits: true}, nil
	case sharedpb.Allows_ALLOWS_OFFRAMP, sharedpb.Allows_ALLOWS_NONE:
		return ledger.AccountFlags{DebitsMustNotExceedCredits: true}, nil
	default:
		return ledger.AccountFlags{}, fmt.Errorf("token: wallet allows %v is not set", allows)
	}
}
