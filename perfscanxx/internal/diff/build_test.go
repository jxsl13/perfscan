package diff

import (
	"errors"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/perfscanxx/internal/catalog"
	"github.com/jxsl13/perfscan/perfscanxx/internal/fixes"
)

// memFS is an in-memory FS for exercising the snapshot/fix/restore flow with no
// disk. writes records every WriteFile call so tests can assert the restore
// happened.
type memFS struct {
	files  map[string]string
	writes []string
}

func newMemFS(files map[string]string) *memFS {
	m := map[string]string{}
	for k, v := range files {
		m[k] = v
	}
	return &memFS{files: m}
}

func (m *memFS) ReadFile(path string) ([]byte, error) {
	s, ok := m.files[path]
	if !ok {
		return nil, errors.New("no such file: " + path)
	}
	return []byte(s), nil
}

func (m *memFS) WriteFile(path string, data []byte) error {
	m.files[path] = string(data)
	m.writes = append(m.writes, path)
	return nil
}

func TestBuildDiffsAndRestores(t *testing.T) {
	const path = "/abs/main.cpp"
	const orig = "for (auto x : items) {}\n"
	// What "clang-tidy --fix" would write to the file in place.
	const fixed = "for (const auto& x : items) {}\n"

	fs := newMemFS(map[string]string{path: orig})
	ef := &fixes.ExportFile{
		MainSourceFile: path,
		Diagnostics: []fixes.Diagnostic{{
			DiagnosticName: "performance-for-range-copy", // PX1001, L1
			DiagnosticMessage: fixes.DiagnosticMessage{
				FilePath: path,
				Replacements: []fixes.Replacement{
					{FilePath: path, Offset: 5, Length: 6, ReplacementText: "const auto& x"},
				},
			},
		}},
	}

	// runFix simulates the real --fix: rewrite the file in place.
	runFix := func() error {
		return fs.WriteFile(path, []byte(fixed))
	}

	diffs, snapshots, err := Build(ef, catalog.LevelAggressive, runFix, fs)
	if err != nil {
		t.Fatal(err)
	}
	if len(diffs) != 1 {
		t.Fatalf("got %d diffs, want 1", len(diffs))
	}
	if !strings.Contains(diffs[0].Patch, "-for (auto x : items) {}") ||
		!strings.Contains(diffs[0].Patch, "+for (const auto& x : items) {}") {
		t.Errorf("patch missing expected -/+ lines:\n%s", diffs[0].Patch)
	}

	// The snapshot must hold the ORIGINAL bytes.
	if got := string(snapshots[path]); got != orig {
		t.Errorf("snapshot = %q, want original %q", got, orig)
	}
	// After Build returns, the deferred restore must have written the original
	// back — the file is byte-identical to its snapshot.
	if got := fs.files[path]; got != orig {
		t.Errorf("file not restored: on disk %q, want %q", got, orig)
	}
	// And the restore write actually happened (last write is the restore).
	if len(fs.writes) < 2 || fs.writes[len(fs.writes)-1] != path {
		t.Errorf("expected a restore write of %s, writes = %v", path, fs.writes)
	}
}

func TestBuildLevelGating(t *testing.T) {
	const path = "/abs/v.cpp"
	ef := &fixes.ExportFile{
		MainSourceFile: path,
		Diagnostics: []fixes.Diagnostic{{
			DiagnosticName: "performance-inefficient-vector-operation", // PX2001, L2
			DiagnosticMessage: fixes.DiagnosticMessage{
				FilePath:     path,
				Replacements: []fixes.Replacement{{FilePath: path, Offset: 0, Length: 1, ReplacementText: "X"}},
			},
		}},
	}
	// At -level 1 the L2 diagnostic is gated out: the file is not even
	// snapshotted, and runFix (were it real) would touch nothing of interest.
	fs := newMemFS(map[string]string{path: "abc"})
	diffs, snapshots, err := Build(ef, catalog.LevelIdiomatic, func() error { return nil }, fs)
	if err != nil {
		t.Fatal(err)
	}
	if len(diffs) != 0 {
		t.Fatalf("level-1 gating should drop the L2 fix, got %d diffs", len(diffs))
	}
	if len(snapshots) != 0 {
		t.Fatalf("level-1 gating should snapshot no files, got %d", len(snapshots))
	}

	// At -level 2 the file IS snapshotted; simulate --fix rewriting it.
	fs = newMemFS(map[string]string{path: "abc"})
	diffs, snapshots, err = Build(ef, catalog.LevelStructured, func() error {
		return fs.WriteFile(path, []byte("Xbc"))
	}, fs)
	if err != nil {
		t.Fatal(err)
	}
	if len(diffs) != 1 {
		t.Fatalf("level-2 should include the L2 fix, got %d diffs", len(diffs))
	}
	if got := fs.files[path]; got != "abc" {
		t.Errorf("file not restored at level 2: %q", got)
	}
	if string(snapshots[path]) != "abc" {
		t.Errorf("snapshot wrong: %q", snapshots[path])
	}
}

