package checks_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/checks"
	"github.com/jxsl13/perfscan/internal/runner"
	"github.com/jxsl13/perfscan/lint"
)

const ps2145RunnerSource = `package sample

import (
	"encoding/binary"
	"io"
	"os"
)

type result struct{ left, right []byte }

func open(path string) (*result, error) {
	f, err := os.Open(path)
	if err != nil { return nil, err }
	defer f.Close()
	return parse(f)
}

func parse(r io.Reader) (*result, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil { return nil, err }
	size := binary.LittleEndian.Uint32(header[:])
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil { return nil, err }
	return &result{left: payload[:1], right: payload[1:]}, nil
}
`

func TestPS2145RunnerFixLeavesSourceUnchanged(t *testing.T) {
	t.Parallel()
	const childVariable = "PERFSCAN_PS2145_RUNNER_CHILD"
	if os.Getenv(childVariable) != "1" {
		command := exec.Command(os.Args[0], "-test.run=^TestPS2145RunnerFixLeavesSourceUnchanged$", "-test.parallel=1")
		command.Env = append(os.Environ(), childVariable+"=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("isolated -fix runner failed: %v\n%s", err, output)
		}
		return
	}

	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte("module sample\n\ngo 1.25\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "sample.go")
	before := []byte(ps2145RunnerSource)
	if err := os.WriteFile(path, before, 0o600); err != nil {
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
	code := runner.Run(checks.All(), runner.Options{
		Patterns: []string{"./..."}, Checks: "PS2145", MaxLevel: lint.LevelAggressive,
		Fix: true, ExitZero: true, Stdout: &stdout, Stderr: &stderr,
	})
	if code != 0 || !strings.Contains(stdout.String(), "PS2145") || !strings.Contains(stderr.String(), "applied 0 fix(es)") {
		t.Fatalf("unexpected -fix result code=%d\nstdout:\n%s\nstderr:\n%s", code, &stdout, &stderr)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("advisory -fix changed source:\n%s", after)
	}
}
