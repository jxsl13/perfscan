// Package cmake optionally bootstraps a compilation database (and, on request,
// generated headers) for a CMake project, so `perfscanxx -cmake ./...` can work
// without a pre-existing build. Running CMake and a build executes the
// project's build scripts, so callers gate this behind an explicit flag.
package cmake

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

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

// Available reports whether a cmake binary is on PATH.
func Available() bool {
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
