package detid

import (
	"testing"
	"uuid"
)

func TestNewIsDeterministic(t *testing.T) {
	t.Parallel()

	first := New("seed-1")
	second := New("seed-1")
	if first != second {
		t.Errorf("New(%q) = %q then %q, want the same value every time", "seed-1", first, second)
	}
	if New("seed-1") == New("seed-2") {
		t.Error(`New("seed-1") == New("seed-2"), want distinct seeds to produce distinct ids`)
	}
}

func TestNewIsAValidUUID(t *testing.T) {
	t.Parallel()

	if _, err := uuid.Parse(New("seed-1")); err != nil {
		t.Errorf("uuid.Parse(New(%q)) error = %v, want a well-formed UUID", "seed-1", err)
	}
}
