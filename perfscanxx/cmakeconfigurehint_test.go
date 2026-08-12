package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/perfscanxx/internal/cmake"
)

// TestCMakeConfigureFailureHint pins that when `-cmake` auto-configure fails
// (common: the project's default targets need optional deps a bare configure
// can't satisfy — e.g. leveldb's benchmarks want sqlite3), perfscanxx exits 2
// with an ACTIONABLE hint pointing at the manual `-p <build-dir>` path, instead
// of leaving the user with only a raw CMake error. Hermetic: cmake is injected
// (Available + a failing Runner), so no real cmake is needed.
func TestCMakeConfigureFailureHint(t *testing.T) {
	origAvail, origRunner := cmake.Available, cmake.Runner
	defer func() { cmake.Available, cmake.Runner = origAvail, origRunner }()
	cmake.Available = func() bool { return true }
	cmake.Runner = func(_ context.Context, _ string, _ []string) ([]byte, error) {
		return []byte("CMake Error: could not find package Sqlite3"), fmt.Errorf("exit status 1")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CMakeLists.txt"), []byte("cmake_minimum_required(VERSION 3.10)\nproject(x)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI("-cmake", ".")
	if code != 2 {
		t.Errorf("configure failure: exit %d, want 2; stderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "configure step failed") {
		t.Errorf("expected the configure-failure explanation; stderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "-p <build-dir>") || !strings.Contains(stderr, "BUILD_TESTING=OFF") {
		t.Errorf("expected the actionable manual-configure hint (-p, BUILD_TESTING=OFF); stderr:\n%s", stderr)
	}
	// The raw CMake error must still be shown so the real cause isn't hidden.
	if !strings.Contains(stderr, "Sqlite3") {
		t.Errorf("expected the underlying cmake error to be preserved; stderr:\n%s", stderr)
	}
}
