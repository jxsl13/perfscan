package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToSetAndCompile(t *testing.T) {
	t.Parallel()
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
		CacheLineBytes:           128,
		ElementAccessors:         []string{"AtF64", "SetF64"},
		AllocatorFuncs:           []string{"Zeros"},
		SelectorPromotionSymbols: []string{"selectKernel"},
		InPlaceFusionContracts: []InPlaceFusionContract{{
			Name: "swiglu",
		}},
		ElementCountMethods: nil, // stays nil after compile
	}
	sets := c.Compile()
	if sets.CacheLineBytes != 128 {
		t.Errorf("Compile lost cache line size: %d", sets.CacheLineBytes)
	}
	if !sets.ElementAccessors["AtF64"] || !sets.ElementAccessors["SetF64"] {
		t.Errorf("Compile lost element accessors: %v", sets.ElementAccessors)
	}
	if !sets.AllocatorFuncs["Zeros"] {
		t.Errorf("Compile lost allocator funcs: %v", sets.AllocatorFuncs)
	}
	if !sets.SelectorPromotionSymbols["selectKernel"] {
		t.Errorf("Compile lost selector promotion symbols: %v", sets.SelectorPromotionSymbols)
	}
	if len(sets.InPlaceFusionContracts) != 1 || sets.InPlaceFusionContracts[0].Name != "swiglu" {
		t.Errorf("Compile lost in-place fusion contracts: %+v", sets.InPlaceFusionContracts)
	}
	c.InPlaceFusionContracts[0].Name = "mutated"
	if sets.InPlaceFusionContracts[0].Name != "swiglu" {
		t.Error("Compile must clone in-place fusion contracts")
	}
	if sets.ElementCountMethods != nil {
		t.Errorf("empty field must compile to a nil set, got %v", sets.ElementCountMethods)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()

	// Valid YAML round-trips into the typed config.
	good := filepath.Join(dir, "perfscan.yaml")
	os.WriteFile(good, []byte("cacheLineBytes: 128\nelementAccessors: [AtF64, SetF64]\nallocatorFuncs: [Zeros]\n"), 0o644)
	c, err := Load(good)
	if err != nil {
		t.Fatalf("Load(valid) error: %v", err)
	}
	if c.CacheLineBytes != 128 || len(c.ElementAccessors) != 2 || c.ElementAccessors[0] != "AtF64" || c.AllocatorFuncs[0] != "Zeros" {
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
	restore := SetForTesting(Config{CacheLineBytes: 128, FanOutHelpers: []string{"parallelFor"}})
	if Current().CacheLineBytes != 128 {
		t.Error("SetForTesting did not install numeric tuning")
	}
	if !Current().FanOutHelpers["parallelFor"] {
		t.Error("SetForTesting did not install the vocabulary")
	}
	restore()
	if Current().FanOutHelpers["parallelFor"] {
		t.Error("restore() did not revert the vocabulary")
	}
}

func TestUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "perfscan.yaml")

	// elementAccesors (missing an 's') and bogus are unknown; the two correctly
	// spelled keys are not. Result is sorted.
	os.WriteFile(p, []byte("elementAccessors: [A]\nelementAccesors: [B]\nbogus: 1\nallocatorFuncs: [Z]\n"), 0o644)
	got := UnknownKeys(p)
	if len(got) != 2 || got[0] != "bogus" || got[1] != "elementAccesors" {
		t.Errorf("UnknownKeys = %v, want [bogus elementAccesors]", got)
	}

	// All-known config -> no unknowns.
	os.WriteFile(p, []byte("_comment: metadata\nelementAccessors: [A]\nfanOutHelpers: [F]\n"), 0o644)
	if got := UnknownKeys(p); len(got) != 0 {
		t.Errorf("UnknownKeys(all known) = %v, want none", got)
	}

	// Missing file and a non-mapping YAML document are best-effort nil.
	if got := UnknownKeys(filepath.Join(dir, "nope.yaml")); got != nil {
		t.Errorf("UnknownKeys(missing) = %v, want nil", got)
	}
	seq := filepath.Join(dir, "seq.yaml")
	os.WriteFile(seq, []byte("- a\n- b\n"), 0o644)
	if got := UnknownKeys(seq); got != nil {
		t.Errorf("UnknownKeys(non-mapping) = %v, want nil", got)
	}
}

