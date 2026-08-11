package cmake

import (
	"context"
	"os"
	"path/filepath"
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
}

func TestConfigureBuildArgs(t *testing.T) {
	var gotDir string
	var gotArgv []string
	orig := Runner
	defer func() { Runner = orig }()
	Runner = func(_ context.Context, dir string, argv []string) ([]byte, error) {
		gotDir, gotArgv = dir, argv
		return nil, nil
	}
	// Available() checks PATH for cmake; skip if absent so the test is hermetic.
	if !Available() {
		t.Skip("cmake not on PATH")
	}
	_ = Configure(context.Background(), "/src", "/src/build")
	if gotDir != "/src" || gotArgv[0] != "cmake" || gotArgv[1] != "-S" {
		t.Errorf("configure argv=%v dir=%q", gotArgv, gotDir)
	}
	_ = Build(context.Background(), "/src/build", "gen")
	if gotArgv[1] != "--build" || gotArgv[len(gotArgv)-1] != "gen" {
		t.Errorf("build argv=%v", gotArgv)
	}
}
