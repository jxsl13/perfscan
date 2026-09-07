package runner

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/checks"
	"github.com/jxsl13/perfscan/lint"
)

// TestPS6108FixModeLeavesAdvisorySourceUnchanged exercises the real -fix
// runner path in an isolated child process. The child owns its working
// directory, so the ordinary parent test remains safe to run in parallel.
func TestPS6108FixModeLeavesAdvisorySourceUnchanged(t *testing.T) {
	if directory := os.Getenv("PERFSCAN_PS6108_FIX_CHILD"); directory != "" {
		ps6108FixModeChild(t, directory)
		return
	}
	t.Parallel()

	fixture, err := os.ReadFile(filepath.Join("..", "..", "checks", "testdata", "src", "ps6108", "ps6108.go"))
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte("module ps6108fix\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "ps6108.go"), fixture, 0o644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestPS6108FixModeLeavesAdvisorySourceUnchanged$", "-test.count=1")
	command.Env = append(os.Environ(), "PERFSCAN_PS6108_FIX_CHILD="+directory)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("isolated PS6108 -fix child failed: %v\n%s", err, output)
	}
}

func ps6108FixModeChild(t *testing.T, directory string) {
	path := filepath.Join(directory, "ps6108.go")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(workingDirectory) }()

	var stdout, stderr bytes.Buffer
	code := Run(checks.All(), Options{
		Patterns: []string{"./..."},
		Checks:   "PS6108",
		MaxLevel: lint.LevelAggressive,
		Fix:      true,
		Stdout:   &stdout,
		Stderr:   &stderr,
	})
	if code != 1 {
		t.Fatalf("PS6108 -fix exit = %d, want 1 for advisory findings\nstdout:\n%s\nstderr:\n%s", code, &stdout, &stderr)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("PS6108 advisory source changed under -fix")
	}
	if !strings.Contains(stdout.String(), "PS6108") || !strings.Contains(stderr.String(), "applied 0 fix(es)") {
		t.Fatalf("PS6108 -fix output lacks advisory/zero-fix evidence:\nstdout:\n%s\nstderr:\n%s", &stdout, &stderr)
	}
}
