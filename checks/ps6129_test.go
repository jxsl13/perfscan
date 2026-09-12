package checks

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6129AdversarialProofs(t *testing.T) {
	t.Parallel()
	base := `package adversary
func gemm(a,b,c []float32,start,end,depth,columns int){}
func unknown(){}
func f(n,seq,live,width int,b,out []float32){
a:=make([]float32,n*seq)
for r:=range n {clear(a[r*seq+live:(r+1)*seq])}
gemm(a,b,out,0,n,seq,width)
}`
	for _, tc := range []struct{ name, source string }{
		{"pointer_escape", strings.Replace(base, "for r:=", "pointer:= &(a[0]); _=pointer\nfor r:=", 1)},
		{"intervening_call", strings.Replace(base, "gemm(a,b,out", "unknown(); gemm(a,b,out", 1)},
		{"partial_clear", strings.Replace(base, "range n {clear", "range n-1 {clear", 1)},
		{"short_suffix", strings.Replace(base, "(r+1)*seq]", "(r+1)*seq-1]", 1)},
		{"conditional_clear", strings.Replace(base, "for r:=range n {clear(a[r*seq+live:(r+1)*seq])}", "for r:=range n {if r%2==0 {clear(a[r*seq+live:(r+1)*seq])}}", 1)},
		{"function_value", strings.Replace(base, "gemm(a,b,out", "indirect:=gemm; indirect(a,b,out", 1)},
		{"rebound_geometry", strings.Replace(base, "gemm(a,b,out", "live=seq; gemm(a,b,out", 1)},
		{"addressed_geometry", strings.Replace(base, "a:=make", "pointer:= &live; _=pointer\na:=make", 1)},
		{"parenthesized_addressed_geometry", strings.Replace(base, "a:=make", "pointer:= &((live)); _=pointer\na:=make", 1)},
		{"mixed_short_declaration_geometry", strings.Replace(base, "gemm(a,b,out", "seq,scratch:=live,0;_=scratch;gemm(a,b,out", 1)},
		{"slice_alias", strings.Replace(base, "for r:=", "alias:=a[:]; _=alias\nfor r:=", 1)},
		{"concurrent_mutation", strings.Replace(base, "for r:=", "go unknown()\nfor r:=", 1)},
		{"wrong_signature", strings.ReplaceAll(base, "float32", "int")},
		{"no_live_suffix", strings.ReplaceAll(base, "r*seq+live", "r*seq+seq")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir, cleanup, err := analysistest.WriteFiles(map[string]string{"adversary/adversary.go": tc.source})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(cleanup)
			analyzer := &analysis.Analyzer{Name: "PS6129", Doc: "negative", Run: func(pass *analysis.Pass) (any, error) {
				return runPS6129WithFuncs(pass, map[string]bool{"adversary.gemm": true})
			}}
			analysistest.Run(t, dir, analyzer, "adversary")
		})
	}
}

func TestPS6129(t *testing.T) {
	t.Parallel()
	analyzer := &analysis.Analyzer{Name: "PS6129", Doc: "test", Run: func(pass *analysis.Pass) (any, error) {
		return runPS6129WithFuncs(pass, map[string]bool{"ps6129.gemm": true})
	}}
	analysistest.Run(t, analysistest.TestData(), analyzer, "ps6129")
}

func TestPS6129SilentWithoutVocabulary(t *testing.T) {
	t.Parallel()
	dir, cleanup, err := analysistest.WriteFiles(map[string]string{"silent/silent.go": `package silent
func gemm(a,b,c []float32,start,end,depth,columns int){}
func f(n,seq,live,width int,b,out []float32){
a:=make([]float32,n*seq)
for r:=range n {clear(a[r*seq+live:(r+1)*seq])}
gemm(a,b,out,0,n,seq,width)
}`})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	analyzer := &analysis.Analyzer{Name: "PS6129", Doc: "test", Run: func(pass *analysis.Pass) (any, error) { return runPS6129WithFuncs(pass, nil) }}
	analysistest.Run(t, dir, analyzer, "silent")
}

func TestPS6129Metadata(t *testing.T) {
	t.Parallel()
	if PS6129.AutoFix || !PS6129.NeedsConfig || PS6129.Level != 3 || len(PS6129.Vocab) != 2 || PS6129.Vocab[0] != "denseRowGEMMFuncs" || PS6129.Vocab[1] != "causalZeroGEMMContracts" {
		t.Fatalf("metadata drift: %+v", PS6129)
	}
	original := config.Config{DenseRowGEMMFuncs: []string{"example.org/math.gemm"}}
	compiled := original.Compile()
	original.DenseRowGEMMFuncs[0] = "changed"
	if !compiled.DenseRowGEMMFuncs["example.org/math.gemm"] || compiled.DenseRowGEMMFuncs["changed"] {
		t.Fatal("vocabulary aliases input")
	}
}
