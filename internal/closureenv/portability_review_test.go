package closureenv

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCollectNamedPointerCaptures(t *testing.T) {
	t.Parallel()
	for _, mutation := range []string{"", "p = nil"} {
		t.Run("mutation="+mutation, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "pointer.go")
			source := "package fixture\ntype Node struct { Value int }\nvar Sink func()\nfunc Work(p *Node) { Sink = func() { _ = p }; " + mutation + " }\n"
			if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			evidence, err := CollectFile(context.Background(), "go", runtime.GOOS, runtime.GOARCH, path)
			if err != nil || len(evidence) != 1 || len(evidence[0].Captures) != 1 {
				t.Fatalf("named pointer capture = %+v, %v", evidence, err)
			}
			capture := evidence[0].Captures[0]
			if !capture.HasPointers || capture.ByReference != (mutation != "") {
				t.Fatalf("named pointer representation = %+v", capture)
			}
		})
	}
}

func TestCollectPackageCrossTargetProvenance(t *testing.T) {
	t.Parallel()
	for _, architecture := range []string{"amd64", "arm64", "386"} {
		t.Run(architecture, func(t *testing.T) {
			t.Parallel()
			directory := t.TempDir()
			for name, source := range map[string]string{
				"go.mod":    "module example.com/crosstarget\n\ngo 1.25\n",
				"worker.go": "package worker\nvar Sink func()\nfunc Work(data []byte) { Sink = func() { _ = data } }\n",
			} {
				if err := os.WriteFile(filepath.Join(directory, name), []byte(source), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			environment := []string{"GOOS=linux", "GOARCH=" + architecture, "CGO_ENABLED=0"}
			evidence, err := CollectPackage(context.Background(), &PackageRequest{GoBinary: "go", Dir: directory, Pattern: ".", GOOS: "linux", GOARCH: architecture, Env: environment, Function: "Work"})
			if err != nil || len(evidence) != 1 {
				t.Fatalf("target %s collection = %+v, %v", architecture, evidence, err)
			}
			if err := ValidateSelected(context.Background(), &evidence[0], "go", directory, environment); err != nil {
				t.Fatalf("target %s rejected its own compiler evidence: %v", architecture, err)
			}
		})
	}
}
