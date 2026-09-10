package checks

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

// The #839 candidate is the exact #843 baseline: the paired row was already
// coarsened, but the independent row still stages 16 coefficients per block.
// These are complete, byte-pinned owner wrappers, not a synthetic rewrite.
func TestPS6084WholeRowOwner(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, file, digest string
		want               int
	}{
		{"before", "ps6084_owner_candidate.go.txt", "fe4adba5e1477b2b3cf0971dd7592a1d78f746fdee4b2639aaf3f7513e2d1e72", 1},
		{"after", "ps6084_owner_whole_row.go.txt", "73b672ad5a7b8c3617a79bec8c062a00a58b55693b1af2cf9c35408d4f8339c2", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile("testdata/" + tc.file)
			if err != nil {
				t.Fatal(err)
			}
			if digest := fmt.Sprintf("%x", sha256.Sum256(data)); digest != tc.digest {
				t.Fatalf("owner wrapper digest=%s want=%s", digest, tc.digest)
			}
			// Only dependencies absent from the complete wrapper are stubbed. The
			// half conversion retains the actual single-return table-lookup shape;
			// the init arithmetic is not a numerical oracle or a benchmark.
			const support = `
const qkK=256; const q4kBlockSize=144
var qKByteToF32Indexes [256]byte
var f16Table [1<<16]float32
func init(){for i:=range f16Table{f16Table[i]=float32(i)}}
func f16ToF32(h uint16) float32 {return f16Table[h]}
var dotQ4KRowFn func([]float32,[]byte,int)float64
var dotQ4KPairRowFn func([]float32,[]byte,[]byte,int)(float64,float64)
`
			got := ps6084WholeRowDiagnostics(t, string(data)+support)
			if len(got) != tc.want {
				t.Fatalf("whole-row diagnostics=%v want count=%d", got, tc.want)
			}
			if tc.want == 1 {
				if !strings.Contains(got[0].Message, "dotQ4KBlockNeon") || strings.Contains(got[0].Message, "dotQ4KPairBlockNeon") {
					t.Fatalf("only independent block staging should report: %v", got)
				}
				ps6084RequireWholeRowGuardrails(t, got[0].Message)
			}
		})
	}
}

func TestPS6084WholeRowClassicGuardrails(t *testing.T) {
	t.Parallel()
	for _, repeated := range []bool{false, true} {
		t.Run(fmt.Sprintf("repeated=%t", repeated), func(t *testing.T) {
			t.Parallel()
			body := `var coefficients [4]float32
for field := range coefficients { coefficients[field] = float32(packed[field]) }
blockLeaf(&packed[0], &coefficients[0])`
			if repeated {
				body = "for block := 0; block < count; block++ {" + body + "}"
			}
			source := "package p\n//go:noescape\nfunc blockLeaf(*byte, *float32)\nfunc run(packed []byte, count int) {" + body + "}"
			got := ps6084WholeRowDiagnostics(t, source)
			if len(got) != 1 {
				t.Fatalf("classic diagnostics=%v want count=1", got)
			}
			if repeated {
				ps6084RequireWholeRowGuardrails(t, got[0].Message)
			} else if strings.Contains(got[0].Message, "zero-length fast path") {
				t.Fatalf("non-repeated block advice must not claim whole-row orchestration: %v", got)
			}
		})
	}
}

func ps6084RequireWholeRowGuardrails(t *testing.T, message string) {
	t.Helper()
	for _, required := range []string{
		"exact block iteration order", "per-block reduction boundaries",
		"zero-length fast path before element pointers", "architecture-specific routing",
		"minimum-supported-compiler builds", "retained end-to-end benchmarks",
		"Noescape alone does not prove allocation freedom or native purity",
	} {
		if !strings.Contains(message, required) {
			t.Errorf("whole-row diagnostic lacks %q: %s", required, message)
		}
	}
}

// Collect every PS6084 diagnostic, including the original non-grouped path.
// Filtering only the grouped message could hide a new false positive after
// the owner moved both rows across the native boundary.
func ps6084WholeRowDiagnostics(t *testing.T, source string) []analysis.Diagnostic {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "owner.go", source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	sizes := types.SizesFor("gc", "arm64")
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{},
		Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{},
		Instances: map[*ast.Ident]types.Instance{},
	}
	pkg, err := (&types.Config{Importer: importer.Default(), Sizes: sizes}).Check("owner/whole-row", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	var diagnostics []analysis.Diagnostic
	pass := &analysis.Pass{
		Analyzer: PS6084.Analyzer, Fset: fset, Files: []*ast.File{file}, Pkg: pkg,
		TypesInfo: info, TypesSizes: sizes,
		Report: func(d analysis.Diagnostic) {
			if len(d.SuggestedFixes) != 0 {
				t.Error("whole-row orchestration must remain advisory")
			}
			diagnostics = append(diagnostics, d)
		},
	}
	if _, err := runPS6084(pass); err != nil {
		t.Fatal(err)
	}
	return diagnostics
}
