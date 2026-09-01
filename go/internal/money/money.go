// Package money validates the shared.v1.Money values every command in this
// system carries. The same three checks — present, currency set, minor_units
// nonzero — appeared identically in token.validateCapacity,
// transfer.validateAmount, and inline in operation.validateInitiate, with
// only the field name differing between them. This is the one place that
// rule lives now, mirroring why internal/id exists for UUID validation.
package money

import (
	"github.com/twitchtv/twirp"

	sharedpb "github.com/namelessnotion/money_flow/go/gen/proto/shared/v1"
)

// Validate reports whether m is a well-formed Money value for the named
// field: present, with a currency set and a minor_units greater than zero.
func Validate(field string, m *sharedpb.Money) error {
	if m == nil {
		return twirp.RequiredArgumentError(field)
	}
	if m.GetCurrency() == "" {
		return twirp.RequiredArgumentError(field + ".currency")
	}
	if m.GetMinorUnits() == 0 {
		return twirp.InvalidArgumentError(field+".minor_units", "must be greater than zero")
	}
	return nil
}
