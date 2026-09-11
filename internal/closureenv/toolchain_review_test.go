package closureenv

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCollectToolchainAndStandardLibrary(t *testing.T) {
	t.Parallel()
	for _, target := range []struct {
		pkg, function, file string
		captures            int
	}{
		{"context", "AfterFunc", "context.go", 1},
		{"cmd/go/internal/lockedfile", "Lock", "mutex.go", 2},
	} {
		t.Run(target.pkg, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			evidence, err := CollectPackage(ctx, &PackageRequest{
				GoBinary: "go", Dir: filepath.Join(selectedTestGOROOT(t), "src"),
				Pattern: target.pkg, Function: target.function, File: target.file,
			})
			if err != nil || len(evidence) != 1 {
				t.Fatalf("actual %s.%s evidence = %+v, %v", target.pkg, target.function, evidence, err)
			}
			if evidence[0].Package != target.pkg || !evidence[0].Escapes || !evidence[0].Scanned || len(evidence[0].Captures) != target.captures {
				t.Fatalf("actual source closure proof = %+v", evidence[0])
			}
		})
	}
}

func selectedTestGOROOT(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	output, err := exec.CommandContext(ctx, "go", "env", "GOROOT").Output()
	if err != nil {
		t.Fatal(err)
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		t.Fatal("selected Go command returned an empty GOROOT")
	}
	return root
}
