package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	Version int `json:"version"`
	// Entries maps "file\x00id\x00message" → accepted count. Serialized
	// as a sorted list for stable diffs.
	Entries []baselineEntry `json:"entries"`
}

type baselineEntry struct {
	File    string `json:"file"`
	ID      string `json:"id"`
	Message string `json:"message"`
	Count   int    `json:"count"`
}

func baselineKey(f Finding) string {
	return relPath(f.Pos.Filename) + "\x00" + f.Check.ID + "\x00" + f.Message
}

// writeBaseline writes the current findings as the accepted baseline.
func writeBaseline(path string, findings []Finding) error {
	counts := map[string]*baselineEntry{}
	for _, f := range findings {
		k := baselineKey(f)
		if e, ok := counts[k]; ok {
			e.Count++
			continue
		}
		counts[k] = &baselineEntry{
			File:    relPath(f.Pos.Filename),
			ID:      f.Check.ID,
			Message: f.Message,
			Count:   1,
		}
	}
	entries := make([]baselineEntry, 0, len(counts))
	for _, e := range counts {
		entries = append(entries, *e)
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		return a.Message < b.Message
	})
	b, err := json.MarshalIndent(baselineFile{Version: 1, Entries: entries}, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// applyBaseline drops findings covered by the baseline, consuming counts.
// It returns the surviving findings and the number suppressed.
func applyBaseline(path string, findings []Finding) ([]Finding, int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return findings, 0, err
	}
	var bf baselineFile
	if err := json.Unmarshal(raw, &bf); err != nil {
		return findings, 0, fmt.Errorf("%s: %w", path, err)
	}
	if bf.Version != 1 {
		return findings, 0, fmt.Errorf("%s: unsupported baseline version %d", path, bf.Version)
	}
	budget := map[string]int{}
	for _, e := range bf.Entries {
		budget[e.File+"\x00"+e.ID+"\x00"+e.Message] += e.Count
	}
	out := make([]Finding, 0, len(findings))
	suppressed := 0
	for _, f := range findings {
		k := baselineKey(f)
		if budget[k] > 0 {
			budget[k]--
			suppressed++
			continue
		}
		out = append(out, f)
	}
	return out, suppressed, nil
}
