package config

import (
	"os"
	"path/filepath"
	"testing"
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
