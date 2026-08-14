// Package cmake optionally bootstraps a compilation database (and, on request,
// generated headers) for a CMake project, so `perfscanxx -cmake ./...` can work
// without a pre-existing build. Running CMake and a build executes the
// project's build scripts, so callers gate this behind an explicit flag.
package cmake

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ErrNoDatabaseProduced is returned by Configure when cmake exits successfully
// but writes no compile_commands.json — the signature of a CMake generator that
// silently ignores CMAKE_EXPORT_COMPILE_COMMANDS (only the Makefile and Ninja
// generators honor it; the Xcode and Visual Studio generators do not). The caller
// distinguishes it (errors.Is) from a genuine configure failure so it can offer
// the right fix (switch generator) instead of the wrong one (disable test targets).
var ErrNoDatabaseProduced = errors.New("cmake produced no compilation database")

// dbName is the compilation-database filename cmake emits with
// CMAKE_EXPORT_COMPILE_COMMANDS=ON (kept local to avoid importing compdb).
const dbName = "compile_commands.json"

// Runner executes argv in dir and returns combined output; a package variable
// so tests run without cmake installed.
var Runner = func(ctx context.Context, dir string, argv []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	return buf.Bytes(), err
}

// FindProject walks upward from start (default ".") for the top-most directory
// that has a CMakeLists.txt — the project source root — returning it and true.
func FindProject(start string) (string, bool) {
	if start == "" {
		start = "."
	}
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	best := ""
	for {
		if _, err := os.Stat(filepath.Join(dir, "CMakeLists.txt")); err == nil {
			best = dir // keep climbing: the outermost CMakeLists.txt is the root
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return best, best != ""
}

// Available reports whether a cmake binary is on PATH. It is a package variable
// (like Runner) so tests can exercise Configure/Build — including their error
// and not-available paths — hermetically, without a real cmake on PATH.
var Available = func() bool {
	_, err := exec.LookPath("cmake")
	return err == nil
}

// Configure runs `cmake -S src -B build -DCMAKE_EXPORT_COMPILE_COMMANDS=ON`,
// producing build/compile_commands.json. It is a no-op-safe re-run (CMake
// caches configuration).
func Configure(ctx context.Context, src, build string) error {
	if !Available() {
		return fmt.Errorf("cmake not found in PATH; install it or pass -p <build-dir>")
	}
	out, err := Runner(ctx, src, []string{
		"cmake", "-S", src, "-B", build, "-DCMAKE_EXPORT_COMPILE_COMMANDS=ON",
	})
	if err != nil {
		return fmt.Errorf("cmake configure failed: %w\n%s", err, out)
	}
	// cmake exited 0, but CMAKE_EXPORT_COMPILE_COMMANDS is silently IGNORED by the
	// Xcode and Visual Studio generators — only the Makefile and Ninja generators
	// honor it. Verify the database was actually written, so a "successful"
	// configure that produced nothing usable surfaces its real cause HERE (with the
	// fix) instead of as a confusing "no compile_commands.json" far downstream.
	db := filepath.Join(build, dbName)
	if _, statErr := os.Stat(db); statErr != nil {
		return fmt.Errorf("%w: cmake configured %s but wrote no %s in %s (the Xcode and Visual Studio generators ignore CMAKE_EXPORT_COMPILE_COMMANDS; only the Makefile and Ninja generators honor it)",
			ErrNoDatabaseProduced, src, dbName, build)
	}
	return nil
}

// Build runs `cmake --build build` (optionally restricted to targets), which
// generates any build-time headers. It is incremental, so repeat runs are
// cheap. Build failures are returned but the caller may choose to proceed with
// whatever was produced.
func Build(ctx context.Context, build string, targets ...string) error {
	if !Available() {
		return fmt.Errorf("cmake not found in PATH")
	}
	argv := []string{"cmake", "--build", build}
	for _, t := range targets {
		argv = append(argv, "--target", t)
	}
	out, err := Runner(ctx, build, argv)
	if err != nil {
		return fmt.Errorf("cmake build failed: %w\n%s", err, out)
	}
	return nil
}
