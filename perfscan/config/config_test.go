package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToSetAndCompile(t *testing.T) {
	// An empty slice compiles to a nil set (not an empty map) so membership
	// tests are a plain, allocation-free nil-map read.
	if got := toSet(nil); got != nil {
		t.Errorf("toSet(nil) = %v, want nil", got)
	}
	if got := toSet([]string{}); got != nil {
		t.Errorf("toSet(empty) = %v, want nil", got)
	}
	s := toSet([]string{"A", "B", "A"})
	if !s["A"] || !s["B"] || s["C"] {
		t.Errorf("toSet membership wrong: %v", s)
	}

	c := Config{
		ElementAccessors:    []string{"AtF64", "SetF64"},
		AllocatorFuncs:      []string{"Zeros"},
		ElementCountMethods: nil, // stays nil after compile
	}
	sets := c.Compile()
	if !sets.ElementAccessors["AtF64"] || !sets.ElementAccessors["SetF64"] {
		t.Errorf("Compile lost element accessors: %v", sets.ElementAccessors)
	}
	if !sets.AllocatorFuncs["Zeros"] {
		t.Errorf("Compile lost allocator funcs: %v", sets.AllocatorFuncs)
	}
	if sets.ElementCountMethods != nil {
		t.Errorf("empty field must compile to a nil set, got %v", sets.ElementCountMethods)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()

	// Valid YAML round-trips into the typed config.
	good := filepath.Join(dir, "perfscan.yaml")
	os.WriteFile(good, []byte("elementAccessors: [AtF64, SetF64]\nallocatorFuncs: [Zeros]\n"), 0o644)
	c, err := Load(good)
	if err != nil {
		t.Fatalf("Load(valid) error: %v", err)
	}
	if len(c.ElementAccessors) != 2 || c.ElementAccessors[0] != "AtF64" || c.AllocatorFuncs[0] != "Zeros" {
		t.Errorf("Load parsed wrong config: %+v", c)
	}

	// A missing file is an error.
	if _, err := Load(filepath.Join(dir, "nope.yaml")); err == nil {
		t.Error("Load(missing) = nil error, want error")
	}

	// Malformed YAML is an error naming the file.
	bad := filepath.Join(dir, "bad.yaml")
	os.WriteFile(bad, []byte("elementAccessors: [unterminated\n"), 0o644)
	_, err = Load(bad)
	if err == nil || !strings.Contains(err.Error(), "bad.yaml") {
		t.Errorf("Load(malformed) error = %v, want it to name bad.yaml", err)
	}
}

func TestDiscover(t *testing.T) {
	// Layout:
	//   root/               (has go.mod — the module root)
	//     perfscan.yaml     (the config)
	//     a/b/              (a deep package dir)
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// No config anywhere yet -> zero config, empty path (the walk stops at the
	// go.mod-bearing module root).
	if c, p := Discover(deep); p != "" || len(c.ElementAccessors) != 0 {
		t.Errorf("Discover(no config) = %+v,%q want zero,\"\"", c, p)
	}

	// A config in the module root IS found from a deep package dir: the name
	// check runs before the go.mod stop in the same directory.
	cfg := filepath.Join(root, "perfscan.yaml")
	if err := os.WriteFile(cfg, []byte("allocatorFuncs: [Zeros]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, p := Discover(deep)
	if p != cfg {
		t.Fatalf("Discover(with root config) path = %q, want %q", p, cfg)
	}
	if !c.Compile().AllocatorFuncs["Zeros"] {
		t.Errorf("Discover returned a config missing its vocabulary: %+v", c)
	}

	// The go.mod stop prevents escaping the module: a config ABOVE the module
	// root is not discovered. Self-contained layout: base/perfscan.yaml,
	// base/mod/go.mod, base/mod/pkg — discovering from pkg must stop at
	// mod/go.mod and never reach base/perfscan.yaml.
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "perfscan.yaml"), []byte("allocatorFuncs: [Escaped]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(base, "mod", "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "mod", "go.mod"), []byte("module y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if c, p := Discover(pkg); p != "" || len(c.AllocatorFuncs) != 0 {
		t.Errorf("Discover must stop at mod/go.mod, not escape to base/perfscan.yaml; got %+v,%q", c, p)
	}
}

func TestSetForTesting(t *testing.T) {
	restore := SetForTesting(Config{FanOutHelpers: []string{"parallelFor"}})
	if !Current().FanOutHelpers["parallelFor"] {
		t.Error("SetForTesting did not install the vocabulary")
	}
	restore()
	if Current().FanOutHelpers["parallelFor"] {
		t.Error("restore() did not revert the vocabulary")
	}
}
