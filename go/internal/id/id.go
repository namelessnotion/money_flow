// Package id validates the ids Holder and Wallet commands carry across the
// Twirp boundary. Every id in this system is a UUID by convention — Ruby
// generates them with SecureRandom.uuid_v7 before ever calling in — but
// nothing enforced that until now. Validating here, once, keeps the check out
// of holder and wallet's command-handling logic and out of the generic event
// store, which has no business knowing what a valid aggregate id looks like
// for any particular aggregate type.
package id

import (
	"uuid"

	"github.com/twitchtv/twirp"
)

// Validate reports whether value is a well-formed, non-empty UUID in its
// canonical string form for the named field, using the same required/invalid
// distinction Twirp already makes for other arguments.
//
// uuid.Parse alone isn't enough: it also accepts bare 32-hex, {braced}, and
// urn:uuid: forms, and is case-insensitive. Accepting those would let two
// textually different strings denote the same UUID, and anything downstream
// that compares ids by raw string equality — map keys, ownership checks, the
// in-memory event store — would wrongly treat them as different ids. Requiring
// the canonical form value.String() produces rules that out.
func Validate(field, value string) error {
	if value == "" {
		return twirp.RequiredArgumentError(field)
	}
	parsed, err := uuid.Parse(value)
	if err != nil || parsed.String() != value {
		return twirp.InvalidArgumentError(field, "must be a valid UUID")
	}
	return nil
}
