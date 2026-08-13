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

// TestBuildMultiFileDiffsSortedAndRestored pins the multi-file shape that real
// projects produce (motivated by a corpus run on fmtlib/fmt, where one clang-
// tidy --fix touched base.h, format.h AND os.h): a single ExportFile whose
// diagnostics edit THREE separate files. Build must (1) return one FileDiff per
// changed file, DETERMINISTICALLY ordered by path regardless of diagnostic input
// order, and (2) snapshot+restore each file INDEPENDENTLY so every one is byte-
// identical afterward. Every existing Build test edits a single file, so the
// sort-and-per-file-restore path was unexercised.
func TestBuildMultiFileDiffsSortedAndRestored(t *testing.T) {
	// Intentionally NOT in sorted order, to prove Build sorts.
	const pB = "/abs/b.cpp"
	const pA = "/abs/a.cpp"
	const pC = "/abs/c.cpp"
	orig := map[string]string{
		pB: "for (auto x : bs) {}\n",
		pA: "for (auto x : as) {}\n",
		pC: "for (auto x : cs) {}\n",
	}
	fixed := map[string]string{
		pB: "for (const auto& x : bs) {}\n",
		pA: "for (const auto& x : as) {}\n",
		pC: "for (const auto& x : cs) {}\n",
	}
	fs := newMemFS(orig)
	mkDiag := func(p string) fixes.Diagnostic {
		return fixes.Diagnostic{
			DiagnosticName: "performance-for-range-copy", // PX1001, L1
			DiagnosticMessage: fixes.DiagnosticMessage{
				FilePath:     p,
				Replacements: []fixes.Replacement{{FilePath: p, Offset: 5, Length: 6, ReplacementText: "const auto& x"}},
			},
		}
	}
	ef := &fixes.ExportFile{
		MainSourceFile: pA,
		Diagnostics:    []fixes.Diagnostic{mkDiag(pB), mkDiag(pC), mkDiag(pA)}, // scrambled
	}
	runFix := func() error {
		for p, f := range fixed {
			if err := fs.WriteFile(p, []byte(f)); err != nil {
				return err
			}
		}
		return nil
	}

	diffs, snapshots, err := Build(ef, catalog.LevelAggressive, runFix, fs)
	if err != nil {
		t.Fatal(err)
	}
	if len(diffs) != 3 {
		t.Fatalf("got %d diffs, want 3 (one per file)", len(diffs))
	}
	// (1) Deterministic path-sorted order: a.cpp, b.cpp, c.cpp.
	wantOrder := []string{pA, pB, pC}
	for i, d := range diffs {
		if d.AbsPath != wantOrder[i] {
			t.Errorf("diffs[%d].AbsPath = %q, want %q (sorted)", i, d.AbsPath, wantOrder[i])
		}
		if !strings.Contains(d.Patch, "+for (const auto& x :") {
			t.Errorf("diffs[%d] patch missing the fixed line:\n%s", i, d.Patch)
		}
	}
	// (2) Every file snapshotted with its own original and restored on disk.
	for p, want := range orig {
		if got := string(snapshots[p]); got != want {
			t.Errorf("snapshot[%s] = %q, want %q", p, got, want)
		}
		if got := fs.files[p]; got != want {
			t.Errorf("file %s not restored: on disk %q, want %q", p, got, want)
		}
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

// failReadAfterFixFS reads and writes like memFS, but once armed (set by the
// runFix callback) its ReadFile fails for one path — reproducing the case where
// a file is snapshotted fine, --fix rewrites it, but reading the MODIFIED bytes
// back errors. Build must record that read error and skip the file's diff, not
// crash or emit a bogus hunk.
type failReadAfterFixFS struct {
	*memFS
	failPath string
	armed    *bool
}

func (f *failReadAfterFixFS) ReadFile(path string) ([]byte, error) {
	if *f.armed && path == f.failPath {
		return nil, errors.New("read error after fix")
	}
	return f.memFS.ReadFile(path)
}

func TestBuildReadModifiedError(t *testing.T) {
	const path = "/abs/main.cpp"
	armed := false
	fs := &failReadAfterFixFS{memFS: newMemFS(map[string]string{path: "orig\n"}), failPath: path, armed: &armed}
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
	// runFix rewrites the file (so a diff WOULD exist), then arms the read
	// failure so the subsequent re-read of the modified bytes errors.
	runFix := func() error {
		if err := fs.WriteFile(path, []byte("fixed\n")); err != nil {
			return err
		}
		armed = true
		return nil
	}

	diffs, _, err := Build(ef, catalog.LevelAggressive, runFix, fs)
	if err == nil || !strings.Contains(err.Error(), "reading modified") {
		t.Fatalf("Build with a failing re-read = %v, want a 'reading modified' error", err)
	}
	if len(diffs) != 0 {
		t.Errorf("no diff should be produced when the modified read fails, got %d", len(diffs))
	}
	// The deferred restore still runs (it writes, not reads): file is restored.
	if got := fs.files[path]; got != "orig\n" {
		t.Errorf("file not restored after the read error: on disk %q, want %q", got, "orig\n")
	}
}
