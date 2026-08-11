// Package tidy invokes clang-tidy and returns its --export-fixes payload.
//
// clang-tidy is a RUNTIME dependency only: this package (and everything that
// imports it) builds and unit-tests without clang-tidy installed. The
// Executor variable is the injection point — tests replace it with a stub.
package tidy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// NotFoundHint is the actionable message printed when clang-tidy is absent.
const NotFoundHint = "clang-tidy not found in PATH.\n" +
	"perfscanxx orchestrates clang-tidy; install it via:\n\n" +
	"    brew install llvm\n" +
	"    export PATH=\"$(brew --prefix llvm)/bin:$PATH\"\n\n" +
	"(on Linux: apt install clang-tidy)"

// ErrNotFound is returned when the clang-tidy binary cannot be located.
var ErrNotFound = errors.New(NotFoundHint)

// Options describes one clang-tidy invocation.
type Options struct {
	// Binary overrides the executable name (default "clang-tidy").
	Binary string
	// BuildDir is the directory holding compile_commands.json (-p).
	BuildDir string
	// Checks are the exact clang-tidy check names to enable; everything
	// else is disabled with a leading "-*". Ignored when ConfigFile is set.
	Checks []string
	// ConfigFile, when non-empty, is passed as --config-file and drives
	// check selection (used for query-based custom checks, whose CustomChecks
	// definitions live in that config); the cmdline --checks is then omitted.
	ConfigFile string
	// Experimental enables clang-tidy --experimental-custom-checks, required
	// for query-based custom checks (LLVM >= 20).
	Experimental bool
	// Fix makes clang-tidy apply its fix-its in place.
	Fix bool
	// ExportFixes is the --export-fixes destination. Empty means: use a
	// temp file managed by Run.
	ExportFixes string
	// Files are the translation units to analyze.
	Files []string
	// ExtraArgs are appended verbatim (escape hatch).
	ExtraArgs []string
}

// Argv builds the full clang-tidy argument vector (argv[0] included).
func Argv(o Options) []string {
	bin := o.Binary
	if bin == "" {
		bin = "clang-tidy"
	}
	args := []string{bin, "--quiet"}
	if o.BuildDir != "" {
		args = append(args, "-p", o.BuildDir)
	}
	if o.Experimental {
		args = append(args, "--experimental-custom-checks")
	}
	if o.ConfigFile != "" {
		// The config file supplies both Checks and any CustomChecks; a
		// cmdline --checks would override its Checks line, so omit it.
		args = append(args, "--config-file="+o.ConfigFile)
	} else {
		checks := append([]string{"-*"}, o.Checks...)
		args = append(args, "--checks="+strings.Join(checks, ","))
	}
	if o.ExportFixes != "" {
		args = append(args, "--export-fixes="+o.ExportFixes)
	}
	if o.Fix {
		args = append(args, "--fix")
	}
	if len(o.ExtraArgs) > 0 {
		args = append(args, o.ExtraArgs...)
	}
	args = append(args, o.Files...)
	return args
}

// Result is the outcome of one clang-tidy run.
type Result struct {
	// ExportYAML is the raw --export-fixes file contents (may be empty
	// when there were no diagnostics).
	ExportYAML []byte
	// Stderr is clang-tidy's diagnostic chatter, for -v style surfacing.
	Stderr string
	// ExitCode is clang-tidy's exit code. clang-tidy exits non-zero on
	// compile errors, not on ordinary warnings.
	ExitCode int
}

// Executor runs argv and returns its exit code. It is a package variable so
// unit tests can run without clang-tidy installed. The default implementation
// shells out via os/exec.
var Executor = func(ctx context.Context, argv []string, stdout, stderr *bytes.Buffer) (int, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	if err != nil {
		return -1, err
	}
	return 0, nil
}

// LookPath locates the clang-tidy binary; a variable for testability.
var LookPath = exec.LookPath

// Check verifies that clang-tidy is invocable and returns its resolved path.
// When absent it returns ErrNotFound, whose message tells the user exactly
// how to install it (brew install llvm).
func Check(binary string) (string, error) {
	if binary == "" {
		binary = "clang-tidy"
	}
	path, err := LookPath(binary)
	if err != nil {
		return "", ErrNotFound
	}
	return path, nil
}

// Run checks availability, invokes clang-tidy once over o.Files and returns
// the export-fixes payload. A missing binary surfaces as ErrNotFound before
// anything is executed.
func Run(ctx context.Context, o Options) (*Result, error) {
	if _, err := Check(o.Binary); err != nil {
		return nil, err
	}
	if len(o.Files) == 0 {
		return nil, errors.New("tidy: no input files")
	}

	export := o.ExportFixes
	if export == "" {
		dir, err := os.MkdirTemp("", "perfscanxx-fixes-*")
		if err != nil {
			return nil, fmt.Errorf("tidy: %w", err)
		}
		defer os.RemoveAll(dir)
		export = filepath.Join(dir, "fixes.yaml")
		o.ExportFixes = export
	}

	var stdout, stderr bytes.Buffer
	code, err := Executor(ctx, Argv(o), &stdout, &stderr)
	if err != nil {
		return nil, fmt.Errorf("tidy: invoking clang-tidy: %w", err)
	}

	// clang-tidy only writes the file when it produced diagnostics.
	yamlBytes, readErr := os.ReadFile(export)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return nil, fmt.Errorf("tidy: reading export-fixes: %w", readErr)
	}

	return &Result{
		ExportYAML: yamlBytes,
		Stderr:     stderr.String(),
		ExitCode:   code,
	}, nil
}
