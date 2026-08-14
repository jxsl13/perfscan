// Package baseline implements a "ratchet": record the accepted findings of an
// existing codebase so later runs report only findings NOT covered by it — CI
// fails on regressions while the existing backlog is burned down incrementally.
//
// Finding identity is deliberately line-INDEPENDENT — {file, check id, message}
// with a count per key — so unrelated edits that shift line numbers do not
// resurrect baselined findings. Renaming a file or changing a finding's message
// invalidates its entry, which errs on the loud side. Mirrors perfscan's model.
package baseline

import (
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/jxsl13/perfscan/perfscanxx/internal/report"
)

type file struct {
	Version int     `yaml:"version"`
	Entries []entry `yaml:"entries"`
}

type entry struct {
	File    string `yaml:"file"`
	ID      string `yaml:"id"`
	Message string `yaml:"message"`
	Count   int    `yaml:"count"`
}

// relPath renders a finding's path relative to anchorDir (the directory of the
// baseline file) for a portable, stable, invocation-independent key — the same
// model .gitignore uses, where paths are relative to the file itself, not the
// process CWD. It first reconstructs the finding's ABSOLUTE path: a finding's
// File is already CWD-relative (report.displayPath), so filepath.Abs recovers
// the absolute path using the same CWD the finding was built under, then
// filepath.Rel re-anchors it to anchorDir. Keying on the baseline's own location
// (rather than the CWD) is what makes suppression survive a run from a different
// directory — e.g. a baseline written at the repo root and checked from build/
// in CI. For the common case (baseline at the root, run from the root) the key
// is identical to the previous CWD-relative one, so existing baselines keep
// matching. A path that cannot be made relative (different volume, unresolvable)
// falls back to its slash-normalized absolute form.
func relPath(p, anchorDir string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	if rel, err := filepath.Rel(anchorDir, abs); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(abs)
}

// anchorFor is the directory a baseline's paths are keyed against: the absolute
// directory containing the baseline file. Absolutizing here (rather than storing
// a CWD-relative anchor) is what decouples the key from the invocation CWD.
func anchorFor(baselinePath string) string {
	if abs, err := filepath.Abs(baselinePath); err == nil {
		return filepath.Dir(abs)
	}
	return filepath.Dir(baselinePath)
}

type key struct{ file, id, message string }

func keyOf(f report.Finding, anchorDir string) key {
	return key{relPath(f.File, anchorDir), f.ID, f.Message}
}

// Exists reports whether a baseline file is present.
func Exists(path string) bool { _, err := os.Stat(path); return err == nil }

// Write records findings as the accepted baseline (sorted for stable diffs) and
// returns how many findings were written.
func Write(path string, findings []report.Finding) (int, error) {
	anchor := anchorFor(path)
	counts := map[key]int{}
	for _, f := range findings {
		counts[keyOf(f, anchor)]++
	}
	entries := make([]entry, 0, len(counts))
	for k, n := range counts {
		entries = append(entries, entry{File: k.file, ID: k.id, Message: k.message, Count: n})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].File != entries[j].File {
			return entries[i].File < entries[j].File
		}
		if entries[i].ID != entries[j].ID {
			return entries[i].ID < entries[j].ID
		}
		return entries[i].Message < entries[j].Message
	})
	data, err := yaml.Marshal(file{Version: 1, Entries: entries})
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return 0, err
	}
	return len(findings), nil
}

// Filter loads the baseline at path and returns the findings NOT covered by it
// (the regressions) plus the number suppressed. Each baselined {file,id,message}
// suppresses up to its recorded Count of matching findings.
//
// stale is the number of baselined findings that did NOT occur this run (their
// key was recorded with a higher Count than the run produced — usually because
// they were fixed). A stale entry still holds suppression "credit", so it would
// MASK the same finding if it were reintroduced; the caller surfaces stale > 0 so
// the user can regenerate the baseline to tighten the ratchet.
func Filter(path string, findings []report.Finding) (kept []report.Finding, suppressed, stale int, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, 0, err
	}
	var bf file
	if err := yaml.Unmarshal(data, &bf); err != nil {
		return nil, 0, 0, err
	}
	anchor := anchorFor(path)
	remaining := make(map[key]int, len(bf.Entries))
	for _, e := range bf.Entries {
		remaining[key{e.File, e.ID, e.Message}] += e.Count
	}
	for _, f := range findings {
		if k := keyOf(f, anchor); remaining[k] > 0 {
			remaining[k]--
			suppressed++
			continue
		}
		kept = append(kept, f)
	}
	// Whatever suppression credit went unspent is stale: those baselined findings
	// no longer occur, so their entries now only serve to mask a reintroduction.
	for _, n := range remaining {
		if n > 0 {
			stale += n
		}
	}
	return kept, suppressed, stale, nil
}
