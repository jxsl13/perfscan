package checks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/internal/closureenv"
	"github.com/jxsl13/perfscan/internal/coefficientcodegen"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/packages"
)

const ps6130ArchSource = `package archsimd
type Float64x2 struct{};type Mask64x2 struct{};type Int64x2 struct{};type Uint64x2 struct{}
func BroadcastFloat64x2(float64)Float64x2;func BroadcastInt64x2(int64)Int64x2;func BroadcastUint64x2(uint64)Uint64x2;func LoadFloat64x2Array(*[2]float64)Float64x2
func(Float64x2)Max(Float64x2)Float64x2;func(Float64x2)Mul(Float64x2)Float64x2;func(Float64x2)Round()Float64x2;func(Float64x2)MulAdd(Float64x2,Float64x2)Float64x2;func(Float64x2)Add(Float64x2)Float64x2;func(Float64x2)Sub(Float64x2)Float64x2;func(Float64x2)Div(Float64x2)Float64x2;func(Float64x2)Abs()Float64x2;func(Float64x2)ConvertToInt64()Int64x2;func(Float64x2)ToBits()Uint64x2;func(Float64x2)Less(Float64x2)Mask64x2;func(Float64x2)GreaterEqual(Float64x2)Mask64x2;func(Float64x2)IfElse(Mask64x2,Float64x2)Float64x2;func(Float64x2)StoreArray(*[2]float64)
func(Mask64x2)ToInt64x2()Int64x2;func(Int64x2)GetElem(uint8)int64;func(Int64x2)Add(Int64x2)Int64x2;func(Int64x2)ShiftAllLeft(uint64)Int64x2;func(Int64x2)ToBits()Uint64x2;func(Uint64x2)And(Uint64x2)Uint64x2;func(Uint64x2)Or(Uint64x2)Uint64x2;func(Uint64x2)BitsToFloat64()Float64x2`

func ps6130TestPass(t *testing.T, sources map[string]string) (*analysis.Pass, *coefficientcodegen.Artifact, *[]analysis.Diagnostic) {
	t.Helper()
	fset := token.NewFileSet()
	sizes := types.SizesFor("gc", "arm64")
	arch, err := parser.ParseFile(fset, "arch.go", ps6130ArchSource, 0)
	if err != nil {
		t.Fatal(err)
	}
	archPkg, err := (&types.Config{Sizes: sizes}).Check("simd/archsimd", fset, []*ast.File{arch}, nil)
	if err != nil {
		t.Fatal(err)
	}
	files := []*ast.File{}
	hashes := map[string]string{}
	for name, text := range sources {
		file, err := parser.ParseFile(fset, name, text, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
		hash := sha256.Sum256([]byte(text))
		hashes[name] = hex.EncodeToString(hash[:])
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
	pkg, err := (&types.Config{Importer: ps6123Importer{archPkg}, Sizes: sizes}).Check("github.com/jxsl13/goai/backend/cpu", fset, files, info)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := []analysis.Diagnostic{}
	pass := &analysis.Pass{Analyzer: PS6130.Analyzer, Fset: fset, Files: files, Pkg: pkg, TypesInfo: info, TypesSizes: sizes, ReadFile: func(path string) ([]byte, error) { return []byte(sources[path]), nil }, Report: func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) }}
	a := &coefficientcodegen.Artifact{Build: closureenv.BinaryBuild{Package: pkg.Path(), GoVersion: "go1.27.1", GOOS: "darwin", GOARCH: "arm64", ArchitectureLevel: "v8.0", CGOEnabled: "0", MaterialSHA256: strings.Repeat("a", 64), ToolchainSHA256: strings.Repeat("b", 64), BinarySHA256: strings.Repeat("c", 64), SourceSHA256: hashes}, Function: "leaf"}
	return pass, a, &diagnostics
}

func ps6130RunFixture(t *testing.T, pass *analysis.Pass, a *coefficientcodegen.Artifact) error {
	t.Helper()
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	_, err = runPS6130Artifacts(pass, []string{string(data)}, false, "simd/archsimd")
	return err
}

func TestPS6130AnalysisFixture(t *testing.T) {
	t.Parallel()
	analyzer := *PS6130.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		hashes := make(map[string]string, len(pass.Files))
		for _, file := range pass.Files {
			path := pass.Fset.Position(file.Pos()).Filename
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			digest := sha256.Sum256(data)
			hashes[filepath.Base(path)] = hex.EncodeToString(digest[:])
		}
		// Synthetic evidence isolates analysistest source/machine joining.
		// Actual controlled SDK collection/replay has separate owner coverage.
		a := coefficientcodegen.Artifact{Function: "leaf", Build: closureenv.BinaryBuild{
			Package: pass.Pkg.Path(), GoVersion: "synthetic", GOOS: "darwin", GOARCH: "arm64", ArchitectureLevel: "v8.0", CGOEnabled: "0",
			MaterialSHA256: strings.Repeat("a", 64), ToolchainSHA256: strings.Repeat("b", 64), BinarySHA256: strings.Repeat("c", 64), SourceSHA256: hashes,
		}}
		for i := 0; i < 8; i++ {
			a.Loads = append(a.Loads, coefficientcodegen.Load{PC: uint64(8192 + 12*i), Page: 4096, Address: uint64(4096 + 16*i), Symbol: fmt.Sprintf("%s.c%d", pass.Pkg.Path(), i)})
		}
		data, err := json.Marshal(a)
		if err != nil {
			return nil, err
		}
		return runPS6130Artifacts(pass, []string{string(data)}, false, "ps6130/archsimd")
	}
	analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6130")
}

