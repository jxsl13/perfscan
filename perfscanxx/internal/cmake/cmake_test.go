package cmake

import (
	"context"
	"os"
	"path/filepath"
	"slices"
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
	Runner = func(_ context.Context, dir string, argv []string) ([]byte, error) {
		gotDir, gotArgv = dir, argv
		return nil, nil
	}
	// Stub Available (not skip): the arg-construction contract must be pinned
	// even on machines/CI without a real cmake — that is why Available is a var.
	Available = func() bool { return true }

	// Configure runs in the SOURCE dir and requests the compile database.
	if err := Configure(context.Background(), "/src", "/src/build"); err != nil {
		t.Fatal(err)
	}
	wantCfg := []string{"cmake", "-S", "/src", "-B", "/src/build", "-DCMAKE_EXPORT_COMPILE_COMMANDS=ON"}
	if gotDir != "/src" || !slices.Equal(gotArgv, wantCfg) {
		t.Errorf("configure argv=%v dir=%q\n want argv=%v dir=/src", gotArgv, gotDir, wantCfg)
	}

	// Build (no targets) runs in the BUILD dir. (Multi-target argv and the
	// not-available / failure branches are covered by TestConfigureBuildErrorPaths.)
	if err := Build(context.Background(), "/src/build"); err != nil {
		t.Fatal(err)
	}
	if gotDir != "/src/build" || !slices.Equal(gotArgv, []string{"cmake", "--build", "/src/build"}) {
		t.Errorf("build (no target) argv=%v dir=%q", gotArgv, gotDir)
	}
}
