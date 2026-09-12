package closureenv

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPortableBinaryPreservesMachOGates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for name, source := range map[string]string{"go.mod": "module example.test/portable\n\ngo 1.25.0\n", "leaf.go": "package portable\nfunc Leaf(v float64)float64{return v*2}\n", "leaf_test.go": "package portable\nimport \"testing\"\nfunc TestLeaf(t *testing.T){if Leaf(1)!=2{t.Fatal(\"wrong output\")}}\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	request := PackageRequest{Dir: dir, Pattern: ".", Env: []string{"GOOS=linux", "GOARCH=amd64", "GOAMD64=v1", "CGO_ENABLED=0", "GOFLAGS=", "GOEXPERIMENT=", "GOWORK=off", "GOTOOLCHAIN=local"}}
	if _, err := CollectBinary(context.Background(), &request); err == nil {
		t.Fatal("PS6130 collector accepted non-Mach-O target")
	}
	first, err := CollectPortableBinary(context.Background(), &request)
	if err != nil {
		t.Fatal(err)
	}
	if first.GOOS != "linux" || first.GOARCH != "amd64" || first.ArchitectureLevel != "v1" || first.CGOEnabled != "0" || len(first.Data) == 0 || first.ToolchainSHA256 == "" || first.MaterialSHA256 == "" || first.PortableMaterialSHA256 == "" {
		t.Fatalf("incomplete observed portable build: %+v", first)
	}
	second, err := CollectPortableBinary(context.Background(), &request)
	if err != nil {
		t.Fatal(err)
	}
	if first.BinarySHA256 != second.BinarySHA256 || first.MaterialSHA256 != second.MaterialSHA256 || first.ToolchainSHA256 != second.ToolchainSHA256 || first.PortableMaterialSHA256 != second.PortableMaterialSHA256 {
		t.Fatal("portable controlled build did not reproduce")
	}
	relocated := t.TempDir()
	for _, name := range []string{"go.mod", "leaf.go", "leaf_test.go"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(relocated, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	relocatedRequest := request
	relocatedRequest.Dir = relocated
	third, err := CollectPortableBinary(context.Background(), &relocatedRequest)
	if err != nil {
		t.Fatal(err)
	}
	if first.PortableMaterialSHA256 != third.PortableMaterialSHA256 || first.BinarySHA256 != third.BinarySHA256 || first.MaterialSHA256 == third.MaterialSHA256 {
		t.Fatal("controlled trimpath relocated build did not retain portable identity while distinguishing physical material location")
	}
	if _, err := CollectPortableBinary(context.Background(), &request, "simd/archsimd"); err == nil {
		t.Fatal("portable API relaxed supplied SDK identity")
	}
	request.Env = append(request.Env, "CGO_ENABLED=1")
	if _, err := CollectPortableBinary(context.Background(), &request); err == nil {
		t.Fatal("portable API relaxed cgo gate")
	}
}
