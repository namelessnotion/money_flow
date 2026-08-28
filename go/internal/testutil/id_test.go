package testutil

import (
	"testing"
	"uuid"
)

func TestIDIsDeterministic(t *testing.T) {
	t.Parallel()

	first := ID("h1")
	second := ID("h1")
	if first != second {
		t.Errorf("ID(%q) = %q then %q, want the same value every time", "h1", first, second)
	}
	if ID("h1") == ID("h2") {
		t.Error("ID(\"h1\") == ID(\"h2\"), want distinct seeds to produce distinct ids")
	}
}

func TestIDIsAValidUUID(t *testing.T) {
	t.Parallel()

	if _, err := uuid.Parse(ID("h1")); err != nil {
		t.Errorf("uuid.Parse(ID(\"h1\")) error = %v, want a well-formed UUID", err)
	}
}
