package runner

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/checks"
	"golang.org/x/tools/go/packages"
	"gopkg.in/yaml.v3"
)

// Each invocation runs in its own process: Run configures process-wide state,
// whereas the parent regression test must remain safe to run in parallel.
func TestBaselinePortableAcrossWorktrees(t *testing.T) {
	t.Parallel()
	const helper = "PERFSCAN_BASELINE_PORTABILITY_HELPER"
	if os.Getenv(helper) == "1" {
		os.Exit(Run(checks.All(), Options{
			Checks:        "PS2101",
			Baseline:      os.Getenv("PERFSCAN_BASELINE_PORTABILITY_FILE"),
			WriteBaseline: os.Getenv("PERFSCAN_BASELINE_PORTABILITY_WRITE") == "1",
		}))
	}

	root := t.TempDir()
	baselinePath := filepath.Join(root, "evidence", "baseline.yaml")
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const source = `package sample
func collect(src []string) []string {
	out := []string{}
	for _, s := range src { out = append(out, s) }
	return out
}
`
	oldRoot := filepath.Join(root, "baseline-checkout")
	newRoot := filepath.Join(root, "candidate-with-different-name")
	for _, dir := range []string{oldRoot, newRoot} {
		write(filepath.Join(dir, "go.mod"), "module example.org/portable\n\ngo 1.25.0\n")
		write(filepath.Join(dir, "pkg", "sample.go"), source)
	}
	run := func(dir string, seed bool, wantExit int) string {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestBaselinePortableAcrossWorktrees$")
		cmd.Dir = dir
		seedValue := "0"
		if seed {
			seedValue = "1"
		}
		cmd.Env = append(os.Environ(), helper+"=1", "GOWORK=off",
			"PERFSCAN_BASELINE_PORTABILITY_FILE="+baselinePath,
			"PERFSCAN_BASELINE_PORTABILITY_WRITE="+seedValue)
		out, err := cmd.CombinedOutput()
		if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != wantExit {
			t.Fatalf("baseline invocation in %s: %v; want exit %d\n%s", dir, err, wantExit, out)
		}
		return string(out)
	}
	run(oldRoot, true, 0)
	before, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatal(err)
	}
	var bf baselineFile
	if err := yaml.Unmarshal(before, &bf); err != nil {
		t.Fatal(err)
	}
	if len(bf.Entries) != 1 || bf.Entries[0].File != "pkg/sample.go" {
		t.Fatalf("baseline paths must exclude both checkout and evidence directories: %+v", bf.Entries)
	}
	if bf.Version != 2 || bf.Entries[0].Module != "example.org/portable" {
		t.Fatalf("new baseline must record versioned module identity: %+v", bf)
	}
	if out := run(newRoot, false, 0); !strings.Contains(out, "1 baselined finding(s) suppressed") {
		t.Fatalf("relocated identical finding was not suppressed:\n%s", out)
	}
	if out := run(filepath.Join(newRoot, "pkg"), false, 0); !strings.Contains(out, "1 baselined finding(s) suppressed") {
		t.Fatalf("subdirectory invocation changed baseline identity:\n%s", out)
	}
	// A new occurrence of the same finding must exceed the stored count.
	write(filepath.Join(newRoot, "pkg", "sample.go"), source+strings.ReplaceAll(strings.TrimPrefix(source, "package sample\n"), "collect", "collectAgain"))
	if out := run(newRoot, false, 1); !strings.Contains(out, "1 baselined finding(s) suppressed") {
		t.Fatalf("regression run did not retain the old suppression:\n%s", out)
	}
	after, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("filtering must not rewrite the baseline")
	}
}

