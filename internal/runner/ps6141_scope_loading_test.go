package runner

import (
	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/scanscope"
	"github.com/jxsl13/perfscan/lint"
	"golang.org/x/tools/go/packages"
	"strings"
	"testing"
)

func TestPS6141ConditionalStrictScopeLoading(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"noRule", "noVocabulary", "noExemption", "ambiguous", "invalidContract", "applicablePolicy", "changedTarget"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := config.SingleUseQuantizationContract{Quantizer: "fixture.pack", Consumer: "fixture.dot", ConsumerForm: "twoInputDot", PackedArgument: 0, WeightArgument: 1, RowsArgument: -1, QuantizationAndDotMeaningReviewed: true, BenchmarkExemptions: []config.SingleUseQuantizationExemption{{Policy: "policy.json", SHA256: strings.Repeat("a", 64)}}}
			contracts := []config.SingleUseQuantizationContract{c}
			enabled := []*lint.Check{{ID: "PS6141"}}
			switch name {
			case "noRule":
				enabled = []*lint.Check{{ID: "PS2001"}}
			case "noVocabulary":
				contracts = nil
			case "noExemption":
				contracts[0].BenchmarkExemptions = nil
			case "ambiguous":
				contracts = append(contracts, c)
			case "invalidContract":
				contracts[0].QuantizationAndDotMeaningReviewed = false
			}
			opts := &Options{Patterns: []string{"fixture"}, Tests: true}
			pkg := &packages.Package{ID: "fixture"}
			ordinaryCalls, strictCalls, probes, loads := 0, 0, 0, 0
			target := scanscope.Target{GoVersion: "go1.26.0", GOOS: "linux", GOARCH: "arm64", ContextSHA256: strings.Repeat("b", 64)}
			ordinary := func(got *Options) ([]*packages.Package, error) {
				ordinaryCalls++
				if got != opts {
					t.Fatal("ordinary load options changed")
				}
				return []*packages.Package{pkg}, nil
			}
			strict := func(got *Options) ([]*packages.Package, *scanscope.Target, error) {
				strictCalls++
				if got != opts {
					t.Fatal("strict load options changed")
				}
				cfg := &packages.Config{Dir: "captured", Env: []string{"GOPACKAGESDRIVER=off"}}
				return loadScopeContextWithDriver(cfg, opts.Patterns, func(dir string, env []string) (scanscope.Target, error) {
					probes++
					if dir != cfg.Dir || len(env) != 1 || env[0] != cfg.Env[0] {
						t.Fatal("strict captured context changed")
					}
					current := target
					if name == "changedTarget" && probes == 2 {
						current.GOARCH = "amd64"
					}
					return current, nil
				}, func(got *packages.Config, patterns ...string) ([]*packages.Package, error) {
					loads++
					if got != cfg || len(patterns) != 1 || patterns[0] != "fixture" {
						t.Fatal("strict package load changed")
					}
					return []*packages.Package{pkg}, nil
				}, func([]string) bool { return true })
			}
			pkgs, scope, err := loadForChecks(opts, enabled, contracts, ordinary, strict)
			if err != nil || len(pkgs) != 1 || pkgs[0] != pkg {
				t.Fatalf("load result=%v error=%v", pkgs, err)
			}
			wantStrict := name == "applicablePolicy" || name == "changedTarget"
			if wantStrict {
				if ordinaryCalls != 0 || strictCalls != 1 || probes != 2 || loads != 1 {
					t.Fatalf("strict calls ordinary=%d strict=%d probes=%d loads=%d", ordinaryCalls, strictCalls, probes, loads)
				}
				if (scope != nil) != (name == "applicablePolicy") {
					t.Fatalf("strict scope=%v", scope)
				}
			} else if ordinaryCalls != 1 || strictCalls != 0 || probes != 0 || loads != 0 || scope != nil {
				t.Fatalf("ordinary path unexpectedly probed: ordinary=%d strict=%d probes=%d loads=%d scope=%v", ordinaryCalls, strictCalls, probes, loads, scope)
			}
		})
	}
}
