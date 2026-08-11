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
