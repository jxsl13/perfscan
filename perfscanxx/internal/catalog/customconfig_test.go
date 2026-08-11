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
}
