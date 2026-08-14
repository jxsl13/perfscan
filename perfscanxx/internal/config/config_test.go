package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadValid(t *testing.T) {
	p := write(t, t.TempDir(), "c.yml", "level: 2\nchecks: PX3013\nexclude:\n  - vendor/\n  - third_party/\n")
	f, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Level == nil || *f.Level != 2 {
		t.Errorf("Level = %v, want 2", f.Level)
	}
	if f.Checks == nil || *f.Checks != "PX3013" {
		t.Errorf("Checks = %v, want PX3013", f.Checks)
	}
	if len(f.Exclude) != 2 || f.Exclude[0] != "vendor/" || f.Exclude[1] != "third_party/" {
		t.Errorf("Exclude = %v, want [vendor/ third_party/]", f.Exclude)
	}
}

func TestLoadPartialLeavesOthersNil(t *testing.T) {
	// Only `checks` set: Level nil (flag default kept), Exclude nil.
	p := write(t, t.TempDir(), "c.yml", "checks: PX1*\n")
	f, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Level != nil {
		t.Errorf("Level = %v, want nil (unspecified)", f.Level)
	}
	if f.Exclude != nil {
		t.Errorf("Exclude = %v, want nil (unspecified)", f.Exclude)
	}
	if f.Checks == nil || *f.Checks != "PX1*" {
		t.Errorf("Checks = %v, want PX1*", f.Checks)
	}
}

func TestLoadEmptyIsNoOp(t *testing.T) {
	for _, content := range []string{"", "   \n", "# just a comment\n"} {
		p := write(t, t.TempDir(), "c.yml", content)
		f, err := Load(p)
		if err != nil {
			t.Fatalf("Load(%q) errored: %v", content, err)
		}
		if f.Level != nil || f.Checks != nil || f.Exclude != nil {
			t.Errorf("Load(%q) = %+v, want all-nil no-op", content, f)
		}
	}
}

func TestLoadUnknownKeyRejected(t *testing.T) {
	p := write(t, t.TempDir(), "c.yml", "levl: 2\n") // typo
	if _, err := Load(p); err == nil {
		t.Error("Load with an unknown key should error (typo protection)")
	}
}

func TestLoadLevelRange(t *testing.T) {
	for _, bad := range []string{"level: 0\n", "level: 4\n", "level: -1\n"} {
		p := write(t, t.TempDir(), "c.yml", bad)
		if _, err := Load(p); err == nil {
			t.Errorf("Load(%q) should reject an out-of-range level", bad)
		}
	}
}

func TestDiscover(t *testing.T) {
	dir := t.TempDir()
	if _, ok := Discover(dir); ok {
		t.Error("Discover found a config in an empty dir")
	}
	write(t, dir, ".perfscanxx.yml", "level: 1\n")
	p, ok := Discover(dir)
	if !ok || filepath.Base(p) != ".perfscanxx.yml" {
		t.Errorf("Discover = (%q, %v), want the .perfscanxx.yml", p, ok)
	}
}

func TestLoadTidyAndExtraArgs(t *testing.T) {
	p := write(t, t.TempDir(), "c.yml", "tidy: /opt/homebrew/opt/llvm/bin/clang-tidy\nextra-args:\n  - -isysroot\n  - /sdk/path\n")
	f, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Tidy == nil || *f.Tidy != "/opt/homebrew/opt/llvm/bin/clang-tidy" {
		t.Errorf("Tidy = %v", f.Tidy)
	}
	if len(f.ExtraArgs) != 2 || f.ExtraArgs[0] != "-isysroot" || f.ExtraArgs[1] != "/sdk/path" {
		t.Errorf("ExtraArgs = %v, want [-isysroot /sdk/path]", f.ExtraArgs)
	}
}