func TestGoAICurrentVocabularyCompatibility(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "perfscan.json")
	data := []byte(`{
  "_comment": "schema compatibility fixture",
	"cacheLineBytes": 128,
  "pureComputeFuncs": ["forward"],
  "layoutOpConstants": ["OpSlice", "OpConcat"],
  "pointerTypeNames": ["Tensor", "Storage"],
  "variadicDispatchWrappers": ["exec", "visExecN"],
  "topKSelectorFuncs": ["topKIndices"],
  "inputViewFuncs": ["f64Data", "f32Data"],
  "outputViewFuncs": ["outF64", "outF32"],
  "referenceBackendPkg": "ref",
  "optimizedBackendPkgs": ["cpu"],
  "kernelRegisterFuncs": ["add"]
}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if unknown := UnknownKeys(path); len(unknown) != 0 {
		t.Fatalf("current GoAI vocabulary has unknown keys: %v", unknown)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(current GoAI vocabulary): %v", err)
	}
	if cfg.Comment == "" || cfg.CacheLineBytes != 128 || cfg.ReferenceBackendPkg != "ref" ||
		len(cfg.PureComputeFuncs) != 1 || len(cfg.LayoutOpConstants) != 2 ||
		len(cfg.PointerTypeNames) != 2 || len(cfg.VariadicDispatchWrappers) != 2 ||
		len(cfg.TopKSelectorFuncs) != 1 || len(cfg.InputViewFuncs) != 2 ||
		len(cfg.OutputViewFuncs) != 2 || len(cfg.OptimizedBackendPkgs) != 1 ||
		len(cfg.KernelRegisterFuncs) != 1 {
		t.Fatalf("current GoAI vocabulary did not round-trip: %+v", cfg)
	}
	sets := cfg.Compile()
	if sets.CacheLineBytes != 128 || !sets.PureComputeFuncs["forward"] || !sets.LayoutOpConstants["OpSlice"] ||
		!sets.PointerTypeNames["Tensor"] || !sets.VariadicDispatchWrappers["exec"] ||
		!sets.TopKSelectorFuncs["topKIndices"] || !sets.InputViewFuncs["f64Data"] ||
		!sets.OutputViewFuncs["outF64"] || sets.ReferenceBackendPkg != "ref" ||
		!sets.OptimizedBackendPkgs["cpu"] || !sets.KernelRegisterFuncs["add"] {
		t.Fatalf("Compile lost current GoAI vocabulary: %+v", sets)
	}
}

// TestExampleConfigIsValidAndGeneric pins the shipped template: it must parse,
// contain NO unknown keys (so it never drifts back to stale/renamed fields), and
// populate every documented field so it stays a complete, working reference.
func TestExampleConfigIsValidAndGeneric(t *testing.T) {
	t.Parallel()
	const path = "../docs/perfscan.example.yaml"
	if unk := UnknownKeys(path); len(unk) != 0 {
		t.Errorf("example config has unknown keys %v — every key must map to a Config field", unk)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load(example): %v", err)
	}
	fields := map[string]int{
		"ElementAccessors": len(c.ElementAccessors), "FastPathHelpers": len(c.FastPathHelpers),
		"SelectorPromotionSymbols": len(c.SelectorPromotionSymbols),
		"ElementCountMethods":      len(c.ElementCountMethods), "ShapeMethods": len(c.ShapeMethods),
		"IndexDecomposeFuncs": len(c.IndexDecomposeFuncs), "AllocatorFuncs": len(c.AllocatorFuncs),
		"PerElementVisitors": len(c.PerElementVisitors), "BulkCopyHelpers": len(c.BulkCopyHelpers),
		"VectorizedSiblingFuncs": len(c.VectorizedSiblingFuncs), "FanOutHelpers": len(c.FanOutHelpers),
		"DtypeMethods": len(c.DtypeMethods), "OutputBufferElemTypes": len(c.OutputBufferElemTypes),
		"CompiledResourceFuncs": len(c.CompiledResourceFuncs), "GPUReductionKernels": len(c.GPUReductionKernels),
		"PureComputeFuncs": len(c.PureComputeFuncs), "LayoutOpConstants": len(c.LayoutOpConstants),
		"PointerTypeNames": len(c.PointerTypeNames), "VariadicDispatchWrappers": len(c.VariadicDispatchWrappers),
		"TopKSelectorFuncs": len(c.TopKSelectorFuncs), "InputViewFuncs": len(c.InputViewFuncs),
		"OutputViewFuncs": len(c.OutputViewFuncs), "OptimizedBackendPkgs": len(c.OptimizedBackendPkgs),
		"KernelRegisterFuncs":    len(c.KernelRegisterFuncs),
		"InPlaceFusionContracts": len(c.InPlaceFusionContracts),
	}
	for name, n := range fields {
		if n == 0 {
			t.Errorf("example config leaves %s empty; the template should exercise every field", name)
		}
	}
	if c.Comment == "" || c.ReferenceBackendPkg == "" {
		t.Errorf("example config must populate _comment and referenceBackendPkg")
	}
	if c.CacheLineBytes <= 0 {
		t.Errorf("example config must populate cacheLineBytes")
	}
}