func TestBuildRestoresOnFixError(t *testing.T) {
	const path = "/abs/main.cpp"
	const orig = "original\n"
	fs := newMemFS(map[string]string{path: orig})
	ef := &fixes.ExportFile{
		MainSourceFile: path,
		Diagnostics: []fixes.Diagnostic{{
			DiagnosticName: "performance-for-range-copy",
			DiagnosticMessage: fixes.DiagnosticMessage{
				FilePath:     path,
				Replacements: []fixes.Replacement{{FilePath: path, Offset: 0, Length: 1, ReplacementText: "X"}},
			},
		}},
	}
	// runFix corrupts the file THEN fails — the restore must still fire.
	runFix := func() error {
		_ = fs.WriteFile(path, []byte("HALF-WRITTEN"))
		return errors.New("clang-tidy blew up")
	}
	_, snapshots, err := Build(ef, catalog.LevelAggressive, runFix, fs)
	if err == nil {
		t.Fatal("expected an error from a failing runFix")
	}
	if got := fs.files[path]; got != orig {
		t.Errorf("file must be restored even when runFix fails: on disk %q, want %q", got, orig)
	}
	if string(snapshots[path]) != orig {
		t.Errorf("snapshot wrong: %q", snapshots[path])
	}
}

func TestBuildRestoresOnPanic(t *testing.T) {
	const path = "/abs/main.cpp"
	const orig = "original\n"
	fs := newMemFS(map[string]string{path: orig})
	ef := &fixes.ExportFile{
		MainSourceFile: path,
		Diagnostics: []fixes.Diagnostic{{
			DiagnosticName: "performance-for-range-copy",
			DiagnosticMessage: fixes.DiagnosticMessage{
				FilePath:     path,
				Replacements: []fixes.Replacement{{FilePath: path, Offset: 0, Length: 1, ReplacementText: "X"}},
			},
		}},
	}
	runFix := func() error {
		_ = fs.WriteFile(path, []byte("CORRUPT"))
		panic("boom in --fix")
	}
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected the panic to propagate")
			}
		}()
		_, _, _ = Build(ef, catalog.LevelAggressive, runFix, fs)
	}()
	// The deferred restore in Build must have run before the panic unwound past it.
	if got := fs.files[path]; got != orig {
		t.Errorf("file must be restored even on panic: on disk %q, want %q", got, orig)
	}
}

// TestBuildSnapshotReadError pins that a diagnostic pointing at a file that
// cannot be read surfaces a snapshot error (rather than silently proceeding to
// -fix an unsnapshotted, hence unrestorable, file).
func TestBuildSnapshotReadError(t *testing.T) {
	const missing = "/abs/vanished.cpp"
	fs := newMemFS(map[string]string{}) // the target is NOT present
	ef := &fixes.ExportFile{
		MainSourceFile: missing,
		Diagnostics: []fixes.Diagnostic{{
			DiagnosticName: "performance-for-range-copy",
			DiagnosticMessage: fixes.DiagnosticMessage{
				FilePath:     missing,
				Replacements: []fixes.Replacement{{FilePath: missing, Offset: 0, Length: 1, ReplacementText: "x"}},
			},
		}},
	}
	_, _, err := Build(ef, catalog.LevelAggressive, func() error { return nil }, fs)
	if err == nil || !strings.Contains(err.Error(), "snapshot") {
		t.Fatalf("Build with an unreadable target = %v, want a snapshot error", err)
	}
}

// failWriteFS reads like memFS but fails WriteFile for one path, to exercise the
// restore-write error path (a failed restore must be surfaced, not swallowed —
// otherwise -diff could leave the tree modified).
type failWriteFS struct {
	*memFS
	failPath string
}

func (f *failWriteFS) WriteFile(path string, data []byte) error {
	if path == f.failPath {
		return errors.New("disk full")
	}
	return f.memFS.WriteFile(path, data)
}

func TestBuildRestoreWriteError(t *testing.T) {
	const path = "/abs/main.cpp"
	fs := &failWriteFS{memFS: newMemFS(map[string]string{path: "orig\n"}), failPath: path}
	ef := &fixes.ExportFile{
		MainSourceFile: path,
		Diagnostics: []fixes.Diagnostic{{
			DiagnosticName: "performance-for-range-copy",
			DiagnosticMessage: fixes.DiagnosticMessage{
				FilePath:     path,
				Replacements: []fixes.Replacement{{FilePath: path, Offset: 0, Length: 4, ReplacementText: "NEW"}},
			},
		}},
	}
	// runFix "modifies" the file; the deferred restore then fails on WriteFile.
	_, _, err := Build(ef, catalog.LevelAggressive, func() error { return nil }, fs)
	if err == nil || !strings.Contains(err.Error(), "restore") {
		t.Fatalf("Build with a failing restore = %v, want a restore error", err)
	}
}
