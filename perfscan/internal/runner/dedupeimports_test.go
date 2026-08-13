package runner

import "testing"

// TestDedupeImports pins the post-fix collapse of exact-duplicate import specs
// that two different checks can each add (e.g. PS3104 + PS3105 both add
// "slices"), which would otherwise be a "redeclared in this block" error. It
// must keep exactly one, preserve intentionally-different aliases of one path,
// leave blank imports (legal duplicates) alone, and not touch cgo/line files.
func TestDedupeImports(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		changed bool
		// countOf: substring that must appear exactly `want` times after dedupe.
		countOf string
		want    int
		present string
	}{
		{
			name: "two identical unnamed imports collapse to one",
			src: "package p\n\nimport \"slices\"\nimport \"slices\"\n\n" +
				"func f(x []int) { slices.Sort(x) }\n",
			changed: true, countOf: `"slices"`, want: 1,
		},
		{
			name: "duplicate inside and outside a group collapses",
			src: "package p\n\nimport (\n\t\"fmt\"\n\t\"slices\"\n)\nimport \"slices\"\n\n" +
				"func f(x []int) { slices.Sort(x); _ = fmt.Sprint(x) }\n",
			changed: true, countOf: `"slices"`, want: 1,
		},
		{
			name: "intentionally different aliases of one path are preserved",
			src: "package p\n\nimport (\n\ta \"errors\"\n\tb \"errors\"\n)\n\n" +
				"var _ = a.New\nvar _ = b.New\n",
			changed: false, present: `a "errors"`,
		},
		{
			name: "duplicate blank imports left alone (legal)",
			src: "package p\n\nimport (\n\t_ \"embed\"\n\t_ \"embed\"\n)\n\n" +
				"func f() {}\n",
			changed: false, present: `_ "embed"`,
		},
		{
			name: "no duplicates: unchanged",
			src: "package p\n\nimport (\n\t\"fmt\"\n\t\"slices\"\n)\n\n" +
				"func f(x []int) { slices.Sort(x); _ = fmt.Sprint(x) }\n",
			changed: false,
		},
		{
			name: "cgo file left untouched even with a duplicate",
			src: "package p\n\n// #include <stdlib.h>\nimport \"C\"\nimport \"slices\"\nimport \"slices\"\n\n" +
				"func f(x []int) { _ = C.malloc; slices.Sort(x) }\n",
			changed: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := string(dedupeImports([]byte(tc.src)))
			if tc.changed && got == tc.src {
				t.Errorf("expected a change (dedupe) but source unchanged:\n%s", got)
			}
			if !tc.changed && got != tc.src {
				t.Errorf("expected NO change but got:\n--- before ---\n%s\n--- after ---\n%s", tc.src, got)
			}
			if tc.countOf != "" {
				if n := countSub(got, tc.countOf); n != tc.want {
					t.Errorf("%q appears %d times, want %d:\n%s", tc.countOf, n, tc.want, got)
				}
			}
			if tc.present != "" && !containsSub(got, tc.present) {
				t.Errorf("expected %q preserved:\n%s", tc.present, got)
			}
		})
	}
}

func countSub(s, sub string) int {
	n := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			n++
		}
	}
	return n
}

func containsSub(s, sub string) bool { return countSub(s, sub) > 0 }