func TestBaselinePortableModuleCounts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "baseline.yaml")
	metadata := fakeEvidenceMetadata("go1.25.3", "1.25.0")
	var pkgs []*packages.Package
	finding := func(checkout, module string, line int) Finding {
		dir := filepath.Join(root, checkout, module)
		f := fakeFinding(filepath.Join(dir, "pkg", "same.go"), "PS2101", "same message", line)
		pkgs = append(pkgs, &packages.Package{Module: &packages.Module{Path: "example.org/" + module, Dir: dir}})
		return f
	}
	old := []Finding{finding("before", "one", 1), finding("before", "one", 2), finding("before", "two", 1)}
	if err := writeBaseline(path, old, metadata, pkgs...); err != nil {
		t.Fatal(err)
	}
	// The second occurrence in module two must not consume module one's budget.
	current := []Finding{finding("after", "two", 10), finding("after", "two", 20), finding("after", "one", 30), finding("after", "one", 40)}
	remaining, suppressed, _, err := applyBaseline(path, current, metadata, pkgs...)
	if err != nil {
		t.Fatal(err)
	}
	if suppressed != 3 || len(remaining) != 1 || remaining[0].Pos.Line != 20 {
		t.Fatalf("module-relative counts collided: suppressed=%d remaining=%+v", suppressed, remaining)
	}
	// A third, unbaselined module with exactly the same relative file and
	// diagnostic must remain visible, not inherit another module's acceptance.
	third := []Finding{finding("after", "three", 1)}
	remaining, suppressed, _, err = applyBaseline(path, third, metadata, pkgs...)
	if err != nil || suppressed != 0 || len(remaining) != 1 {
		t.Fatalf("unrelated module suppressed: suppressed=%d remaining=%+v err=%v", suppressed, remaining, err)
	}
}

func TestBaselinePortableFallbackAndLegacyPaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "module")
	path := filepath.Join(root, "evidence", "baseline.yaml")
	metadata := fakeEvidenceMetadata("go1.25.3", "1.25.0")
	f := fakeFinding(filepath.Join(dir, "pkg", "sample.go"), "PS2101", "same message", 1)
	pkgs := []*packages.Package{{Module: &packages.Module{Path: "example.org/portable", Dir: dir}}}
	modules := baselineModules(pkgs)
	legacyEntry := baselineFindingEntry(&f, baselineAnchor(path), 1, modules)
	if legacyEntry.Module != "" || legacyEntry.File != "../module/pkg/sample.go" {
		t.Fatalf("legacy identity changed: %+v", legacyEntry)
	}
	legacy, err := yaml.Marshal(baselineFile{Version: 1, Metadata: metadata, Entries: []baselineEntry{legacyEntry}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	remaining, suppressed, _, err := applyBaseline(path, []Finding{f}, metadata, pkgs...)
	if err != nil || suppressed != 1 || len(remaining) != 0 {
		t.Fatalf("version-1 baseline must retain its exact convention: suppressed=%d remaining=%+v err=%v", suppressed, remaining, err)
	}
	// A file outside the module cannot borrow a module-relative identity.
	f.Pos.Filename = filepath.Join(root, "module-other", "pkg", "sample.go")
	outside := baselineFindingEntry(&f, baselineAnchor(path), 2, modules)
	if outside.Module != "" || outside.File != "../module-other/pkg/sample.go" {
		t.Fatalf("outside-module path incorrectly scoped: %+v", outside)
	}
	withoutModule := baselineFindingEntry(&f, baselineAnchor(path), 2, nil)
	if outside != withoutModule {
		t.Fatalf("moduleless fallback changed: got %+v want %+v", withoutModule, outside)
	}
}

func TestBaselineRejectsUnknownVersion(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "baseline.yaml")
	if err := os.WriteFile(path, []byte("version: 99\nentries: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := applyBaseline(path, nil, evidenceMetadata{})
	if err == nil || !strings.Contains(err.Error(), "unsupported baseline version 99") {
		t.Fatalf("unknown baseline format must fail loudly: %v", err)
	}
}

func TestBaselineNestedAndAmbiguousModuleRoots(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	outerPackage := &packages.Package{Module: &packages.Module{Path: "example.org/outer", Dir: root}}
	innerPackage := &packages.Package{Module: &packages.Module{Path: "example.org/inner", Dir: nested}}
	f := fakeFinding(filepath.Join(nested, "same.go"), "PS2101", "message", 1)
	modules := baselineModules([]*packages.Package{outerPackage, innerPackage, innerPackage, {}})
	entry := baselineFindingEntry(&f, root, 2, modules)
	if len(modules) != 2 || entry.Module != "example.org/inner" || entry.File != "same.go" {
		t.Fatalf("nested module did not take precedence: %+v, roots=%+v", entry, modules)
	}
	otherPackage := &packages.Package{Module: &packages.Module{Path: "example.org/other", Dir: nested}}
	modules = baselineModules([]*packages.Package{outerPackage, innerPackage, otherPackage, innerPackage})
	entry = baselineFindingEntry(&f, root, 2, modules)
	if entry.Module != "" || entry.File != "nested/same.go" {
		t.Fatalf("ambiguous replacement directory borrowed a module identity: %+v", entry)
	}
}
