package plugin

import (
	"testing"

	"github.com/jxsl13/perfscan/checks"
	"golang.org/x/tools/go/analysis"
)

func buildAnalyzers(t *testing.T, conf any) []*analysis.Analyzer {
	t.Helper()
	p, err := New(conf)
	if err != nil {
		t.Fatalf("New(%v): %v", conf, err)
	}
	az, err := p.BuildAnalyzers()
	if err != nil {
		t.Fatalf("BuildAnalyzers: %v", err)
	}
	return az
}

// TestPluginExposesEveryCheck guards against plugin↔registry drift: the plugin
// must surface EVERY check from the perfscan catalog (at the default aggressive
// level), by name. If a check is added to the registry but the plugin stops
// wiring the full catalog, this fails.
func TestPluginExposesEveryCheck(t *testing.T) {
	az := buildAnalyzers(t, nil) // nil settings -> MaxLevel defaults to aggressive (all)
	all := checks.All()
	if len(az) != len(all) {
		t.Fatalf("plugin exposed %d analyzers at the default level, want all %d catalog checks",
			len(az), len(all))
	}
	present := make(map[string]bool, len(az))
	for _, a := range az {
		present[a.Name] = true
	}
	for _, c := range all {
		if c.Analyzer == nil {
			t.Errorf("catalog check %s has a nil Analyzer", c.ID)
			continue
		}
		if !present[c.Analyzer.Name] {
			t.Errorf("plugin does not expose an analyzer for check %s (%s)", c.ID, c.Analyzer.Name)
		}
	}
}

// TestPluginLevelGating: -level 1 (via maxLevel setting) must expose a strict,
// non-empty subset of the catalog — the plugin honors the same level knob the CLI does.
func TestPluginLevelGating(t *testing.T) {
	l1 := buildAnalyzers(t, map[string]any{"maxLevel": 1})
	all := checks.All()
	if len(l1) == 0 || len(l1) >= len(all) {
		t.Fatalf("maxLevel=1 exposed %d analyzers, want a strict non-empty subset of %d", len(l1), len(all))
	}
}