func TestLoadBaselineAndFixErrors(t *testing.T) {
	p := write(t, t.TempDir(), "c.yml", "baseline: .perfscanxx-baseline.yaml\nfix-errors: true\n")
	f, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Baseline == nil || *f.Baseline != ".perfscanxx-baseline.yaml" {
		t.Errorf("Baseline = %v", f.Baseline)
	}
	if f.FixErrors == nil || *f.FixErrors != true {
		t.Errorf("FixErrors = %v, want true", f.FixErrors)
	}
	// omitted -> nil (leave flag default)
	p2 := write(t, t.TempDir(), "c.yml", "level: 1\n")
	f2, _ := Load(p2)
	if f2.Baseline != nil || f2.FixErrors != nil {
		t.Errorf("omitted keys should be nil: %+v", f2)
	}
}

// TestLoadJobsAndTimeout pins the -j / -timeout config keys: jobs round-trips as
// an int, timeout as a Go duration string exposed via TimeoutDuration, and a
// malformed timeout is REJECTED by Load (fail loud, like an out-of-range level)
// rather than silently ignored.
func TestLoadJobsAndTimeout(t *testing.T) {
	p := write(t, t.TempDir(), "c.yml", "jobs: 8\ntimeout: 5m\n")
	f, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Jobs == nil || *f.Jobs != 8 {
		t.Errorf("Jobs = %v, want 8", f.Jobs)
	}
	if d, ok := f.TimeoutDuration(); !ok || d != 5*time.Minute {
		t.Errorf("TimeoutDuration = (%v, %v), want (5m, true)", d, ok)
	}

	// Omitted -> nil / not-set.
	f2, _ := Load(write(t, t.TempDir(), "c.yml", "level: 1\n"))
	if f2.Jobs != nil || f2.Timeout != nil {
		t.Errorf("omitted jobs/timeout should be nil: %+v", f2)
	}
	if _, ok := f2.TimeoutDuration(); ok {
		t.Error("TimeoutDuration on an unset timeout should report false")
	}

	// A malformed duration is a loud error.
	if _, err := Load(write(t, t.TempDir(), "c.yml", "timeout: 5years\n")); err == nil ||
		!strings.Contains(err.Error(), "invalid timeout") {
		t.Errorf("Load with a bad timeout = %v, want an 'invalid timeout' error", err)
	}
}

// TestDiscoverYAMLAndPrecedence pins the parts of Discover that TestDiscover does
// not: the .yaml spelling is found, .yml WINS when both exist (Names order), and
// a DIRECTORY named like a config file is skipped (the !fi.IsDir() guard) so the
// next candidate is still discovered.
func TestDiscoverYAMLAndPrecedence(t *testing.T) {
	// .yaml alone is discovered.
	dyaml := t.TempDir()
	write(t, dyaml, ".perfscanxx.yaml", "level: 1\n")
	if p, ok := Discover(dyaml); !ok || filepath.Base(p) != ".perfscanxx.yaml" {
		t.Errorf("Discover(.yaml only) = (%q, %v), want .perfscanxx.yaml", p, ok)
	}

	// When both exist, .yml wins (it is first in Names).
	dboth := t.TempDir()
	write(t, dboth, ".perfscanxx.yaml", "level: 1\n")
	write(t, dboth, ".perfscanxx.yml", "level: 2\n")
	if p, ok := Discover(dboth); !ok || filepath.Base(p) != ".perfscanxx.yml" {
		t.Errorf("Discover(both) = (%q, %v), want .perfscanxx.yml (precedence)", p, ok)
	}

	// A directory named like a config file must be SKIPPED, and the next
	// candidate found.
	ddir := t.TempDir()
	if err := os.Mkdir(filepath.Join(ddir, ".perfscanxx.yml"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, ddir, ".perfscanxx.yaml", "level: 1\n")
	if p, ok := Discover(ddir); !ok || filepath.Base(p) != ".perfscanxx.yaml" {
		t.Errorf("Discover(dir named .yml) = (%q, %v), want the .yaml file (dir skipped)", p, ok)
	}
}

// TestLoadMissingFile pins that Load surfaces the os.ReadFile error for a
// nonexistent path (rather than silently returning an empty config) so a
// mistyped -config path fails loudly.
func TestLoadMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.yml")
	f, err := Load(missing)
	if err == nil {
		t.Fatalf("Load(%q) = %+v, nil; want an error for a missing file", missing, f)
	}
	if f != nil {
		t.Errorf("Load returned a non-nil config on error: %+v", f)
	}
}
