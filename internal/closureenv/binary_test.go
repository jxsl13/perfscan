package closureenv

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBinarySDKPackageProvenance(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	sdk := listedBinaryPackage{ImportPath: "simd/archsimd", Dir: filepath.Join(root, "src", "simd", "archsimd"), Standard: true, Goroot: true}
	for _, tc := range []struct {
		name   string
		change func(*listedBinaryPackage)
		want   bool
	}{
		{"sdk", func(*listedBinaryPackage) {}, true},
		{"main module impostor", func(p *listedBinaryPackage) { p.Standard = false; p.Goroot = false }, false},
		{"nonstandard", func(p *listedBinaryPackage) { p.Standard = false }, false},
		{"outside SDK", func(p *listedBinaryPackage) { p.Dir = filepath.Join(root, "outside") }, false},
		{"wrong root", func(p *listedBinaryPackage) { p.Goroot = false }, false},
		{"missing", func(p *listedBinaryPackage) { p.ImportPath = "other" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := sdk
			tc.change(&p)
			data, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := binarySDKPackages(data, root, []string{"simd/archsimd"}); (err == nil) != tc.want {
				t.Fatalf("provenance error=%v want valid=%v", err, tc.want)
			}
		})
	}
}

func TestBinaryMaterialInventory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"f.go", "f_test.go", "asm.s", "payload.bin", "go.mod", "go.sum"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	pkg := listedBinaryPackage{ImportPath: "example.test/p", Dir: dir, GoFiles: []string{"f.go"}, TestGoFiles: []string{"f_test.go"}, SFiles: []string{"asm.s"}, EmbedFiles: []string{"payload.bin"}}
	// Module fields retain manifest contents as build materials.
	modulePackage := listedBinaryPackage{ImportPath: "example.test/p", Dir: dir, GoFiles: []string{"f.go"}}
	modulePackage.Module = &struct {
		Dir, GoMod string
		Replace    *struct{ Path string }
	}{Dir: dir, GoMod: filepath.Join(dir, "go.mod")}
	module, err := json.Marshal(modulePackage)
	if err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(pkg)
	if err != nil {
		t.Fatal(err)
	}
	before, err := binaryMaterialDigest(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "asm.s"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	after, err := binaryMaterialDigest(out)
	if err != nil || after == before {
		t.Fatalf("native material change not detected: %v", err)
	}
	before, err = binaryMaterialDigest(module)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.sum"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	after, err = binaryMaterialDigest(module)
	if err != nil || after == before {
		t.Fatalf("module sum change not detected: %v", err)
	}
	if _, err := binaryMaterialDigest([]byte(`{"CgoFiles":["cgo.go"]}`)); err == nil {
		t.Fatal("accepted cgo")
	}
	if _, err := binaryMaterialDigest([]byte(`{"Module":{"Replace":{"Path":"replacement"}}}`)); err == nil {
		t.Fatal("accepted replacement")
	}
	if _, err := binaryMaterialDigest([]byte("{")); err == nil {
		t.Fatal("accepted malformed inventory")
	}
	if err := os.WriteFile(filepath.Join(dir, "f.go"), []byte("package p\n//go:linkname local example.test/p.c0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := binaryMaterialDigest(out, "example.test/p"); err == nil {
		t.Fatal("accepted dependency linkname into target")
	}
	if err := os.WriteFile(filepath.Join(dir, "f.go"), []byte("package p\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "asm.s"), []byte("MOVD $example.test∕p·c0(SB), R0"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := binaryMaterialDigest(out, "example.test/p"); err == nil {
		t.Fatal("accepted qualified native target reference")
	}
	if _, err := CollectBinary(context.Background(), nil); err == nil {
		t.Fatal("accepted missing request")
	}
}

func TestControlledBinaryReproduction(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for name, data := range map[string]string{
		"go.mod":    "module example.test/codegenfixture\n\ngo 1.25.0\n",
		"f.go":      "package codegenfixture\nfunc Leaf(v float64)float64{return v*2}\n",
		"f_test.go": "package codegenfixture\nimport \"testing\"\nfunc TestLeaf(t *testing.T){if Leaf(1)!=2{t.Fatal(\"incorrect\")}}\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	request := PackageRequest{Dir: dir, Pattern: ".", Env: []string{"GOOS=darwin", "GOARCH=arm64", "GOARM64=v8.0", "CGO_ENABLED=0", "GOFLAGS=", "GOWORK=off", "GOEXPERIMENT=", "GOTOOLCHAIN=local"}}
	first, err := CollectBinary(context.Background(), &request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CollectBinary(context.Background(), &request)
	if err != nil {
		t.Fatal(err)
	}
	if first.BinarySHA256 != second.BinarySHA256 || first.MaterialSHA256 != second.MaterialSHA256 || first.ToolchainSHA256 != second.ToolchainSHA256 || len(first.Data) == 0 {
		t.Fatal("controlled binary did not reproduce")
	}
	request.Env = append(request.Env, "GOFLAGS=-overlay=missing")
	if _, err := CollectBinary(context.Background(), &request); err == nil || !strings.Contains(err.Error(), "GOFLAGS") {
		t.Fatalf("overlay not rejected: %v", err)
	}
}
