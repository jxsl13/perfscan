package runner

import (
	"testing"

	"go/token"

	"github.com/jxsl13/perfscan/lint"
)

// TestPathExcluded covers the -exclude matching rule: the finding's path is
// slash-normalized (filepath.ToSlash) and excluded when it contains ANY of
// the exclude patterns as a plain SUBSTRING — no globbing, no anchoring. An
// empty exclude list never matches.
func TestPathExcluded(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		excludes []string
		want     bool
	}{
		{
			name:     "normal path not excluded",
			path:     "internal/runner/runner.go",
			excludes: []string{"vendor/", "testdata/"},
			want:     false,
		},
		{
			name:     "vendor path excluded",
			path:     "vendor/github.com/foo/bar.go",
			excludes: []string{"vendor/", "testdata/"},
			want:     true,
		},
		{
			name:     "nested vendor path excluded",
			path:     "cmd/tool/vendor/lib/lib.go",
			excludes: []string{"vendor/"},
			want:     true,
		},
		{
			name:     "generated protobuf suffix excluded",
			path:     "api/v1/service.pb.go",
			excludes: []string{".pb.go"},
			want:     true,
		},
		{
			// Substring semantics: "end" is not a path component or a full
			// name anywhere in the path, but "vendor/x.go" CONTAINS it —
			// a partial pattern still matches. Documented behavior.
			name:     "substring that is not a full component still matches",
			path:     "vendor/x.go",
			excludes: []string{"end"},
			want:     true,
		},
		{
			name:     "empty excludes never match",
			path:     "vendor/github.com/foo/bar.go",
			excludes: nil,
			want:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pathExcluded(tt.path, tt.excludes); got != tt.want {
				t.Errorf("pathExcluded(%q, %v) = %v, want %v", tt.path, tt.excludes, got, tt.want)
			}
		})
	}
}

// TestFilterExcluded pins the finding-level filter: findings under any
// excluded path prefix/substring are dropped, everything else is kept in
// the original order, and an empty exclude list returns everything.
func TestFilterExcluded(t *testing.T) {
	mk := func(path string) Finding {
		return Finding{
			Check: &lint.Check{ID: "PS2101"},
			Pos:   token.Position{Filename: path, Line: 1},
		}
	}
	vendorPath := "vendor/github.com/foo/bar.go"
	normalPath := "internal/runner/runner.go"
	generatedPath := "generated/api.go"
	in := []Finding{mk(vendorPath), mk(normalPath), mk(generatedPath)}

	files := func(fs []Finding) []string {
		out := make([]string, 0, len(fs))
		for i := range fs {
			out = append(out, fs[i].Pos.Filename)
		}
		return out
	}

	t.Run("drops matching, keeps normal, preserves order", func(t *testing.T) {
		got := filterExcluded(in, []string{"vendor/", "generated/"})
		want := []string{normalPath}
		if len(got) != len(want) {
			t.Fatalf("got %d findings %v, want %d %v", len(got), files(got), len(want), want)
		}
		for i, w := range want {
			if got[i].Pos.Filename != w {
				t.Errorf("finding %d: got %s, want %s (order must be preserved)", i, got[i].Pos.Filename, w)
			}
		}
	})

	t.Run("empty excludes returns everything", func(t *testing.T) {
		got := filterExcluded(in, nil)
		if len(got) != len(in) {
			t.Fatalf("empty excludes: got %d findings %v, want all %d", len(got), files(got), len(in))
		}
		for i := range in {
			if got[i].Pos.Filename != in[i].Pos.Filename {
				t.Errorf("finding %d: got %s, want %s (order must be preserved)", i, got[i].Pos.Filename, in[i].Pos.Filename)
			}
		}
	})
}
