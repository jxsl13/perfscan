package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Baseline support ("ratchet"): a baseline file records the accepted
// findings of an existing codebase; subsequent runs report only findings
// NOT covered by it, so CI fails on regressions while the backlog is
// burned down incrementally.
//
// Finding identity is deliberately line-independent — {file, check ID,
// message} with a count per key — so unrelated edits that shift line
// numbers do not resurrect baselined findings. Renaming a file or changing
// a finding's message (e.g. a variable rename) invalidates its baseline
// entry, which errs on the loud side.

type baselineFile struct {
	// Version guards the format for future changes.
	Version int `json:"version" yaml:"version"`
	// Entries maps "file\x00id\x00message" → accepted count. Serialized
	// as a sorted list for stable diffs.
	Entries []baselineEntry `json:"entries" yaml:"entries"`
}

type baselineEntry struct {
	File    string `json:"file" yaml:"file"`
	ID      string `json:"id" yaml:"id"`
	Message string `json:"message" yaml:"message"`
	Count   int    `json:"count" yaml:"count"`
}

// baselineAnchor is the directory a baseline's paths are keyed against: the
// absolute directory containing the baseline file. Keying on the baseline's own
// location (the model .gitignore uses — paths relative to the file, not the
// process CWD) makes suppression survive a run from a different directory, e.g. a
// baseline written at the repo root and checked from a subdir in CI. Absolutizing
// here is what decouples the anchor from the invocation CWD.
func baselineAnchor(baselinePath string) string {
	if abs, err := filepath.Abs(baselinePath); err == nil {
		return filepath.Dir(abs)
	}
	return filepath.Dir(baselinePath)
}

// baselineRelPath renders a finding's path relative to anchorDir for a stable,
// invocation-CWD-independent key. It reconstructs the finding's absolute path
// first (filepath.Abs recovers it from the same CWD the finding was produced
// under — go/analysis positions are already absolute, so this is usually a
// no-op) and re-anchors it to anchorDir. For the common case (baseline at the
// module root, run from the root) the key equals the previous CWD-relative one,
// so existing baselines keep matching; a path that cannot be made relative
// (different volume) falls back to its slash-normalized absolute form.
func baselineRelPath(p, anchorDir string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	if rel, err := filepath.Rel(anchorDir, abs); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(abs)
}

func baselineKey(f *Finding, anchorDir string) string {
	return baselineRelPath(f.Pos.Filename, anchorDir) + "\x00" + f.Check.ID + "\x00" + f.Message
}

// writeBaseline writes the current findings as the accepted baseline.
func writeBaseline(path string, findings []Finding) error {
	anchor := baselineAnchor(path)
	counts := make(map[string]*baselineEntry, len(findings))
	for i := range findings {
		f := &findings[i]
		k := baselineKey(f, anchor)
		if e, ok := counts[k]; ok {
			e.Count++
			continue
		}
		counts[k] = &baselineEntry{
			File:    baselineRelPath(f.Pos.Filename, anchor),
			ID:      f.Check.ID,
			Message: f.Message,
			Count:   1,
		}
	}
	entries := make([]baselineEntry, 0, len(counts))
	for _, e := range counts {
		entries = append(entries, *e)
	}
	slices.SortFunc(entries, func(a, b baselineEntry) int {
		if c := strings.Compare(a.File, b.File); c != 0 {
			return c
		}
		if c := strings.Compare(a.ID, b.ID); c != 0 {
			return c
		}
		return strings.Compare(a.Message, b.Message)
	})
	b, err := yaml.Marshal(baselineFile{Version: 1, Entries: entries})
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, b, 0o644)
}

// applyBaseline drops findings covered by the baseline, consuming counts.
// It returns the surviving findings and the number suppressed.
func applyBaseline(path string, findings []Finding) ([]Finding, int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return findings, 0, err
	}
	var bf baselineFile
	if err := yaml.Unmarshal(raw, &bf); err != nil {
		return findings, 0, fmt.Errorf("%s: %w", path, err)
	}
	if bf.Version != 1 {
		return findings, 0, fmt.Errorf("%s: unsupported baseline version %d", path, bf.Version)
	}
	anchor := baselineAnchor(path)
	budget := make(map[string]int, len(bf.Entries))
	for _, e := range bf.Entries {
		budget[e.File+"\x00"+e.ID+"\x00"+e.Message] += e.Count
	}
	out := make([]Finding, 0, len(findings))
	suppressed := 0
	for i := range findings {
		f := &findings[i]
		k := baselineKey(f, anchor)
		if budget[k] > 0 {
			budget[k]--
			suppressed++
			continue
		}
		out = append(out, *f)
	}
	return out, suppressed, nil
}
