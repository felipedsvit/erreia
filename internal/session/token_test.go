package session

import (
	"strings"
	"testing"
)

// TestNewTokenLength verifies the token is URL-safe and has enough entropy
// to be hard to guess.
func TestNewTokenLength(t *testing.T) {
	t.Parallel()
	tok, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) < 32 {
		t.Fatalf("token too short: %d", len(tok))
	}
	if strings.ContainsAny(tok, "+/=") {
		t.Fatalf("token uses non-URL-safe chars: %q", tok)
	}
}

// TestNewTokenUniqueness fuzzes the function to make sure collisions are
// vanishingly unlikely.
func TestNewTokenUniqueness(t *testing.T) {
	t.Parallel()
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		tok, err := NewToken()
		if err != nil {
			t.Fatal(err)
		}
		if _, dup := seen[tok]; dup {
			t.Fatalf("duplicate token after %d iterations: %q", i, tok)
		}
		seen[tok] = struct{}{}
	}
}
