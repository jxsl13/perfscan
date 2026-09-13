package runner

import (
	"errors"
	"go/ast"
	"go/token"
	"go/types"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/internal/scanscope"
	"github.com/jxsl13/perfscan/lint"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"
)

func TestStrictLoadScopeBarrier(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"exact", "differentTarget", "differentVersion", "missing", "failedProbe", "failedLoad"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg := &packages.Config{Dir: t.TempDir(), Env: []string{"GOOS=linux", "GOARCH=arm64", "GOPACKAGESDRIVER=off"}}
			target := scanscope.Target{GoVersion: "go1.26.0", GOOS: "linux", GOARCH: "arm64", ContextSHA256: strings.Repeat("a", 64)}
			calls := 0
			probe := func(dir string, env []string) (scanscope.Target, error) {
				if dir != cfg.Dir || !reflect.DeepEqual(env, cfg.Env) {
					t.Fatal("probe did not use captured loader context")
				}
				calls++
				current := target
				if calls == 2 {
					switch name {
					case "differentTarget":
						current.GOARCH = "amd64"
					case "differentVersion":
						current.GoVersion = "go1.25.0"
					case "missing":
						current.GOOS = ""
					case "failedProbe":
						return current, errors.New("unavailable")
					}
				}
				return current, nil
			}
			pkg := &packages.Package{ID: "fixture"}
			loader := func(got *packages.Config, patterns ...string) ([]*packages.Package, error) {
				if calls != 1 || got != cfg || !reflect.DeepEqual(patterns, []string{"fixture"}) {
					t.Fatal("load did not run inside observation barrier")
				}
				if name == "failedLoad" {
					return nil, errors.New("load failed")
				}
				return []*packages.Package{pkg}, nil
			}
			pkgs, scope, err := loadScopeContext(cfg, []string{"fixture"}, probe, loader)
			if name == "failedLoad" {
				if err == nil || scope != nil {
					t.Fatal("failed load qualified")
				}
				return
			}
			if err != nil || len(pkgs) != 1 || pkgs[0] != pkg || calls != 2 {
				t.Fatalf("load outcome lost: %v", err)
			}
			if (scope != nil) != (name == "exact") {
				t.Fatalf("scope=%v", scope)
			}
		})
	}
}

func TestStrictLoadScopeExternalDriver(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		env       []string
		path      string
		lookupErr error
		known     bool
	}{
		{"explicitDriver", []string{"GOPACKAGESDRIVER=custom-driver"}, "", exec.ErrNotFound, false},
		{"automaticDriver", nil, "/synthetic/gopackagesdriver", nil, false},
		{"lookupUncertain", nil, "", errors.New("lookup failed"), false},
		{"explicitOff", []string{"GOPACKAGESDRIVER=off"}, "/synthetic/gopackagesdriver", nil, true},
		{"defaultGo", nil, "", exec.ErrNotFound, true},
		{"emptyUsesAutomatic", []string{"GOPACKAGESDRIVER="}, "/synthetic/gopackagesdriver", nil, false},
		{"lastDriverWins", []string{"GOPACKAGESDRIVER=off", "GOPACKAGESDRIVER=custom-driver"}, "", exec.ErrNotFound, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &packages.Config{Dir: "synthetic-context", Env: tc.env}
			target := scanscope.Target{GoVersion: "go1.26.0", GOOS: "linux", GOARCH: "arm64", ContextSHA256: strings.Repeat("a", 64)}
			probe := func(string, []string) (scanscope.Target, error) { return target, nil }
			loaded := false
			loader := func(got *packages.Config, patterns ...string) ([]*packages.Package, error) {
				loaded = true
				if got != cfg || !reflect.DeepEqual(got.Env, tc.env) || !reflect.DeepEqual(patterns, []string{"fixture"}) {
					t.Fatal("driver screen changed package-load context")
				}
				return []*packages.Package{{ID: "fixture"}}, nil
			}
			builtIn := func(env []string) bool {
				return builtInGoDriver(env, func(name string) (string, error) {
					if name != "gopackagesdriver" {
						t.Fatal("unexpected lookup")
					}
					return tc.path, tc.lookupErr
				})
			}
			pkgs, scope, err := loadScopeContextWithDriver(cfg, []string{"fixture"}, probe, loader, builtIn)
			if err != nil || !loaded || len(pkgs) != 1 || (scope != nil) != tc.known {
				t.Fatalf("loaded=%v scope=%v error=%v", loaded, scope, err)
			}
		})
	}
}

