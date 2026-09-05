// Package detid derives stable, deterministic, validly-formed UUIDs from an
// arbitrary seed — the same seed always yields the same id. Used wherever a
// retried operation needs to resubmit an identical id and converge via the
// receiving system's own idempotency, rather than generating a fresh one
// each attempt.
package detid

import (
	"crypto/sha256"
	"uuid"
)

// New derives a stable, valid UUID (v4-shaped) from seed.
func New(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	var u uuid.UUID
	copy(u[:], sum[:16])
	u[6] = (u[6] & 0x0f) | 0x40
	u[8] = (u[8] & 0x3f) | 0x80
	return u.String()
}
