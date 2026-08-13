package tidy

import (
	"bytes"
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestArgv(t *testing.T) {
	got := Argv(Options{
		BuildDir:    "/proj/build",
		Checks:      []string{"performance-for-range-copy", "performance-avoid-endl"},
		Fix:         true,
		ExportFixes: "/tmp/fixes.yaml",
		Files:       []string{"a.cpp", "b.cpp"},
	})
	want := []string{
		"clang-tidy", "--quiet",
		"-p", "/proj/build",
		"--checks=-*,performance-for-range-copy,performance-avoid-endl",
		"--export-fixes=/tmp/fixes.yaml",
		"--fix",
		"a.cpp", "b.cpp",
	}
	if !slices.Equal(got, want) {
		t.Errorf("Argv =\n  %q\nwant\n  %q", got, want)
	}
}

func TestCheckNotFound(t *testing.T) {
	origLook := LookPath
	defer func() { LookPath = origLook }()
	LookPath = func(string) (string, error) { return "", errors.New("not found") }

	_, err := Check("")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Check: err = %v, want ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "brew install llvm") {
		t.Errorf("ErrNotFound message %q lacks the brew install hint", err.Error())
	}
}

// TestRunStubbed proves the package is unit-testable without clang-tidy:
// the Executor injection point replaces the real process launch.
func TestRunStubbed(t *testing.T) {
	origLook, origExec := LookPath, Executor
	defer func() { LookPath, Executor = origLook, origExec }()

	LookPath = func(string) (string, error) { return "/fake/clang-tidy", nil }

	const fakeYAML = "MainSourceFile: '/src/a.cpp'\nDiagnostics: []\n"
	var gotArgv []string
	Executor = func(_ context.Context, argv []string, stdout, stderr *bytes.Buffer) (int, error) {
		gotArgv = argv
		// Simulate clang-tidy writing the --export-fixes file.
		for _, a := range argv {
			if path, ok := strings.CutPrefix(a, "--export-fixes="); ok {
				if err := os.WriteFile(path, []byte(fakeYAML), 0o644); err != nil {
					return -1, err
				}
			}
		}
		stderr.WriteString("1 warning generated.\n")
		return 0, nil
	}

	res, err := Run(context.Background(), Options{
		BuildDir: "/proj/build",
		Checks:   []string{"performance-avoid-endl"},
		Files:    []string{"/src/a.cpp"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(res.ExportYAML) != fakeYAML {
		t.Errorf("ExportYAML = %q, want %q", res.ExportYAML, fakeYAML)
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", res.ExitCode)
	}
	if len(gotArgv) == 0 || gotArgv[0] != "clang-tidy" {
		t.Errorf("Executor argv = %q, want clang-tidy invocation", gotArgv)
	}
	if !slices.Contains(gotArgv, "--checks=-*,performance-avoid-endl") {
		t.Errorf("argv %q lacks curated --checks", gotArgv)
	}
}

func TestRunMissingBinary(t *testing.T) {
	origLook := LookPath
	defer func() { LookPath = origLook }()
	LookPath = func(string) (string, error) { return "", errors.New("nope") }

	_, err := Run(context.Background(), Options{Files: []string{"a.cpp"}})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Run without clang-tidy: err = %v, want ErrNotFound", err)
	}
}

// TestArgvVariants pins the invocation contract's branches that TestArgv leaves
// uncovered — most importantly that ConfigFile mode OMITS --checks (a stray
// --checks would override the config's Checks line and silently break the
// query-based custom checks that config drives).
func TestArgvVariants(t *testing.T) {
	joined := func(o Options) string { return strings.Join(Argv(o), " ") }

	t.Run("config-file mode omits --checks", func(t *testing.T) {
		got := Argv(Options{
			BuildDir:     "b",
			ConfigFile:   "/tmp/cfg.yaml",
			Experimental: true,
			Checks:       []string{"performance-avoid-endl"}, // must be ignored
			Files:        []string{"a.cpp"},
		})
		s := strings.Join(got, " ")
		if !strings.Contains(s, "--config-file=/tmp/cfg.yaml") {
			t.Errorf("argv %q lacks --config-file", got)
		}
		if strings.Contains(s, "--checks") {
			t.Errorf("argv %q must NOT carry --checks in config-file mode (it would override the config)", got)
		}
		if !strings.Contains(s, "--experimental-custom-checks") {
			t.Errorf("argv %q lacks --experimental-custom-checks", got)
		}
	})

	t.Run("default binary", func(t *testing.T) {
		if got := Argv(Options{Files: []string{"a.cpp"}}); got[0] != "clang-tidy" {
			t.Errorf("argv[0] = %q, want clang-tidy", got[0])
		}
	})

	t.Run("binary override", func(t *testing.T) {
		if got := Argv(Options{Binary: "clang-tidy-20", Files: []string{"a.cpp"}}); got[0] != "clang-tidy-20" {
			t.Errorf("argv[0] = %q, want clang-tidy-20", got[0])
		}
	})

	t.Run("extra args precede files, files come last", func(t *testing.T) {
		got := Argv(Options{
			Checks:    []string{"performance-avoid-endl"},
			ExtraArgs: []string{"-extra-arg=-isysroot", "-extra-arg=/sdk"},
			Files:     []string{"x.cpp", "y.cpp"},
		})
		if got[len(got)-2] != "x.cpp" || got[len(got)-1] != "y.cpp" {
			t.Errorf("files must be the final args; got tail %q", got[len(got)-3:])
		}
		xi, ei := indexOf(got, "x.cpp"), indexOf(got, "-extra-arg=-isysroot")
		if ei < 0 || xi < 0 || ei > xi {
			t.Errorf("extra args must precede files; argv=%q", got)
		}
	})

	t.Run("export-fixes and fix flags", func(t *testing.T) {
		s := joined(Options{Checks: []string{"c"}, ExportFixes: "/tmp/f.yaml", Fix: true, Files: []string{"a.cpp"}})
		if !strings.Contains(s, "--export-fixes=/tmp/f.yaml") || !strings.Contains(s, "--fix") {
			t.Errorf("argv %q missing --export-fixes/--fix", s)
		}
	})
}

func indexOf(ss []string, want string) int {
	for i, s := range ss {
		if s == want {
			return i
		}
	}
	return -1
}

// TestArgvExcludeHeaderFilter pins that an ExcludeHeaderFilter emits BOTH
// --header-filter (clang-tidy requires it alongside) and --exclude-header-filter,
// and that it is omitted entirely when unset.
func TestArgvExcludeHeaderFilter(t *testing.T) {
	with := strings.Join(Argv(Options{Checks: []string{"c"}, Files: []string{"a.cpp"}, ExcludeHeaderFilter: "deps/|vendor/"}), " ")
	if !strings.Contains(with, "--header-filter=.*") || !strings.Contains(with, "--exclude-header-filter=deps/|vendor/") {
		t.Errorf("argv missing header-filter pair: %s", with)
	}
	without := strings.Join(Argv(Options{Checks: []string{"c"}, Files: []string{"a.cpp"}}), " ")
	if strings.Contains(without, "header-filter") {
		t.Errorf("no ExcludeHeaderFilter set, but argv carries a header-filter: %s", without)
	}
}

func TestArgvFixErrors(t *testing.T) {
	has := func(o Options, arg string) bool { return slices.Contains(Argv(o), arg) }
	// FixErrors emits --fix-errors alongside --fix.
	fe := Options{Fix: true, FixErrors: true, Files: []string{"a.cpp"}}
	if !has(fe, "--fix-errors") || !has(fe, "--fix") {
		t.Errorf("argv %v: want both --fix and --fix-errors", Argv(fe))
	}
	// Without FixErrors, --fix-errors is absent.
	if has(Options{Fix: true, Files: []string{"a.cpp"}}, "--fix-errors") {
		t.Error("--fix-errors must be absent when FixErrors is false")
	}
}