func TestStrictLoadScopeDriverDiscoveryTransitions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		paths [2]string
	}{
		{"driverBeforeOnly", [2]string{"/synthetic/gopackagesdriver", ""}},
		{"driverAfterOnly", [2]string{"", "/synthetic/gopackagesdriver"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &packages.Config{Dir: "synthetic-context"}
			target := scanscope.Target{GoVersion: "go1.26.0", GOOS: "linux", GOARCH: "arm64", ContextSHA256: strings.Repeat("a", 64)}
			probes, lookups, loads := 0, 0, 0
			probe := func(string, []string) (scanscope.Target, error) {
				probes++
				return target, nil
			}
			builtIn := func(env []string) bool {
				return builtInGoDriver(env, func(name string) (string, error) {
					if name != "gopackagesdriver" || lookups >= len(tc.paths) {
						t.Fatal("unexpected discovery lookup")
					}
					path := tc.paths[lookups]
					lookups++
					if path == "" {
						return "", exec.ErrNotFound
					}
					return path, nil
				})
			}
			loader := func(got *packages.Config, patterns ...string) ([]*packages.Package, error) {
				loads++
				if got != cfg || !reflect.DeepEqual(patterns, []string{"fixture"}) {
					t.Fatal("driver transition screen changed actual load")
				}
				return []*packages.Package{{ID: "fixture"}}, nil
			}
			pkgs, scope, err := loadScopeContextWithDriver(cfg, []string{"fixture"}, probe, loader, builtIn)
			if err != nil || len(pkgs) != 1 || scope != nil || probes != 2 || lookups != 2 || loads != 1 {
				t.Fatalf("scope=%v probes=%d lookups=%d loads=%d error=%v", scope, probes, lookups, loads, err)
			}
		})
	}
}

func TestLegacyFactCheckKeepsUnknownScope(t *testing.T) {
	t.Parallel()
	calls := 0
	check := &lint.Check{ID: "scopeFact", Analyzer: &analysis.Analyzer{Name: "scopeFact", Doc: "test", Run: func(pass *analysis.Pass) (any, error) {
		calls++
		if _, known := scanscope.Get(pass); known {
			t.Fatal("legacy fact wrapper invented scope")
		}
		return nil, nil
	}}}
	pkg := &packages.Package{PkgPath: "fixture", Fset: token.NewFileSet(), Types: types.NewPackage("fixture", "fixture"), Syntax: []*ast.File{{Name: ast.NewIdent("fixture")}}}
	if _, err := runFactCheck(check, []*packages.Package{pkg}); err != nil || calls != 1 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}

func TestLegacyLoadKeepsPackageInventory(t *testing.T) {
	t.Parallel()
	pkgs, err := load(&Options{Patterns: []string{"../testparallel/testdata/racefixture"}})
	if err != nil || len(pkgs) != 1 {
		t.Fatalf("packages=%d error=%v", len(pkgs), err)
	}
	if len(pkgs[0].Errors) != 0 || pkgs[0].Name != "racefixture" {
		t.Fatalf("load changed: %+v", pkgs[0].Errors)
	}
}

func TestRunCheckStrictScopeAndLegacyUnknown(t *testing.T) {
	t.Parallel()
	for _, known := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacyUnknown", true: "observed"}[known], func(t *testing.T) {
			t.Parallel()
			target := scanscope.Target{GoVersion: "go1.26.0", GOOS: "windows", GOARCH: "amd64", ContextSHA256: strings.Repeat("b", 64)}
			check := &lint.Check{ID: "scopeTest", Analyzer: &analysis.Analyzer{Name: "scopeTest", Doc: "test", Run: func(pass *analysis.Pass) (any, error) {
				got, ok := scanscope.Get(pass)
				if ok != known || known && got != target {
					t.Fatalf("scope=%v known=%v", got, ok)
				}
				return nil, nil
			}}}
			pkg := &packages.Package{PkgPath: "fixture", Fset: token.NewFileSet(), Types: types.NewPackage("fixture", "fixture")}
			var err error
			if known {
				_, err = runCheckWithScope(check, pkg, nil, &target)
			} else {
				_, err = runCheck(check, pkg, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
