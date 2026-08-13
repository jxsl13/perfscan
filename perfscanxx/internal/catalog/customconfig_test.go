package catalog

import "strings"

import "testing"

func TestClangTidyConfigCustom(t *testing.T) {
	sel := Select("PX2101", LevelAggressive) // the custom reserve-before-loop
	if len(sel) != 1 || !sel[0].Custom {
		t.Fatalf("expected 1 custom entry, got %v", sel)
	}
	if !AnyCustom(sel) {
		t.Fatal("AnyCustom = false for a custom selection")
	}
	cfg := ClangTidyConfig(sel)
	for _, want := range []string{"custom-reserve-before-loop", "CustomChecks:", "Name: reserve-before-loop", "Query: |", "match cxxMemberCallExpr", "BindName: grow", "Level: Warning"} {
		if !strings.Contains(cfg, want) {
			t.Errorf("config missing %q:\n%s", want, cfg)
		}
	}
}

func TestClangTidyConfigNoCustom(t *testing.T) {
	sel := Select("PX1001", LevelIdiomatic)
	if AnyCustom(sel) {
		t.Fatal("AnyCustom = true for a builtin-only selection")
	}
	// A builtin-only selection must emit the Checks line and NOTHING else — no
	// CustomChecks section (the early-return path in ClangTidyConfig). Emitting
	// an empty CustomChecks would make clang-tidy reject the config.
	cfg := ClangTidyConfig(sel)
	if !strings.HasPrefix(cfg, "Checks: '") {
		t.Errorf("config should start with the Checks line, got:\n%s", cfg)
	}
	if strings.Contains(cfg, "CustomChecks:") {
		t.Errorf("builtin-only selection must not emit a CustomChecks section:\n%s", cfg)
	}
}
