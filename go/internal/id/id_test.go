package id

import (
	"errors"
	"testing"

	"github.com/twitchtv/twirp"
)

func TestValidateAcceptsAWellFormedUUID(t *testing.T) {
	t.Parallel()

	if err := Validate("id", "0198c2e4-7c1a-7c1a-8c1a-0198c2e47c1a"); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidateRejectsAnEmptyValue(t *testing.T) {
	t.Parallel()

	err := Validate("wallet_id", "")
	if err == nil {
		t.Fatal("Validate() error = nil, want a required-argument error")
	}

	var twerr twirp.Error
	if !errors.As(err, &twerr) {
		t.Fatalf("Validate() error = %T, want twirp.Error", err)
	}
	if twerr.Code() != twirp.InvalidArgument {
		t.Errorf("error code = %q, want %q", twerr.Code(), twirp.InvalidArgument)
	}
	if got := twerr.Meta("argument"); got != "wallet_id" {
		t.Errorf("error argument = %q, want %q", got, "wallet_id")
	}
}

// A non-empty value that isn't UUID-shaped must be reported as a validation
// failure of that specific value, not confused with a merely-absent one.
func TestValidateRejectsAMalformedValue(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"not-a-uuid", "h1", "12345", "0198c2e4-7c1a-7c1a-8c1a"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			err := Validate("id", value)
			if err == nil {
				t.Fatalf("Validate(%q) error = nil, want an invalid-argument error", value)
			}

			var twerr twirp.Error
			if !errors.As(err, &twerr) {
				t.Fatalf("Validate(%q) error = %T, want twirp.Error", value, err)
			}
			if twerr.Code() != twirp.InvalidArgument {
				t.Errorf("error code = %q, want %q", twerr.Code(), twirp.InvalidArgument)
			}
			if got := twerr.Meta("argument"); got != "id" {
				t.Errorf("error argument = %q, want %q", got, "id")
			}
		})
	}
}

// uuid.Parse accepts several textual forms of the same UUID: bare 32-hex,
// {braces}, and urn:uuid: prefixed, on top of the canonical dashed form.
// Validate must accept only the canonical form — otherwise two semantically
// identical ids that differ only in surface syntax would validate as two
// different-looking strings, and anything downstream that compares ids by
// raw string equality (map keys, ownership checks, the in-memory event
// store) would treat them as different ids.
func TestValidateRejectsNonCanonicalUUIDForms(t *testing.T) {
	t.Parallel()

	for _, value := range []string{
		"0198c2e47c1a7c1a8c1a0198c2e47c1a",              // bare 32-hex, no dashes
		"{0198c2e4-7c1a-7c1a-8c1a-0198c2e47c1a}",        // braces
		"urn:uuid:0198c2e4-7c1a-7c1a-8c1a-0198c2e47c1a", // urn form
		"0198C2E4-7C1A-7C1A-8C1A-0198C2E47C1A",          // uppercase
	} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			err := Validate("id", value)
			if err == nil {
				t.Fatalf("Validate(%q) error = nil, want an invalid-argument error", value)
			}

			var twerr twirp.Error
			if !errors.As(err, &twerr) {
				t.Fatalf("Validate(%q) error = %T, want twirp.Error", value, err)
			}
			if twerr.Code() != twirp.InvalidArgument {
				t.Errorf("error code = %q, want %q", twerr.Code(), twirp.InvalidArgument)
			}
			if got := twerr.Meta("argument"); got != "id" {
				t.Errorf("error argument = %q, want %q", got, "id")
			}
		})
	}
}