func TestPS6130SourceAndMachineBoundaries(t *testing.T) {
	t.Parallel()
	prefix := `package cpu
import "simd/archsimd"
type V = archsimd.Float64x2
var c0=archsimd.BroadcastFloat64x2(0);var c1=archsimd.BroadcastFloat64x2(1);var c2=archsimd.BroadcastFloat64x2(2);var c3=archsimd.BroadcastFloat64x2(3);var c4=archsimd.BroadcastFloat64x2(4)
var c5=archsimd.BroadcastFloat64x2(5);var c6=archsimd.BroadcastFloat64x2(6);var c7=archsimd.BroadcastFloat64x2(7);var c8=archsimd.BroadcastFloat64x2(8);var c9=archsimd.BroadcastFloat64x2(9)
func leaf(x V)V{a:=c0.MulAdd(x,c1);a=a.MulAdd(x,c2);a=a.MulAdd(x,c3);a=a.MulAdd(x,c4);b:=c5.MulAdd(x,c6);b=b.MulAdd(x,c7);b=b.MulAdd(x,c8);b=b.MulAdd(x,c9);return a.Div(b)}
`
	for _, tc := range []struct {
		name, caller, extra string
		loads               int
		want                int
	}{
		{"live", "func hot(x []V){for i:=range x{_=leaf(x[i])}}", "", 10, 1},
		{"fixed two", "func hot(x V){for i:=0;i<2;i++{_=leaf(x)}}", "", 10, 1},
		{"stride two", "func hot(x []V){upper:=len(x)&^1;for i:=0;i<upper;i+=2{_=leaf(x[i])}}", "", 10, 1},
		{"stride bound decrement", "func hot(x []V){upper:=len(x)&^1;upper--;for i:=0;i<upper;i+=2{_=leaf(x[i])}}", "", 10, 0},
		{"stride bound escape", "func hot(x []V){upper:=len(x)&^1;p:=&upper;_=p;for i:=0;i<upper;i+=2{_=leaf(x[i])}}", "", 10, 0},
		{"stride bound range assignment", "func hot(x []V){upper:=len(x)&^1;for upper=range x{};for i:=0;i<upper;i+=2{_=leaf(x[i])}}", "", 10, 0},
		{"one", "func hot(x V){for i:=0;i<1;i++{_=leaf(x)}}", "", 10, 0},
		{"break", "func hot(x V){for i:=0;i<2;i++{_=leaf(x);break}}", "", 10, 0},
		{"index write", "func hot(x V){for i:=0;i<2;i++{_=leaf(x);i=2}}", "", 10, 0},
		{"dead", "func hot(x []V){for i:=range x{if false{_=leaf(x[i])}}}", "", 10, 0},
		{"deferred", "func hot(x []V){for i:=range x{defer func(){_=leaf(x[i])}()}}", "", 10, 0},
		{"async", "func hot(x []V){for i:=range x{go func(){_=leaf(x[i])}()}}", "", 10, 0},
		{"writer", "func hot(x []V){for i:=range x{_=leaf(x[i])}}", "func write(){c0=c1}", 10, 0},
		{"range value writer", "func hot(x []V){for i:=range x{_=leaf(x[i])}}", "func write(x []V){for _,c0=range x{}}", 10, 0},
		{"closure writer", "func hot(x []V){for i:=range x{_=leaf(x[i])}}", "var writer=func(){c0=c1}", 10, 0},
		{"address escape", "func hot(x []V){for i:=range x{_=leaf(x[i])}}", "var escaped=&c0", 10, 0},
		{"raw linkname directive", "func hot(x []V){for i:=range x{_=leaf(x[i])}}", "//go:linkname c0 external\n", 10, 0},
		{"no machine evidence", "func hot(x []V){for i:=range x{_=leaf(x[i])}}", "", 0, 0},
		{"below threshold", "func hot(x []V){for i:=range x{_=leaf(x[i])}}", "", 7, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pass, a, diagnostics := ps6130TestPass(t, map[string]string{"fixture.go": prefix + tc.caller + "\n" + tc.extra})
			for i := 0; i < tc.loads; i++ {
				a.Loads = append(a.Loads, coefficientcodegen.Load{PC: uint64(0x4000 + 4*i), Page: 0x8000, Address: uint64(0x8000 + 16*i), Symbol: pass.Pkg.Path() + ".c" + string(rune('0'+i))})
			}
			if err := ps6130RunFixture(t, pass, a); err != nil {
				t.Fatal(err)
			}
			if len(*diagnostics) != tc.want {
				t.Fatalf("diagnostics=%+v want%d", *diagnostics, tc.want)
			}
		})
	}
}

