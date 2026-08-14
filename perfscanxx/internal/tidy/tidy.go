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
	"regexp"
	"strconv"
	"strings"
)

// MinExperimentalMajor is the first LLVM/clang-tidy major version that supports
// --experimental-custom-checks, and thus the query-based custom checks
// (PX2101-PX2106). On an older clang-tidy that flag is an unknown argument and
// clang-tidy exits with an error, so the caller degrades gracefully to the
// built-in checks instead.
const MinExperimentalMajor = 20

var versionRe = regexp.MustCompile(`LLVM version (\d+)`)

// MajorVersion returns the clang-tidy LLVM major version by parsing
// `<binary> --version` (via the injectable Executor, so it is hermetically
// testable). ok is false when the binary cannot be run or its output does not
// contain a recognizable "LLVM version N.M.P" line — in which case the caller
// should NOT assume the worst (perfscanxx requires a modern toolchain anyway).
func MajorVersion(ctx context.Context, binary string) (major int, ok bool) {
	if binary == "" {
		binary = "clang-tidy"
	}
	var stdout, stderr bytes.Buffer
	if _, err := Executor(ctx, []string{binary, "--version"}, &stdout, &stderr); err != nil {
		return 0, false
	}
	m := versionRe.FindSubmatch(stdout.Bytes())
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(string(m[1]))
	if err != nil {
		return 0, false
	}
	return n, true
}

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
	// FixErrors adds --fix-errors, applying fix-its even to a translation unit
	// that failed to compile. clang-tidy otherwise SKIPS any TU with an error,
	// so on a project with a missing build-time header (fmt, leveldb, abseil…)
	// plain --fix silently changes nothing. Opt-in because a fix based on a
	// partly-erroneous AST could be wrong; see the -fix-errors flag's caveat.
	FixErrors bool
	// ExcludeHeaderFilter, when non-empty, is a regex passed as
	// --exclude-header-filter so clang-tidy does not DIAGNOSE (and therefore
	// does not --fix) any header whose path matches it — the mechanism that
	// makes -exclude actually cover fix-its landing in an excluded header that a
	// non-excluded TU includes. clang-tidy requires --header-filter alongside it,
	// so HeaderFilter is sent too (default ".*", preserving header reporting).
	ExcludeHeaderFilter string
	// HeaderFilter is the --header-filter regex; only emitted when
	// ExcludeHeaderFilter is set (clang-tidy pairs the two). Empty means ".*".
	HeaderFilter string
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
	if o.FixErrors {
		args = append(args, "--fix-errors")
	}
	if o.ExcludeHeaderFilter != "" {
		hf := o.HeaderFilter
		if hf == "" {
			hf = ".*"
		}
		// clang-tidy requires --header-filter with --exclude-header-filter; the
		// exclude then suppresses matching headers' diagnostics AND fix-its.
		args = append(args, "--header-filter="+hf, "--exclude-header-filter="+o.ExcludeHeaderFilter)
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
	// With no config file, Argv builds --checks=-*,<Checks...>; an empty Checks
	// list collapses to --checks=-*, which DISABLES every check. clang-tidy then
	// runs to completion and reports nothing — indistinguishable from "clean".
	// Empty Checks + no ConfigFile is never an intended request (Argv always
	// prepends -*), so reject it explicitly rather than silently scan nothing —
	// the same fail-loud philosophy as the no-input-files guard above.
	if o.ConfigFile == "" && len(o.Checks) == 0 {
		return nil, errors.New("tidy: no checks selected (would disable all checks and report nothing)")
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
