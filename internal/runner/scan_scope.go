package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/jxsl13/perfscan/internal/scanscope"
	"golang.org/x/tools/go/packages"
)

// Strict loader evidence is needed only by an enabled usable pinned policy.
// Ordinary scans retain the original package loader without target subprocesses.
func loadForChecks(opts *Options, enabled []*lint.Check, contracts []config.SingleUseQuantizationContract, ordinary func(*Options) ([]*packages.Package, error), strict func(*Options) ([]*packages.Package, *scanscope.Target, error)) ([]*packages.Package, *scanscope.Target, error) {
	active := false
	for _, check := range enabled {
		active = active || check.ID == "PS6141"
	}
	claims := map[string]int{}
	for i := range contracts {
		c := &contracts[i]
		claims[c.Quantizer+"\x00"+c.Consumer]++
	}
	if active {
		for i := range contracts {
			c := &contracts[i]
			if c.Valid() && len(c.BenchmarkExemptions) > 0 && claims[c.Quantizer+"\x00"+c.Consumer] == 1 {
				return strict(opts)
			}
		}
	}
	pkgs, err := ordinary(opts)
	return pkgs, nil, err
}

func strictLoadTarget(dir string, env []string) (scanscope.Target, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "env", "-json", "GOVERSION", "GOOS", "GOARCH")
	command.Dir = dir
	command.Env = env
	data, err := command.Output()
	if err != nil {
		return scanscope.Target{}, err
	}
	var result struct{ GOVERSION, GOOS, GOARCH string }
	if err := json.Unmarshal(data, &result); err != nil {
		return scanscope.Target{}, err
	}
	sum := sha256.Sum256([]byte(dir + "\x00" + strings.Join(env, "\x00")))
	return scanscope.Target{GoVersion: result.GOVERSION, GOOS: result.GOOS, GOARCH: result.GOARCH, ContextSHA256: hex.EncodeToString(sum[:])}, nil
}

// One captured context is used for both probes and the package load. Failed or
// differing observations leave scope unknown without changing load semantics.
func loadWithScope(opts *Options) ([]*packages.Package, *scanscope.Target, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}
	env := os.Environ()
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedTypesSizes | packages.NeedImports | packages.NeedDeps | packages.NeedEmbedFiles | packages.NeedModule, Tests: opts.Tests, Dir: dir, Env: env}
	return loadScopeContext(cfg, opts.Patterns, strictLoadTarget, packages.Load)
}

func loadScopeContext(cfg *packages.Config, patterns []string, probe func(string, []string) (scanscope.Target, error), loader func(*packages.Config, ...string) ([]*packages.Package, error)) ([]*packages.Package, *scanscope.Target, error) {
	return loadScopeContextWithDriver(cfg, patterns, probe, loader, func(env []string) bool {
		return builtInGoDriver(env, exec.LookPath)
	})
}

// Match go/packages v0.48's driver selection without forcing a different
// loader. Automatic lookup uses the process PATH, just as its external.go does.
func builtInGoDriver(env []string, lookPath func(string) (string, error)) bool {
	tool := ""
	for _, entry := range env {
		if value, ok := strings.CutPrefix(entry, "GOPACKAGESDRIVER="); ok {
			tool = value
		}
	}
	if tool == "off" {
		return true
	}
	if tool != "" {
		return false
	}
	_, err := lookPath("gopackagesdriver")
	return errors.Is(err, exec.ErrNotFound)
}

func loadScopeContextWithDriver(cfg *packages.Config, patterns []string, probe func(string, []string) (scanscope.Target, error), loader func(*packages.Config, ...string) ([]*packages.Package, error), builtIn func([]string) bool) ([]*packages.Package, *scanscope.Target, error) {
	beforeDriver := builtIn(cfg.Env)
	before, beforeErr := probe(cfg.Dir, cfg.Env)
	pkgs, err := loader(cfg, patterns...)
	if err != nil {
		return pkgs, nil, err
	}
	after, afterErr := probe(cfg.Dir, cfg.Env)
	afterDriver := builtIn(cfg.Env)
	if !beforeDriver || !afterDriver || beforeErr != nil || afterErr != nil || !before.Known() || before != after {
		return pkgs, nil, nil
	}
	return pkgs, &before, nil
}
