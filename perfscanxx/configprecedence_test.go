package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/perfscanxx/internal/tidy"
)

// TestConfigPrecedence pins the .perfscanxx.yml precedence contract documented
// on the -config flag ("command-line flags override it"): config values apply
// only when the corresponding flag is NOT set, and a set flag wins. The
// config-package tests cover PARSING; this covers the main.go merge wiring
// (fs.Visit + !set[...]) end to end, by capturing the --checks selection the
// orchestrator hands clang-tidy. A regression (config always winning, or never
// applied) would silently run the wrong checks.
func TestConfigPrecedence(t *testing.T) {
	// run writes a .perfscanxx.yml (cfgBody), stubs clang-tidy to capture the
	// effective check SELECTION, and returns it. The selection travels either as
	// a --checks= argument (built-in-only sets) or, when custom checks are
	// included, inside the generated --config-file's Checks line — so capture
	// both to stay robust across level tiers.
	run := func(t *testing.T, cfgBody string, extra ...string) string {
		t.Helper()
		dir := t.TempDir()
		cfg := filepath.Join(dir, ".perfscanxx.yml")
		if err := os.WriteFile(cfg, []byte(cfgBody), 0o644); err != nil {
			t.Fatal(err)
		}
		src := filepath.Join(dir, "main.cpp")
		if err := os.WriteFile(src, []byte("int f(){return 0;}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cc := `[{"directory":"` + dir + `","file":"` + src + `","command":"clang++ -std=c++17 -c ` + src + `"}]`
		if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
			t.Fatal(err)
		}

		var gotSelection string
		origLook, origExec := tidy.LookPath, tidy.Executor
		tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }
		tidy.Executor = func(_ context.Context, argv []string, _, _ *bytes.Buffer) (int, error) {
			for _, a := range argv {
				if strings.HasPrefix(a, "--checks=") {
					gotSelection += a
				}
				if strings.HasPrefix(a, "--config-file=") {
					if b, err := os.ReadFile(strings.TrimPrefix(a, "--config-file=")); err == nil {
						gotSelection += string(b)
					}
				}
				if strings.HasPrefix(a, "--export-fixes=") {
					_ = os.WriteFile(strings.TrimPrefix(a, "--export-fixes="), nil, 0o644)
				}
			}
			return 0, nil
		}
		defer func() { tidy.LookPath, tidy.Executor = origLook, origExec }()

		args := append([]string{"-config", cfg, "-p", dir}, extra...)
		args = append(args, src)
		runCLI(args...)
		return gotSelection
	}

	// checks: applied from config when there is no -checks flag...
	if got := run(t, "checks: PX1001\n"); !strings.Contains(got, "performance-for-range-copy") || strings.Contains(got, "performance-avoid-endl") {
		t.Errorf("config checks:PX1001 not applied to the clang-tidy selection: %q", got)
	}
	// ...but a -checks flag overrides it.
	if got := run(t, "checks: PX1001\n", "-checks", "PX3003"); !strings.Contains(got, "performance-avoid-endl") || strings.Contains(got, "performance-for-range-copy") {
		t.Errorf("CLI -checks PX3003 must override config checks:PX1001, got: %q", got)
	}

	// level: config level 1 excludes the L3 check performance-no-int-to-ptr...
	if got := run(t, "level: 1\n"); strings.Contains(got, "performance-no-int-to-ptr") {
		t.Errorf("config level:1 must exclude the L3 check performance-no-int-to-ptr, got: %q", got)
	}
	// ...but -level 3 overrides config and restores it.
	if got := run(t, "level: 1\n", "-level", "3"); !strings.Contains(got, "performance-no-int-to-ptr") {
		t.Errorf("CLI -level 3 must override config level:1 (L3 check present), got: %q", got)
	}
}
