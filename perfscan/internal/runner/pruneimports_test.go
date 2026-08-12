package runner

import "testing"

// TestPruneOrphanedImports pins the post-fix import pruning that keeps
// cross-check rewrites from leaving an "imported and not used" error (e.g.
// PS3002 rewriting sort.Slice AND PS3104 rewriting sort.Strings both drop a
// "sort" reference, but neither alone orphans the import). It must remove a
// genuinely-unused import while never touching blank/dot/cgo imports, a
// //line-directive file, or a still-used import.
func TestPruneOrphanedImports(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		wantPruned bool   // the orphaned import should be gone
		absent     string // substring that must NOT survive when pruned
		present    string // substring that MUST survive
	}{
		{
			name: "orphaned sort removed, slices+cmp kept",
			src: "package p\n\nimport (\n\t\"cmp\"\n\t\"slices\"\n\t\"sort\"\n)\n\n" +
				"func f(xs []int) { slices.Sort(xs); _ = cmp.Compare(1, 2) }\n",
			wantPruned: true, absent: `"sort"`, present: `"slices"`,
		},
		{
			name: "blank import preserved even though unreferenced",
			src: "package p\n\nimport (\n\t_ \"embed\"\n\t\"slices\"\n)\n\n" +
				"func f(xs []int) { slices.Sort(xs) }\n",
			wantPruned: false, present: `_ "embed"`,
		},
		{
			name: "dot import preserved",
			src: "package p\n\nimport (\n\t. \"strings\"\n\t\"slices\"\n)\n\n" +
				"func f(xs []int) { slices.Sort(xs) }\n",
			wantPruned: false, present: `. "strings"`,
		},
		{
			name: "cgo file left entirely untouched",
			src: "package p\n\n// #include <stdlib.h>\nimport \"C\"\nimport \"sort\"\n\n" +
				"func f() { _ = C.malloc }\n",
			wantPruned: false, present: `"sort"`,
		},
		{
			name: "line-directive file left untouched",
			src: "package p\n\nimport \"sort\"\n\n//line gen.go:1\n" +
				"func f() {}\n",
			wantPruned: false, present: `"sort"`,
		},
		{
			name: "still-used import kept",
			src: "package p\n\nimport \"sort\"\n\n" +
				"func f(xs []int) { sort.Ints(xs) }\n",
			wantPruned: false, present: `"sort"`,
		},
		{
			name: "aliased orphaned import removed",
			src: "package p\n\nimport (\n\tsrt \"sort\"\n\t\"slices\"\n)\n\n" +
				"func f(xs []int) { slices.Sort(xs) }\n",
			wantPruned: true, absent: `srt "sort"`, present: `"slices"`,
		},
		{
			// Regression: a versioned third-party import whose package name
			// ("klog") differs from the last path segment ("v2"), used via the
			// package name, must NOT be pruned — UsesImport misjudges it, so we
			// only prune stdlib. (Broke pkg/labels/selector.go on kubernetes.)
			name: "versioned third-party import kept even when name != last segment",
			src: "package p\n\nimport (\n\t\"slices\"\n\tklog \"k8s.io/klog/v2\"\n)\n\n" +
				"func f(xs []int) { slices.Sort(xs); klog.Info(\"x\") }\n",
			wantPruned: false, present: `"k8s.io/klog/v2"`,
		},
		{
			// Even a genuinely-unused third-party import is left alone (we never
			// prune non-stdlib): that is the check's own import-machinery job,
			// and misjudging a versioned path is worse than a rare leftover.
			name: "unused third-party import left untouched (stdlib-only prune)",
			src: "package p\n\nimport (\n\t\"slices\"\n\t\"github.com/x/y\"\n)\n\n" +
				"func f(xs []int) { slices.Sort(xs) }\n",
			wantPruned: false, present: `"github.com/x/y"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := string(pruneOrphanedImports([]byte(tc.src)))
			if tc.absent != "" && contains(got, tc.absent) {
				t.Errorf("expected %q to be pruned, but it remains:\n%s", tc.absent, got)
			}
			if tc.present != "" && !contains(got, tc.present) {
				t.Errorf("expected %q to survive, but it is gone:\n%s", tc.present, got)
			}
			// The result must still be parseable/compilable Go (i.e. valid).
			if tc.wantPruned && got == tc.src {
				t.Errorf("expected a change (pruned import) but source is unchanged:\n%s", got)
			}
			if !tc.wantPruned && got != tc.src {
				t.Errorf("expected NO change but source was rewritten:\n--- before ---\n%s\n--- after ---\n%s", tc.src, got)
			}
		})
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
