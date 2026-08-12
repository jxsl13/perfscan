package cmake

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

// Configure/Build wrap Runner failures and short-circuit when cmake is absent;
// the existing arg test only covers the success path (and skips without cmake).
// Stub both Available and Runner to cover the error, not-available, and
// multi-target branches hermetically.
func TestConfigureBuildErrorPaths(t *testing.T) {
	origR, origA := Runner, Available
	defer func() { Runner, Available = origR, origA }()

	Available = func() bool { return true }
	Runner = func(context.Context, string, []string) ([]byte, error) {
		return []byte("cmake said no"), errors.New("exit status 1")
	}
	if err := Configure(context.Background(), "/s", "/b"); err == nil ||
		!strings.Contains(err.Error(), "cmake configure failed") ||
		!strings.Contains(err.Error(), "cmake said no") {
		t.Errorf("Configure error = %v; want it to wrap the failure and include output", err)
	}
	if err := Build(context.Background(), "/b", "gen"); err == nil ||
		!strings.Contains(err.Error(), "cmake build failed") {
		t.Errorf("Build error = %v; want it to wrap the failure", err)
	}

	// Multiple targets each get their own --target flag.
	var gotArgv []string
	Runner = func(_ context.Context, _ string, argv []string) ([]byte, error) {
		gotArgv = argv
		return nil, nil
	}
	if err := Build(context.Background(), "/b", "t1", "t2"); err != nil {
		t.Fatalf("Build: unexpected error %v", err)
	}
	want := []string{"cmake", "--build", "/b", "--target", "t1", "--target", "t2"}
	if !slices.Equal(gotArgv, want) {
		t.Errorf("Build argv = %v, want %v", gotArgv, want)
	}

	// When cmake is not available, both short-circuit with a clear error and
	// never invoke Runner.
	Available = func() bool { return false }
	ranNoCmake := false
	Runner = func(context.Context, string, []string) ([]byte, error) {
		ranNoCmake = true
		return nil, nil
	}
	if err := Configure(context.Background(), "/s", "/b"); err == nil || !strings.Contains(err.Error(), "cmake not found") {
		t.Errorf("Configure without cmake = %v, want a 'cmake not found' error", err)
	}
	if err := Build(context.Background(), "/b"); err == nil || !strings.Contains(err.Error(), "cmake not found") {
		t.Errorf("Build without cmake = %v, want a 'cmake not found' error", err)
	}
	if ranNoCmake {
		t.Error("Runner must not be invoked when cmake is unavailable")
	}
}
