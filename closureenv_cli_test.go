package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jxsl13/perfscan/internal/closureenv"
)

// Exercise both installed command entry points, not a handcrafted analysis.Pass.
func TestClosureEnvironmentCLI(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	var environment []string
	command := func(directory, binary string, args ...string) (string, int) {
		t.Helper()
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Dir = directory
		cmd.Env = environment
		output, err := cmd.CombinedOutput()
		if err == nil {
			return string(output), 0
		}
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("execute %s: %v", binary, err)
		}
		return string(output), exit.ExitCode()
	}
	write := func(path, text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	collector, scanner := filepath.Join(root, "closureenv"), filepath.Join(root, "perfscan")
	if runtime.GOOS == "windows" {
		collector += ".exe"
		scanner += ".exe"
	}
	for _, build := range []struct{ path, pkg string }{{collector, "./cmd/perfscan-closureenv"}, {scanner, "."}} {
		if output, code := command("", "go", "build", "-o", build.path, build.pkg); code != 0 {
			t.Fatalf("build %s: exit %d: %s", build.pkg, code, output)
		}
	}
	beforeDir, afterDir := filepath.Join(root, "before"), filepath.Join(root, "after")
	for _, directory := range []string{beforeDir, afterDir} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		write(filepath.Join(directory, "go.mod"), "module example.com/worker\n\ngo 1.25\n")
	}
	header := "package worker\nvar sink func(int,int)\nfunc parallelWork(f func(int,int)){sink=f}\n"
	before := header + "func Work(a0,a1,a2,a3,a4,a5,a6,a7,a8 []byte){\nparallelWork(func(lo,hi int){_,_,_,_,_,_,_,_,_,_=a0,a1,a2,a3,a4,a5,a6,a7,a8,lo+hi})\n}\n"
	after := header + "\nfunc Work(a0,a1,a2,a3,a4,a5,a6,a7,a8 []byte,width byte){\nparallelWork(func(lo,hi int){_,_,_,_,_,_,_,_,_,_,_=a0,a1,a2,a3,a4,a5,a6,a7,a8,width,lo+hi})\n}\n"
	write(filepath.Join(beforeDir, "worker.go"), before)
	write(filepath.Join(afterDir, "worker.go"), after)
	encoded, code := command("", collector, "-before-dir", beforeDir, "-after-dir", afterDir, "-package", ".", "-function", "Work", "-before-file", "worker.go", "-before-line", "5", "-before-column", "14", "-after-file", "worker.go", "-after-line", "6", "-after-column", "14")
	if code != 0 {
		t.Fatalf("collect package revisions: exit %d: %s", code, encoded)
	}
	growth, err := closureenv.ReadArtifact(strings.NewReader(encoded))
	if err != nil || growth.Before.Line != 5 || growth.After.Line != 6 {
		t.Fatalf("shifted closure evidence: %v, %v", growth, err)
	}
	configPath := filepath.Join(afterDir, "perfscan.yaml")
	configure := func(artifact string) {
		write(configPath, "fanOutHelpers:\n  - example.com/worker.parallelWork\nclosureEnvironmentGrowthArtifacts:\n  - |\n    "+strings.ReplaceAll(strings.TrimSpace(artifact), "\n", "\n    ")+"\n")
	}
	configure(encoded)
	output, code := command(afterDir, scanner, "-checks", "PS6126", "-config", configPath, ".")
	if code != 1 || !strings.Contains(output, "crossed allocator class") || !strings.Contains(output, "PS6126") {
		t.Fatalf("real CLI diagnostic: exit %d: %s", code, output)
	}
	renamed := *growth
	renamed.Before.Package, renamed.After.Package = "example.com/renamed", "example.com/renamed"
	var absentPackage bytes.Buffer
	if err := closureenv.WriteArtifact(&absentPackage, &renamed); err != nil {
		t.Fatal(err)
	}
	configure(absentPackage.String())
	output, code = command(afterDir, scanner, "-checks", "PS6126", "-config", configPath, ".")
	if code != 2 || !strings.Contains(output, "outside the loaded scan packages") {
		t.Fatalf("missing evidence package: exit %d: %s", code, output)
	}
	// JSON must not select a different tagged source universe from the scan.
	tagged := *growth
	tagged.Before.BuildTags, tagged.After.BuildTags = []string{"alternate"}, []string{"alternate"}
	var taggedArtifact bytes.Buffer
	if err := closureenv.WriteArtifact(&taggedArtifact, &tagged); err != nil {
		t.Fatal(err)
	}
	configure(taggedArtifact.String())
	output, code = command(afterDir, scanner, "-checks", "PS6126", "-config", configPath, ".")
	if code != 2 || !strings.Contains(output, "artifact-only build tags") {
		t.Fatalf("artifact-selected scan tags: exit %d: %s", code, output)
	}
	// A stale artifact must fail even when findings themselves are exit-zero.
	growth.After.SourceSHA256 = strings.Repeat("0", 64)
	var stale bytes.Buffer
	if err := closureenv.WriteArtifact(&stale, growth); err != nil {
		t.Fatal(err)
	}
	configure(stale.String())
	output, code = command(afterDir, scanner, "-checks", "PS6126", "-config", configPath, "-exit-zero", ".")
	if code != 2 || !strings.Contains(output, "compiler evidence mismatch") {
		t.Fatalf("stale CLI evidence: exit %d: %s", code, output)
	}
	// User-selected flags, in contrast, select both programs' actual context.
	environment = append(os.Environ(), "GOFLAGS=-tags=closureenvfixture")
	encoded, code = command("", collector, "-before-dir", beforeDir, "-after-dir", afterDir, "-package", ".", "-function", "Work")
	if code != 0 {
		t.Fatalf("collect user-selected tags: exit %d: %s", code, encoded)
	}
	configure(encoded)
	output, code = command(afterDir, scanner, "-checks", "PS6126", "-config", configPath, ".")
	if code != 1 || !strings.Contains(output, "crossed allocator class") {
		t.Fatalf("scan user-selected tags: exit %d: %s", code, output)
	}
}
