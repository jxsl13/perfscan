package checks

import (
	"crypto/sha256"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"testing"
)

// These are complete byte-identical owner files, not hand-selected call shapes.
// This provenance test alone does not establish detector coverage or execute
// the owner's native backend. The analyzer replay must be tested separately.
func TestPS6136OwnerProvenance(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct{ path, digest string }{
		{"before/gpt.go.txt", "ad39b6e13d16557b749ecb9c1bbffe49eab6b0a90d33d3b8cd59f9d077bb35c7"},
		{"before/decoder.go.txt", "f13685b8af0cd797ea5b0c8d08983bcee90713f24b1b5fab5f2e8355ca7dbd17"},
		{"after/gpt.go.txt", "023ffa015e40f8405d9ddccff399a4c71e69dfda0818544f4c0d8691abaa4841"},
		{"after/decoder.go.txt", "b4a9355b118852e870a8b03444c99a7ef3f9bd8fb8b482efc0ebae35e88d1d16"},
		{"llamagpu.go.txt", "d404d5d843de7d9caabfe3e07cbbc40bca43723b1b55701217ae9a800bc468db"},
		{"cuda.go.txt", "8319c09049371d175d236fc2b6ca8d9336f88e9ae0db039de4afd3103d945d1c"},
		{"vulkan.go.txt", "610aa8af979ccdc779e05136cf01ee00f2eb6d99a1dde5b08c0239178cc419b5"},
		{"sampling_fastpath.go.txt", "e1f9e9d367ae6a1cca4d20be03f283622496b74e71def553eee7daba006c60e6"},
	} {
		t.Run(fixture.path, func(t *testing.T) {
			t.Parallel()
			path := "testdata/ps6136-owner/" + fixture.path
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != fixture.digest {
				t.Fatalf("owner source changed: %s", got)
			}
			if _, err := parser.ParseFile(token.NewFileSet(), path, data, parser.AllErrors); err != nil {
				t.Fatal(err)
			}
		})
	}
}
