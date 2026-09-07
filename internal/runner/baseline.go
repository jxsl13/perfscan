package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
	"gopkg.in/yaml.v3"
)

// Baseline support ("ratchet"): a baseline file records the accepted
// findings of an existing codebase; subsequent runs report only findings
// NOT covered by it, so CI fails on regressions while the backlog is
// burned down incrementally.
//
// Finding identity is deliberately line-independent — {module, file, check ID,
// message} with a count per key — so unrelated edits that shift line
// numbers do not resurrect baselined findings. Renaming a file or changing
// a finding's message (e.g. a variable rename) invalidates its baseline
// entry, which errs on the loud side.

type baselineFile struct {
	// Version guards the format for future changes.
	Version int `json:"version" yaml:"version"`
	// Metadata fingerprints the toolchain that produced this evidence. It is
	// optional so version-1 baselines written by older perfscan releases remain
	// readable and can produce an actionable warning instead of failing.
	Metadata evidenceMetadata `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	// Entries maps "module\x00file\x00id\x00message" → accepted count. Serialized
	// as a sorted list for stable diffs.
	Entries []baselineEntry `json:"entries" yaml:"entries"`
}

type baselineEntry struct {
	// Module scopes module-relative paths in version 2. An empty module keeps
	// the baseline-file-relative convention for sources without module metadata.
	Module  string `json:"module,omitempty" yaml:"module,omitempty"`
	File    string `json:"file" yaml:"file"`
	ID      string `json:"id" yaml:"id"`
	Message string `json:"message" yaml:"message"`
	Count   int    `json:"count" yaml:"count"`
}

// baselineAnchor is the legacy/fallback directory a baseline's paths are keyed
// against: the absolute directory containing the baseline file. Keying on its own
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

func (entry *baselineEntry) key() string {
	return entry.Module + "\x00" + entry.File + "\x00" + entry.ID + "\x00" + entry.Message
}

// baselineFindingEntry uses the module metadata already discovered by the Go
// package loader, not a common directory inferred from the reported findings.
// That keeps identity stable when scanning a subset or when findings disappear,
// and distinguishes identically named files in different workspace modules.
func baselineFindingEntry(f *Finding, anchorDir string, version int, modules []*packages.Module) baselineEntry {
	entry := baselineEntry{
		ID: f.Check.ID, Message: f.Message, Count: 1,
	}
	if version >= 2 {
		if name, err := filepath.Abs(f.Pos.Filename); err == nil {
			for _, module := range modules {
				rel, err := filepath.Rel(module.Dir, name)
				if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
					continue
				}
				if module.Path == "" {
					break // ambiguous module identity: use the conservative fallback
				}
				entry.Module = module.Path
				entry.File = filepath.ToSlash(rel)
				return entry
			}
		}
	}
	// A finding outside its package's module (for example a //line path) must
	// not acquire another source file's module-relative identity.
	entry.File = baselineRelPath(f.Pos.Filename, anchorDir)
	return entry
}

func baselineModules(pkgs []*packages.Package) []*packages.Module {
	byDir := make(map[string]*packages.Module, len(pkgs))
	for _, pkg := range pkgs {
		if pkg.Module == nil || pkg.Module.Dir == "" || pkg.Module.Path == "" {
			continue
		}
		module := pkg.Module
		if prior := byDir[module.Dir]; prior != nil && prior.Path != module.Path {
			// Different module identities can use the same replacement directory.
			// A filename alone cannot disambiguate their findings.
			byDir[module.Dir] = &packages.Module{Dir: module.Dir}
			continue
		}
		byDir[module.Dir] = module
	}
	modules := make([]*packages.Module, 0, len(byDir))
	for _, module := range byDir {
		modules = append(modules, module)
	}
	// Nested workspace modules take precedence over their containing module.
	slices.SortFunc(modules, func(a, b *packages.Module) int {
		return len(b.Dir) - len(a.Dir)
	})
	return modules
}

// writeBaseline writes the current findings as the accepted baseline.
func writeBaseline(path string, findings []Finding, metadata evidenceMetadata, pkgs ...*packages.Package) error {
	anchor := baselineAnchor(path)
	modules := baselineModules(pkgs)
	counts := make(map[string]*baselineEntry, len(findings))
	for i := range findings {
		entry := baselineFindingEntry(&findings[i], anchor, 2, modules)
		k := entry.key()
		if e, ok := counts[k]; ok {
			e.Count++
			continue
		}
		counts[k] = &entry
	}
	entries := make([]baselineEntry, 0, len(counts))
	for _, e := range counts {
		entries = append(entries, *e)
	}
	slices.SortFunc(entries, func(a, b baselineEntry) int {
		if c := strings.Compare(a.Module, b.Module); c != 0 {
			return c
		}
		if c := strings.Compare(a.File, b.File); c != 0 {
			return c
		}
		if c := strings.Compare(a.ID, b.ID); c != 0 {
			return c
		}
		return strings.Compare(a.Message, b.Message)
	})
	b, err := yaml.Marshal(baselineFile{Version: 2, Metadata: metadata, Entries: entries})
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
func applyBaseline(path string, findings []Finding, metadata evidenceMetadata, pkgs ...*packages.Package) ([]Finding, int, []evidenceWarning, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return findings, 0, nil, err
	}
	var bf baselineFile
	if err := yaml.Unmarshal(raw, &bf); err != nil {
		return findings, 0, nil, fmt.Errorf("%s: %w", path, err)
	}
	if bf.Version != 1 && bf.Version != 2 {
		return findings, 0, nil, fmt.Errorf("%s: unsupported baseline version %d", path, bf.Version)
	}
	anchor := baselineAnchor(path)
	modules := baselineModules(pkgs)
	budget := make(map[string]int, len(bf.Entries))
	for _, e := range bf.Entries {
		if bf.Version == 1 {
			e.Module = "" // version 1 always uses baseline-file-relative paths
		}
		budget[e.key()] += e.Count
	}
	out := make([]Finding, 0, len(findings))
	suppressed := 0
	for i := range findings {
		f := &findings[i]
		entry := baselineFindingEntry(f, anchor, bf.Version, modules)
		k := entry.key()
		if budget[k] > 0 {
			budget[k]--
			suppressed++
			continue
		}
		out = append(out, *f)
	}
	return out, suppressed, baselineMetadataWarnings(bf.Metadata, metadata), nil
}
