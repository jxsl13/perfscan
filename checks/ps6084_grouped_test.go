package checks

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/analysis"
	"os"
	"strings"
	"testing"
)

// Synthetic bounded architecture probes, not runtime benchmarks.
func TestPS6084GroupedArchitecture(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                                                         string
		want                                                         int
		extra, prefix, decl, outer, inside, fill, call, suffix, wrap string
	}{
		{`header boundary live`, 1, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := range 4 { c0[j] = float32(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`whole root live`, 1, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := range 4 { c0[j] = float32(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[0], &p1[0], &c0[0], &c1[0])`, ``, ``},
		{`offset within header rejected`, 0, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := range 4 { c0[j] = float32(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[1], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`offset beyond proved header rejected`, 0, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := range 4 { c0[j] = float32(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[5], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`different packed subpath rejected`, 0, ``, `h0:=p0[8:12]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := range 4 { c0[j] = float32(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`unpassed third packed source rejected`, 0, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := range 4 { c0[j] = float32(h0[j])+float32(p2[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`single return parameter pure helper live`, 1, `func decode(v byte) float32 { return float32(v) }`, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := range 4 { c0[j] = decode(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`helper mutable global dependency rejected`, 0, `var hidden float32; func change(){hidden++}; func decode(v byte) float32 { return hidden }`, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := range 4 { c0[j] = decode(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`ignored helper argument is not value provenance`, 0, `func decode(v byte) float32 { return 3 }`, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := range 4 { c0[j] = decode(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`opaque RHS rejected`, 0, `func decode(v byte) float32`, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := range 4 { c0[j] = decode(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`exact little endian helper live`, 1, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := range 4 { c0[j] = float32(binary.LittleEndian.Uint16(h0[0:2])); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`interface ByteOrder call not proven pure`, 0, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := range 4 { c0[j] = float32(order.Uint16(h0[0:2])); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`compound writes are reads not overwrites`, 0, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := range 4 { c0[j] += float32(h0[j]); c1[j] += float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`index version must precede store`, 0, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := range 4 { idx:=0; c0[idx]=float32(h0[j]); idx=j; c1[j]=float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`RHS version must precede store`, 0, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := range 4 { value:=p2[j]; c0[j]=float32(value); value=h0[j]; c1[j]=float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`branch assignment does not dominate fill`, 0, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, `value:=p2[0]; if flag {value=h0[0]}`, `for j := range 4 { c0[j] = float32(value); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`unmodeled global write in fill rejected`, 0, `var sideCount float32; func side()float32{sideCount++;return sideCount}`, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := range 4 { sideCount=side(); c0[j] = float32(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`nonzero scratch initialization rejected`, 0, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `c0,c1:=[4]float32{1},[4]float32{2}`, `for block:=0;block<trips;block++`, ``, `for j := range 4 { c0[j] = float32(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`nonzero for start cannot enumerate as zero`, 0, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := 1; j < 5; j++ { c0[j] = float32(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`nonzero for start correctly shifted live`, 1, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := 1; j < 5; j++ { c0[j-1] = float32(h0[j-1]); c1[j-1] = float32(h1[j-1]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`stride cannot enumerate as unit increment`, 0, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := 0; j < 8; j += 2 { c0[j] = float32(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`exact two outer trips live`, 1, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<2;block++`, ``, `for j := range 4 { c0[j] = float32(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`body mutation prevents second consumer`, 0, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<2;block++`, ``, `for j := range 4 { c0[j] = float32(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, `block=2`, ``},
		{`uint8 wrap prevents second consumer`, 0, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=uint8(255);block>0;block++`, ``, `for j := range 4 { c0[j] = float32(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`break prevents second consumer`, 0, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := range 4 { c0[j] = float32(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, `break`, ``},
		{`nil receive prefix rejected`, 0, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, `<-chan int(nil)`, `for j := range 4 { c0[j] = float32(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`nil receive suffix rejected`, 0, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := range 4 { c0[j] = float32(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, `<-chan int(nil)`, ``},
		{`blocking IIFE prefix rejected`, 0, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, `func(){select{}}()`, `for j := range 4 { c0[j] = float32(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`blocking IIFE suffix rejected`, 0, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := range 4 { c0[j] = float32(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, `func(){select{}}()`, ``},
		{`returning IIFE prefix live`, 1, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, `func(){return}()`, `for j := range 4 { c0[j] = float32(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, ``},
		{`stored uninvoked region silent`, 0, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := range 4 { c0[j] = float32(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, `stored`},
		{`synchronous IIFE region current unsupported`, 0, ``, `h0:=p0[:4]; h1:=p1[:4]; _=h0; _=h1`, `var c0,c1 [4]float32`, `for block:=0;block<trips;block++`, ``, `for j := range 4 { c0[j] = float32(h0[j]); c1[j] = float32(h1[j]) }`, `_, _ = leaf(&p0[4], &p1[4], &c0[0], &c1[0])`, ``, `iife`},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := tc.decl + ";" + tc.prefix + ";" + tc.outer + "{" + tc.inside + ";" + tc.fill + ";" + tc.call + ";" + tc.suffix + "}"
			if tc.wrap == "stored" {
				body = "f:=func(){" + body + "};_=f"
			}
			if tc.wrap == "iife" {
				body = "func(){" + body + "}()"
			}
			source := "package p\nimport \"encoding/binary\"\n//go:noescape\nfunc leaf(a,b *byte,c,d *float32)(float32,float32)\n" + tc.extra + "\nfunc run(p0,p1,p2 []byte,trips int,flag bool,order binary.ByteOrder){" + body + "}"
			got := ps6084GroupedDiagnostics(t, source)
			if len(got) != tc.want {
				t.Fatalf("grouped diagnostics=%d want=%d: %v", len(got), tc.want, got)
			}
		})
	}
}

func TestPS6084GroupedCompleteOwner(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/ps6084_owner_baseline.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	if digest := fmt.Sprintf("%x", sha256.Sum256(data)); digest != "ae5b05ea5bf9259d458deed2225bb55a8228aa7d1ecf458e9fd071f4391ab79a" {
		t.Fatalf("owner baseline digest=%s", digest)
	}
	// Preserve every byte of the pinned wrapper. The half conversion has the actual
	// pinned single-return lookup shape, rather than the author's numeric-cast stub.
	const support = `
 const qkK=256;const q4kBlockSize=144
 var qKByteToF32Indexes [256]byte
 var f16Table [1<<16]float32
 func init(){for i:=range f16Table{f16Table[i]=f16ToF32bits(uint16(i))}}
 func f16ToF32(h uint16) float32 {return f16Table[h]}
 func f16ToF32bits(h uint16)float32{return float32(h)}
 var dotQ4KRowFn func([]float32,[]byte,int)float64
 var dotQ4KPairRowFn func([]float32,[]byte,[]byte,int)(float64,float64)
 `
	got := ps6084GroupedDiagnostics(t, string(data)+support)
	if len(got) != 2 {
		t.Fatalf("complete owner grouped diagnostics=%d want=2 (single and pair): %v", len(got), got)
	}
	pair := 0
	single := 0
	for _, message := range got {
		if strings.Contains(message, "dotQ4KPairBlockNeon") {
			pair++
		}
		if strings.Contains(message, "dotQ4KBlockNeon") {
			single++
		}
	}
	if pair != 1 || single != 1 {
		t.Fatalf("owner deduplication pair=%d single=%d", pair, single)
	}
}

func TestPS6084GroupedCompleteCandidateSilent(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/ps6084_owner_candidate.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	if digest := fmt.Sprintf("%x", sha256.Sum256(data)); digest != "fe4adba5e1477b2b3cf0971dd7592a1d78f746fdee4b2639aaf3f7513e2d1e72" {
		t.Fatalf("owner candidate digest=%s", digest)
	}
	const support = `
 const qkK=256;const q4kBlockSize=144
 var qKByteToF32Indexes [256]byte
 var f16Table [1<<16]float32
 func init(){for i:=range f16Table{f16Table[i]=f16ToF32bits(uint16(i))}}
 func f16ToF32(h uint16) float32 {return f16Table[h]}
 func f16ToF32bits(h uint16)float32{return float32(h)}
 var dotQ4KRowFn func([]float32,[]byte,int)float64
 var dotQ4KPairRowFn func([]float32,[]byte,[]byte,int)(float64,float64)
 `
	got := ps6084GroupedDiagnostics(t, string(data)+support)
	if len(got) != 1 || !strings.Contains(got[0], "dotQ4KBlockNeon") || strings.Contains(got[0], "dotQ4KPairBlockNeon") {
		t.Fatalf("complete owner candidate must retain only the unchanged single-row finding: %v", got)
	}
}

func TestPS6084GroupedInitOnlyTable(t *testing.T) {
	t.Parallel()
	const source = `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)
var table [256]float32
func init(){for i:=range table{table[i]=float32(i)}}
func decode(v byte)float32{return table[v]}
func run(p0,p1 []byte,trips int){for block:=0;block<trips;block++{var c0,c1 [4]float32;for j:=range 4{c0[j]=decode(p0[j]);c1[j]=decode(p1[j])};_,_=leaf(&p0[0],&p1[0],&c0[0],&c1[0])}}
`
	if got := ps6084GroupedDiagnostics(t, source); len(got) != 1 {
		t.Fatalf("init-only table grouped diagnostics=%d want=1: %v", len(got), got)
	}
}

func ps6084GroupedDiagnostics(t *testing.T, source string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "review.go", source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	sizes := types.SizesFor("gc", "arm64")
	if sizes == nil {
		t.Fatal("no explicit ARM64 sizes")
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}, Instances: map[*ast.Ident]types.Instance{}}
	pkg, err := (&types.Config{Importer: importer.Default(), Sizes: sizes}).Check("review/grouped", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	var messages []string
	pass := &analysis.Pass{Analyzer: PS6084.Analyzer, Fset: fset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info, TypesSizes: sizes, Report: func(d analysis.Diagnostic) {
		if len(d.SuggestedFixes) != 0 {
			t.Error("unexpected automatic fix")
		}
		if strings.Contains(d.Message, "fixed scratch arrays") {
			messages = append(messages, d.Message)
		}
	}}
	if _, err := runPS6084(pass); err != nil {
		t.Fatal(err)
	}
	return messages
}
