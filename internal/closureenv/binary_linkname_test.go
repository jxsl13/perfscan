package closureenv

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Preserve the previous scanner as an equivalence oracle, including its
// conservative whitespace behavior rather than interpreting new Go syntax.
func binaryGoReferencesTargetReference(data []byte, target string) bool {
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "//go:linkname" && strings.HasPrefix(fields[2], target+".") {
			return true
		}
	}
	return false
}

func TestBinaryLinknameScreenPreservesTokens(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, source string
		want         bool
	}{
		{"empty", "", false},
		{"ordinary", "package p\nfunc Value() int { return 1 }\n", false},
		{"target", "//go:linkname local example.test/p.Target\n", true},
		{"tabs", "\t//go:linkname\tlocal\texample.test/p.Target\r\n", true},
		{"unicodeWhitespace", "\u00a0//go:linkname\u2003local\u0085example.test/p.Target", true},
		{"extraFields", "//go:linkname local example.test/p.Target extra", true},
		{"otherPackage", "//go:linkname local example.test/other.Target", false},
		{"packagePrefix", "//go:linkname local example.test/plus.Target", false},
		{"twoFields", "//go:linkname example.test/p.Target", false},
		{"wrongDirective", "//go:linknames local example.test/p.Target", false},
		{"notFirstToken", "var x = 1 //go:linkname local example.test/p.Target", false},
		{"quoted", "var x = `//go:linkname local example.test/p.Target`", false},
		{"lateTarget", strings.Repeat("var ordinary = 1\n", 4096) + "//go:linkname local example.test/p.Target", true},
		{"invalidUTF8", "\xff //go:linkname local example.test/p.Target", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data := []byte(tc.source)
			got := binaryGoReferencesTarget(data, "example.test/p")
			if reference := binaryGoReferencesTargetReference(data, "example.test/p"); got != reference || got != tc.want {
				t.Fatalf("screen=%v reference=%v want=%v", got, reference, tc.want)
			}
		})
	}
}

func TestBinaryLinknameScreenStillHashesOrdinarySource(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "ordinary.go")
	if err := os.WriteFile(path, []byte("package p\nvar Value = 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	inventory, err := json.Marshal(listedBinaryPackage{ImportPath: "example.test/p", Dir: dir, GoFiles: []string{"ordinary.go"}})
	if err != nil {
		t.Fatal(err)
	}
	before, err := binaryMaterialDigest(inventory, "example.test/p")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := binaryMaterialDigest(inventory)
	if err != nil || plain != before {
		t.Fatalf("directive screening changed the material inventory: %v", err)
	}
	if err := os.WriteFile(path, []byte("package p\nvar Value = 2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	after, err := binaryMaterialDigest(inventory, "example.test/p")
	if err != nil || after == before {
		t.Fatalf("ordinary source change was omitted from material hash: %v", err)
	}
}

func BenchmarkBinaryLinknameScreen(b *testing.B) {
	ordinary := strings.Repeat("func value() int { return 123 }\n", 4096)
	for _, input := range []struct{ name, source string }{
		{"no-directives", ordinary},
		{"unrelated-directive", ordinary + "//go:linkname local unrelated/pkg.Target\n"},
	} {
		for _, implementation := range []struct {
			name string
			run  func([]byte, string) bool
		}{
			{"reference", binaryGoReferencesTargetReference},
			{"screened", binaryGoReferencesTarget},
		} {
			b.Run(input.name+"/"+implementation.name, func(b *testing.B) {
				data := []byte(input.source)
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					if implementation.run(data, "example.test/p") {
						b.Fatal("ordinary source unexpectedly references target")
					}
				}
			})
		}
	}
}
