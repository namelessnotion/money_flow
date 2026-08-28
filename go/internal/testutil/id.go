// Package testutil holds small helpers shared by this module's test files.
package testutil

import (
	"crypto/sha256"
	"uuid"
)

// ID returns a deterministic, valid UUID derived from seed: the same seed
// always yields the same id. Holder/Wallet ids are now validated as UUIDs, but
// tests still want short, meaningful names like "h1" or "w-bank" rather than
// opaque random ones — ID(seed) keeps the seed visible in the test source
// while producing a value that satisfies validation.
func ID(seed string) string {
	sum := sha256.Sum256([]byte(seed))

	var u uuid.UUID
	copy(u[:], sum[:16])
	u[6] = (u[6] & 0x0f) | 0x40 // version 4
	u[8] = (u[8] & 0x3f) | 0x80 // RFC 9562 variant

	return u.String()
}
