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

// TestCMakeNoProjectFound pins the -cmake path when no CMakeLists.txt exists
// walking up from the cwd: perfscanxx reports it plainly (and then falls through
// to the normal no-database handling) rather than silently doing nothing.
func TestCMakeNoProjectFound(t *testing.T) {
	origAvail := cmake.Available
	defer func() { cmake.Available = origAvail }()
	cmake.Available = func() bool { return true }

	dir := t.TempDir() // no CMakeLists.txt anywhere under here
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	_, stderr, _ := runCLI("-cmake", ".")
	if !strings.Contains(stderr, "no CMakeLists.txt found") {
		t.Errorf("expected the 'no CMakeLists.txt found' notice; stderr:\n%s", stderr)
	}
}

// TestCMakeBuildFailureIsNonFatal pins the -cmake-build path when configure
// SUCCEEDS (writes a compile_commands.json) but the subsequent `cmake --build`
// FAILS: perfscanxx must WARN and continue with the configured database (some
// TUs may not parse) rather than abort — a failed header-generating build is
// recoverable, unlike a failed configure. The injected Runner writes the compdb
// on the configure call and errors on the build call.
func TestCMakeBuildFailureIsNonFatal(t *testing.T) {
	origAvail, origRunner := cmake.Available, cmake.Runner
	defer func() { cmake.Available, cmake.Runner = origAvail, origRunner }()
	cmake.Available = func() bool { return true }

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CMakeLists.txt"), []byte("cmake_minimum_required(VERSION 3.10)\nproject(x)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	build := filepath.Join(dir, "build")
	cmake.Runner = func(_ context.Context, _ string, argv []string) ([]byte, error) {
		isBuild := false
		for _, a := range argv {
			if a == "--build" {
				isBuild = true
			}
		}
		if isBuild {
			return []byte("ninja: build stopped: missing header"), fmt.Errorf("exit status 1")
		}
		// configure: materialize an (empty-array) compile_commands.json so the
		// bootstrap proceeds to the build step.
		if err := os.MkdirAll(build, 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(build, "compile_commands.json"), []byte("[]"), 0o644); err != nil {
			return nil, err
		}
		return []byte("-- Configuring done"), nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	_, stderr, _ := runCLI("-cmake-build", ".")
	if !strings.Contains(stderr, "cmake --build") {
		t.Errorf("expected the build-step notice; stderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "warning:") || !strings.Contains(stderr, "continuing with the configured database") {
		t.Errorf("a failed cmake --build must warn and continue, not abort; stderr:\n%s", stderr)
	}
}
