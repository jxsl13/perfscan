package diff

import (
	"fmt"
	"os"
	"sort"

	"github.com/jxsl13/perfscan/perfscanxx/internal/catalog"
	"github.com/jxsl13/perfscan/perfscanxx/internal/fixes"
	"github.com/jxsl13/perfscan/perfscanxx/internal/report"
)

// OSFS is the production FS: it reads and writes real files, preserving each
// file's existing permission bits on write.
type OSFS struct{}

func (OSFS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (OSFS) WriteFile(path string, data []byte) error {
	perm := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		perm = fi.Mode().Perm()
	}
	return os.WriteFile(path, data, perm)
}

// FileDiff is the unified diff for one file that -fix would change.
type FileDiff struct {
	// Path is the cwd-relative (display) path used in the ---/+++ headers.
	Path string
	// AbsPath is the resolved absolute path whose bytes changed.
	AbsPath string
	// Patch is the rendered unified diff (non-empty).
	Patch string
}

// FS abstracts the disk operations the snapshot/fix/restore flow needs, so the
// driver is unit-testable without touching real files. The production
// implementation is OSFS.
type FS interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte) error
}

// Build renders the unified diff that -fix would produce, by construction
// identical to what `perfscanxx -fix` writes:
//
//  1. From the already-parsed --export-fixes YAML (produced by the SAME
//     clang-tidy run, WITHOUT --fix) it collects the set of files that carry
//     replacements, level-gated exactly as report.FromExport / -fix are.
//  2. It SNAPSHOTS each such file's original bytes into memory and immediately
//     registers a deferred restore that writes every snapshot back — so no file
//     is ever left modified, on any return path OR on panic.
//  3. It runs runFix, which must invoke clang-tidy with --fix over the same
//     inputs (the real -fix invocation). clang-tidy rewrites the files in place,
//     coalescing/cleaning adjacent edits — so the on-disk result is what -fix
//     yields, not a naive replay of raw export offsets.
//  4. For each snapshotted file it reads the now-modified bytes and, if they
//     differ, renders a unified diff of original -> modified.
//  5. The deferred restore then writes the originals back.
//
// snapshots is returned so callers/tests can assert every touched file is
// byte-identical to its snapshot after Build returns (i.e. restore worked).
// err is the first read/write error encountered (the restore still runs).
func Build(
	ef *fixes.ExportFile,
	maxLevel catalog.Level,
	runFix func() error,
	fsys FS,
) (diffs []FileDiff, snapshots map[string][]byte, err error) {
	// (1) Files that -fix would touch: every replacement target of a diagnostic
	// whose catalog level is within maxLevel. Resolve each replacement's own
	// FilePath (a diagnostic may edit files other than its anchor).
	touched := map[string]bool{}
	for _, d := range ef.Diagnostics {
		if e, ok := catalog.ByTidyName(d.DiagnosticName); ok && e.Level > maxLevel {
			continue
		}
		for _, r := range d.DiagnosticMessage.Replacements {
			abs := report.ResolvePath(r.FilePath, d.BuildDirectory, ef.MainSourceFile)
			touched[abs] = true
		}
	}

	paths := make([]string, 0, len(touched))
	for p := range touched {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	// (2) Snapshot originals, then IMMEDIATELY arm the restore. The defer is
	// registered before runFix so the files are restored on every return path
	// and even if runFix (or anything after) panics.
	snapshots = make(map[string][]byte, len(paths))
	for _, abs := range paths {
		orig, rerr := fsys.ReadFile(abs)
		if rerr != nil {
			if err == nil {
				err = fmt.Errorf("diff: snapshot %s: %w", abs, rerr)
			}
			continue
		}
		snapshots[abs] = orig
	}
	defer func() {
		for abs, orig := range snapshots {
			if werr := fsys.WriteFile(abs, orig); werr != nil && err == nil {
				err = fmt.Errorf("diff: restore %s: %w", abs, werr)
			}
		}
	}()

	// (3) Apply the real -fix in place. On error the deferred restore still runs.
	if runFix != nil {
		if ferr := runFix(); ferr != nil {
			if err == nil {
				err = fmt.Errorf("diff: applying fixes: %w", ferr)
			}
			return nil, snapshots, err
		}
	}

	// (4) Diff each snapshot against its now-modified on-disk bytes.
	for _, abs := range paths {
		orig, ok := snapshots[abs]
		if !ok {
			continue // failed to snapshot; already recorded in err
		}
		modified, rerr := fsys.ReadFile(abs)
		if rerr != nil {
			if err == nil {
				err = fmt.Errorf("diff: reading modified %s: %w", abs, rerr)
			}
			continue
		}
		disp := report.DisplayPath(abs)
		patch := Unified(disp, disp, orig, modified)
		if patch == "" {
			continue
		}
		diffs = append(diffs, FileDiff{Path: disp, AbsPath: abs, Patch: patch})
	}

	// (5) restore runs here via the deferred func above.
	return diffs, snapshots, err
}
