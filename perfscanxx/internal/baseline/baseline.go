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
	"strings"

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

// relPath renders a finding's path relative to the cwd for a portable, stable
// baseline; falls back to the original path if it can't be made relative.
func relPath(p string) string {
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, p); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(p)
}

type key struct{ file, id, message string }

func keyOf(f report.Finding) key { return key{relPath(f.File), f.ID, f.Message} }

// Exists reports whether a baseline file is present.
func Exists(path string) bool { _, err := os.Stat(path); return err == nil }

// Write records findings as the accepted baseline (sorted for stable diffs) and
// returns how many findings were written.
func Write(path string, findings []report.Finding) (int, error) {
	counts := map[key]int{}
	for _, f := range findings {
		counts[keyOf(f)]++
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
func Filter(path string, findings []report.Finding) (kept []report.Finding, suppressed int, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	var bf file
	if err := yaml.Unmarshal(data, &bf); err != nil {
		return nil, 0, err
	}
	remaining := make(map[key]int, len(bf.Entries))
	for _, e := range bf.Entries {
		remaining[key{e.File, e.ID, e.Message}] += e.Count
	}
	for _, f := range findings {
		if k := keyOf(f); remaining[k] > 0 {
			remaining[k]--
			suppressed++
			continue
		}
		kept = append(kept, f)
	}
	return kept, suppressed, nil
}
