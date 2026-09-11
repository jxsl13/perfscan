package checks

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"
)

// Windows checkouts must preserve the pinned LF bytes: the owner derivation
// tests intentionally require exact multiline matches, not normalized snippets.
func TestPS6127PinnedOwnerFixtureBytes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, digest string
	}{
		{"ps6127_owner_baseline.go.txt", "3c54a75cc70f8bdc91d28bdc5c87ee52861e8ee67ed11797fc531e1b9546ff6c"},
		{"ps6127_owner_behavior_test.go.txt", "0dd6a9c84a095deb2c46b39310add1b97222251129776b2f64b36326bdec17c1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source, err := os.ReadFile("testdata/" + tc.name)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(source, []byte("\r")) {
				t.Fatal("pinned owner fixture contains CR bytes; preserve LF checkout attributes")
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != tc.digest {
				t.Fatalf("pinned owner fixture digest=%s, want %s", got, tc.digest)
			}
		})
	}
}
