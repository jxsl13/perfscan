package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jxsl13/perfscan/perfscanxx/internal/tidy"
)

// writeCfgProject drops a .perfscanxx.yml (body), N .cpp TUs and a compile db,
// returning the dir and the file paths.
func writeCfgProject(t *testing.T, body string, names ...string) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".perfscanxx.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var files []string
	var entries []string
	for _, n := range names {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("int x;\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
		entries = append(entries, `{"directory":"`+dir+`","file":"`+p+`","command":"clang++ -std=c++17 -c `+n+`"}`)
	}
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte("["+strings.Join(entries, ",")+"]"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, files
}

// TestConfigJobsPrecedence pins that `jobs:` in the config parallelizes the
// analysis (one clang-tidy invocation per worker) and that a -j flag overrides it.
func TestConfigJobsPrecedence(t *testing.T) {
	origLook, origExec := tidy.LookPath, tidy.Executor
	defer func() { tidy.LookPath, tidy.Executor = origLook, origExec }()
	tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }
	var mu sync.Mutex
	var runs int
	tidy.Executor = func(_ context.Context, argv []string, stdout, _ *bytes.Buffer) (int, error) {
		if len(argv) >= 2 && argv[1] == "--version" {
			stdout.WriteString("LLVM version 22.0.0\n")
			return 0, nil
		}
		mu.Lock()
		runs++
		mu.Unlock()
		for _, a := range argv {
			if strings.HasPrefix(a, "--export-fixes=") {
				_ = os.WriteFile(strings.TrimPrefix(a, "--export-fixes="), nil, 0o644)
			}
		}
		return 0, nil
	}

	dir, files := writeCfgProject(t, "jobs: 3\n", "a.cpp", "b.cpp", "c.cpp")
	cfg := filepath.Join(dir, ".perfscanxx.yml")

	// config jobs:3 over 3 TUs -> 3 parallel invocations.
	runs = 0
	runCLI(append([]string{"-config", cfg, "-p", dir}, files...)...)
	if runs != 3 {
		t.Errorf("config jobs:3 => %d analysis invocations, want 3", runs)
	}
	// -j 1 flag overrides config -> a single invocation.
	runs = 0
	runCLI(append([]string{"-config", cfg, "-j", "1", "-p", dir}, files...)...)
	if runs != 1 {
		t.Errorf("-j 1 must override config jobs:3 => %d invocations, want 1", runs)
	}
}

// TestConfigTimeoutPrecedence pins that `timeout:` in the config bounds the run
// (a hung invocation aborts with the timeout message) — the config->WithTimeout
// wiring resolved AFTER the config merge.
func TestConfigTimeoutPrecedence(t *testing.T) {
	origLook, origExec := tidy.LookPath, tidy.Executor
	defer func() { tidy.LookPath, tidy.Executor = origLook, origExec }()
	tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }
	tidy.Executor = func(ctx context.Context, argv []string, stdout, _ *bytes.Buffer) (int, error) {
		if len(argv) >= 2 && argv[1] == "--version" {
			stdout.WriteString("LLVM version 22.0.0\n")
			return 0, nil
		}
		<-ctx.Done() // hang until the config timeout cancels us
		return -1, ctx.Err()
	}

	dir, files := writeCfgProject(t, "timeout: 20ms\n", "a.cpp")
	cfg := filepath.Join(dir, ".perfscanxx.yml")
	_, errOut, code := runCLI(append([]string{"-config", cfg, "-p", dir}, files...)...)
	if code != 2 || !strings.Contains(errOut, "exceeded -timeout") {
		t.Errorf("config timeout:20ms must abort with the timeout message; code=%d stderr:\n%s", code, errOut)
	}
}
