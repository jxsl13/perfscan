package cmake

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestFindProject(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b")
	os.MkdirAll(deep, 0o755)
	os.WriteFile(filepath.Join(root, "CMakeLists.txt"), []byte("project(x)"), 0o644)
	os.WriteFile(filepath.Join(root, "a", "CMakeLists.txt"), []byte("add_subdirectory(b)"), 0o644)
	// Outermost CMakeLists.txt (root) is the project source root.
	got, ok := FindProject(deep)
	if !ok || got != root {
		t.Fatalf("FindProject=%q,%v want %q", got, ok, root)
	}
	if _, ok := FindProject(t.TempDir()); ok {
		t.Error("expected no project in an empty dir")
	}
	// An empty start defaults to "." (the current directory) — it must behave
	// exactly like passing "." explicitly, whatever the test's cwd resolves to.
	gotEmpty, okEmpty := FindProject("")
	gotDot, okDot := FindProject(".")
	if gotEmpty != gotDot || okEmpty != okDot {
		t.Errorf("FindProject(\"\")=%q,%v != FindProject(\".\")=%q,%v (empty must default to \".\")", gotEmpty, okEmpty, gotDot, okDot)
	}
}

func TestConfigureBuildArgs(t *testing.T) {
	var gotDir string
	var gotArgv []string
	origRunner, origAvail := Runner, Available
	defer func() { Runner, Available = origRunner, origAvail }()
	// Real dirs so Configure's post-condition (the database was written) can be
	// satisfied by the stub — a successful cmake configure writes the DB.
	src := t.TempDir()
	build := filepath.Join(src, "build")
	Runner = func(_ context.Context, dir string, argv []string) ([]byte, error) {
		gotDir, gotArgv = dir, argv
		_ = os.MkdirAll(build, 0o755)
		_ = os.WriteFile(filepath.Join(build, "compile_commands.json"), []byte("[]"), 0o644)
		return nil, nil
	}
	// Stub Available (not skip): the arg-construction contract must be pinned
	// even on machines/CI without a real cmake — that is why Available is a var.
	Available = func() bool { return true }

	// Configure runs in the SOURCE dir and requests the compile database.
	if err := Configure(context.Background(), src, build); err != nil {
		t.Fatal(err)
	}
	wantCfg := []string{"cmake", "-S", src, "-B", build, "-DCMAKE_EXPORT_COMPILE_COMMANDS=ON"}
	if gotDir != src || !slices.Equal(gotArgv, wantCfg) {
		t.Errorf("configure argv=%v dir=%q\n want argv=%v dir=%s", gotArgv, gotDir, wantCfg, src)
	}

	// Build (no targets) runs in the BUILD dir. (Multi-target argv and the
	// not-available / failure branches are covered by TestConfigureBuildErrorPaths.)
	if err := Build(context.Background(), build); err != nil {
		t.Fatal(err)
	}
	if gotDir != build || !slices.Equal(gotArgv, []string{"cmake", "--build", build}) {
		t.Errorf("build (no target) argv=%v dir=%q", gotArgv, gotDir)
	}
}

// TestConfigureVerifiesDatabaseWritten pins the post-configure check: cmake can
// exit 0 yet write no compile_commands.json when the active generator ignores
// CMAKE_EXPORT_COMPILE_COMMANDS (Xcode / Visual Studio). Configure must then return
// ErrNoDatabaseProduced — carrying the switch-generator fix — rather than a nil
// "success" that only surfaces as a confusing "no compile_commands.json" later.
func TestConfigureVerifiesDatabaseWritten(t *testing.T) {
	origRunner, origAvail := Runner, Available
	defer func() { Runner, Available = origRunner, origAvail }()
	Available = func() bool { return true }

	// cmake "succeeds" but writes nothing (generator ignored the flag).
	src := t.TempDir()
	build := filepath.Join(src, "build")
	Runner = func(context.Context, string, []string) ([]byte, error) { return []byte("-- Configuring done"), nil }
	err := Configure(context.Background(), src, build)
	if !errors.Is(err, ErrNoDatabaseProduced) {
		t.Fatalf("Configure with no DB written = %v, want ErrNoDatabaseProduced", err)
	}
	// The message must name the real cause (the ignored flag) and the supported
	// generators. (The copy-paste `-G Ninja` command line is printed by main.go,
	// mirroring how the tests/deps advice lives in the caller, not the error.)
	for _, want := range []string{"CMAKE_EXPORT_COMPILE_COMMANDS", "Ninja"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}

	// The happy path: cmake writes the DB -> nil.
	Runner = func(context.Context, string, []string) ([]byte, error) {
		_ = os.MkdirAll(build, 0o755)
		_ = os.WriteFile(filepath.Join(build, "compile_commands.json"), []byte("[]"), 0o644)
		return nil, nil
	}
	if err := Configure(context.Background(), src, build); err != nil {
		t.Errorf("Configure that wrote the DB = %v, want nil", err)
	}
}