func TestPS6130PurityRejectsGlobalEffects(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, declaration, body string
		want                    bool
	}{
		{"indexed", "var global [1]int", "global[0]=1;return x", false},
		{"field", "var global struct{N int}", "global.N=1;return x", false},
		{"pointer", "var global *int", "*global=1;return x", false},
		{"increment", "var global int", "global++;return x", false},
		{"indexed increment", "var global [1]int", "global[0]++;return x", false},
		{"field increment", "var global struct{N int}", "global.N++;return x", false},
		{"range key", "var global int", "for global=range [2]int{}{};return x", false},
		{"range value", "var global int", "for _,global=range [2]int{}{};return x", false},
		{"range indexed", "var global [1]int", "for _,global[0]=range [2]int{}{};return x", false},
		{"vector store", "var global [2]float64", "x.StoreArray(&global);return x", false},
		{"value copy", "", "local:=x;return local", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := "package cpu\nimport \"simd/archsimd\"\n" + tc.declaration + "\nfunc leaf(x archsimd.Float64x2)archsimd.Float64x2{" + tc.body + "}"
			pass, _, _ := ps6130TestPass(t, map[string]string{"fixture.go": source})
			decls := ps6115Declarations(pass)
			if got := ps6130Pure(pass, decls[pass.Pkg.Path()+".leaf"], decls, "simd/archsimd", map[*ast.FuncDecl]bool{}); got != tc.want {
				t.Fatalf("pure=%v want%v", got, tc.want)
			}
		})
	}
}

func TestPS6130ControlledOwnerReplay(t *testing.T) {
	t.Parallel()
	dir := os.Getenv("PERFSCAN_PS6130_OWNER_DIR")
	if dir == "" {
		t.Skip("optional local exact owner checkout")
	}
	request := closureenv.PackageRequest{Dir: dir, Pattern: "./backend/cpu"}
	a, err := coefficientcodegen.Collect(context.Background(), &request, "erfF64x2GELU")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &packages.Config{Dir: dir, Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedTypesSizes | packages.NeedImports | packages.NeedDeps | packages.NeedEmbedFiles}
	loaded, err := packages.Load(cfg, "./backend/cpu")
	if err != nil || len(loaded) != 1 || packages.PrintErrors(loaded) != 0 {
		t.Fatalf("load: %v", err)
	}
	pkg := loaded[0]
	diagnostics := []analysis.Diagnostic{}
	pass := &analysis.Pass{Analyzer: PS6130.Analyzer, Fset: pkg.Fset, Files: pkg.Syntax, OtherFiles: append(pkg.OtherFiles, pkg.EmbedFiles...), Pkg: pkg.Types, TypesInfo: pkg.TypesInfo, TypesSizes: pkg.TypesSizes, ReadFile: os.ReadFile, Report: func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) }}
	encoded, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runPS6130Artifacts(pass, []string{string(encoded)}, true, "simd/archsimd"); err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 {
		t.Fatalf("controlled owner diagnostics=%+v", diagnostics)
	}
	t.Log(diagnostics[0].Message)
}

func TestPS6130CompletePinnedOwner(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/ps6123_owner_v2.go")
	if err != nil {
		t.Fatal(err)
	}
	// Existing complete frozen source adds one harmless blank line.
	data = []byte(strings.TrimSuffix(string(data), "\n"))
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != "72c79a0bf18257f19c38206ae6f206f8aa1227803d8e881fc65d068192075c35" {
		t.Fatal("owner source drift")
	}
	stub := `package cpu
const cpuInvSqrt2Pi=0.3989422804014327
func expF64poly(float64)float64{return 0};func geluGradF64(float64,float64)float64{return 0}`
	pass, a, diagnostics := ps6130TestPass(t, map[string]string{"owner.go": string(data), "stub.go": stub})
	a.Function = "erfF64x2GELU"
	for i, name := range []string{"geluNVT0", "geluNVT1", "geluNVT2", "geluNVT3", "geluNVT4", "geluNVU1", "geluNVU2", "geluNVU3", "geluNVU4"} {
		a.Loads = append(a.Loads, coefficientcodegen.Load{PC: uint64(0x4000 + 4*i), Page: 0x8000, Address: uint64(0x8000 + 16*i), Symbol: pass.Pkg.Path() + "." + name})
	}
	if err := ps6130RunFixture(t, pass, a); err != nil {
		t.Fatal(err)
	}
	if len(*diagnostics) != 1 {
		t.Fatalf("owner diagnostics=%+v", *diagnostics)
	}
	a.Build.SourceSHA256["owner.go"] = strings.Repeat("f", 64)
	if err := ps6130RunFixture(t, pass, a); err == nil {
		t.Fatal("accepted stale source")
	}
}
