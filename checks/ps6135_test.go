package checks

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

// These are type scaffolding only. The complete capability and wrapper bodies
// are frozen owner source; opaque provider semantics come from the contract.
const ps6135OwnerTypes = `package owner
type buffer interface{}
type recorder interface{Binary(a,b,o buffer,op int)error}
type Decoder struct{}
`

func ps6135Contract(packagePath string) config.ActiveBoundFallbackContract {
	return config.ActiveBoundFallbackContract{
		WrapperMethod: packagePath + ".Decoder.binElem", ProviderType: packagePath + ".recorder", CapabilityType: packagePath + ".binaryNRecorder",
		BoundedMethod: "BinaryN", CapacityMethod: "Binary", ProviderArgument: 0, BufferArguments: []int{1, 2, 3}, OperationArgument: 4, RowsArgument: 5, WidthArgument: 6,
		BoundedActivePrefixReviewed: true, FallbackCapacityWideReviewed: true, SameOperationBufferRolesReviewed: true,
		ActiveRowsMayBeBelowCapacityReviewed: true, InactiveTailUnobservedReviewed: true, ProviderAndBuildPathsReviewed: true, ErrorsShapeAndSynchronizationReviewed: true,
	}
}

func TestPS6135PinnedOwnerAndAdversarialFlow(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/ps6135_owner_bin_elem.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != "1def3075b0c298ac1d2a6050820bc6953a4400ae6ccd58cb4fc0e8615555e05a" {
		t.Fatalf("owner digest=%s", got)
	}
	owner := string(data)
	for _, tc := range []struct {
		name, source string
		want         int
		change       func(*config.ActiveBoundFallbackContract)
	}{
		{"owner", ps6135OwnerTypes + owner, 1, nil},
		{"generic wrapper", strings.Replace(ps6135OwnerTypes, "type Decoder struct{}", "type Decoder[T any] struct{}", 1) + strings.Replace(owner, "(d *Decoder)", "(d *Decoder[T])", 1), 0, nil},
		{"variadic wrapper", ps6135OwnerTypes + strings.Replace(owner, "op, rows, width int)", "op, rows, width int,extra ...int)", 1), 0, nil},
		{"variadic bounded API", ps6135OwnerTypes + strings.Replace(owner, "op, n int) error", "op int,n ...int) error", 1), 0, nil},
		{"non-error result", strings.ReplaceAll(ps6135OwnerTypes+owner, "error", "*customError") + `type customError struct{};func(*customError)Error()string{return ""}`, 0, nil},
		{"commuted product", ps6135OwnerTypes + strings.Replace(owner, "rows*width", "width*rows", 1), 1, nil},
		{"canonical alias", ps6135OwnerTypes + strings.Replace(owner, "r.(binaryNRecorder)", "r.(boundedAlias)", 1) + "type boundedAlias = binaryNRecorder\n", 1, nil},
		{"missing width guard", ps6135OwnerTypes + strings.Replace(owner, " && width > 0", "", 1), 0, nil},
		{"missing rows guard", ps6135OwnerTypes + strings.Replace(owner, " && rows > 0", "", 1), 0, nil},
		{"inclusive guard", ps6135OwnerTypes + strings.Replace(owner, "rows > 0", "rows >= 0", 1), 0, nil},
		{"wrong shape guard", ps6135OwnerTypes + strings.Replace(owner, "width > 0", "width > 1", 1), 0, nil},
		{"or guard", ps6135OwnerTypes + strings.Replace(owner, "ok && rows > 0", "ok || rows > 0", 1), 0, nil},
		{"zero product", ps6135OwnerTypes + strings.Replace(owner, "rows*width", "rows*0", 1), 0, nil},
		{"wrong product", ps6135OwnerTypes + strings.Replace(owner, "rows*width", "rows*rows", 1), 0, nil},
		{"changed product", ps6135OwnerTypes + strings.Replace(owner, "rows*width", "(rows+1)*width", 1), 0, nil},
		{"changed buffer", ps6135OwnerTypes + strings.Replace(owner, "r.Binary(a, b, o, op)", "r.Binary(b, a, o, op)", 1), 0, nil},
		{"changed bounded buffer", ps6135OwnerTypes + strings.Replace(owner, "bn.BinaryN(a, b, o, op, rows*width)", "bn.BinaryN(a, o, b, op, rows*width)", 1), 0, nil},
		{"changed operation", ps6135OwnerTypes + strings.Replace(owner, "r.Binary(a, b, o, op)", "r.Binary(a, b, o, op+1)", 1), 0, nil},
		{"provider rebind", ps6135OwnerTypes + strings.Replace(owner, "if bn, ok :=", "r=nil;if bn, ok :=", 1), 0, nil},
		{"mixed rows rebind", ps6135OwnerTypes + strings.Replace(owner, "if bn, ok :=", "rows,z:=1,0;_=z;if bn, ok :=", 1), 0, nil},
		{"range width write", ps6135OwnerTypes + strings.Replace(owner, "if bn, ok :=", "for width=range 2{};if bn, ok :=", 1), 0, nil},
		{"address exposure", ps6135OwnerTypes + strings.Replace(owner, "if bn, ok :=", "p:=&rows;_=p;if bn, ok :=", 1), 0, nil},
		{"receiver alias", ps6135OwnerTypes + strings.Replace(owner, "if bn, ok :=", "alias:=r;_=alias;if bn, ok :=", 1), 0, nil},
		{"indirect bounded call", ps6135OwnerTypes + strings.Replace(owner, "return bn.BinaryN(a, b, o, op, rows*width)", "fn:=bn.BinaryN;return fn(a,b,o,op,rows*width)", 1), 0, nil},
		{"indirect fallback", ps6135OwnerTypes + strings.Replace(owner, "return r.Binary(a, b, o, op)", "fn:=r.Binary;return fn(a,b,o,op)", 1), 0, nil},
		{"capability not optional", strings.Replace(ps6135OwnerTypes, "Binary(a,b,o buffer,op int)error", "Binary(a,b,o buffer,op int)error;BinaryN(a,b,o buffer,op,n int)error", 1) + owner, 0, nil},
		{"wrong capability identity", ps6135OwnerTypes + strings.Replace(owner, "r.(binaryNRecorder)", "r.(other)", 1) + "type other interface{BinaryN(a,b,o buffer,op,n int)error}\n", 0, nil},
		{"provider role mismatch", ps6135OwnerTypes + owner, 0, func(c *config.ActiveBoundFallbackContract) { c.ProviderArgument = 1 }},
		{"bad role duplicate", ps6135OwnerTypes + owner, 0, func(c *config.ActiveBoundFallbackContract) { c.RowsArgument = c.WidthArgument }},
		{"unreviewed capacity", ps6135OwnerTypes + owner, 0, func(c *config.ActiveBoundFallbackContract) { c.FallbackCapacityWideReviewed = false }},
		{"unreviewed tail", ps6135OwnerTypes + owner, 0, func(c *config.ActiveBoundFallbackContract) { c.InactiveTailUnobservedReviewed = false }},
		{"unreviewed shapes/errors", ps6135OwnerTypes + owner, 0, func(c *config.ActiveBoundFallbackContract) { c.ErrorsShapeAndSynchronizationReviewed = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := ps6135Contract("owner")
			if tc.change != nil {
				tc.change(&c)
			}
			dir, cleanup, err := analysistest.WriteFiles(map[string]string{"owner/owner.go": tc.source})
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			analyzer := *PS6135.Analyzer
			analyzer.Run = func(pass *analysis.Pass) (any, error) {
				n := 0
				pass.Report = func(d analysis.Diagnostic) {
					n++
					if len(d.SuggestedFixes) != 0 {
						t.Error("unsafe automatic rewrite")
					}
					if !strings.Contains(d.Message, "positive") || !strings.Contains(d.Message, "capacity") {
						t.Error("missing conditional workload scope")
					}
				}
				result, err := runPS6135WithContracts(pass, []config.ActiveBoundFallbackContract{c})
				if n != tc.want {
					t.Errorf("findings=%d want%d", n, tc.want)
				}
				return result, err
			}
			analysistest.Run(t, dir, &analyzer, "owner")
		})
	}
}

func TestPS6135ConfigIsolationAndAmbiguity(t *testing.T) {
	t.Parallel()
	c := ps6135Contract("owner")
	cfg := config.Config{ActiveBoundFallbackContracts: []config.ActiveBoundFallbackContract{c}}
	compiled := cfg.Compile()
	cfg.ActiveBoundFallbackContracts[0].BufferArguments[0] = 9
	if compiled.ActiveBoundFallbackContracts[0].BufferArguments[0] != 1 {
		t.Fatal("compiled roles alias config")
	}
	if config.UsableActiveBoundFallbackContractCount([]config.ActiveBoundFallbackContract{ps6135Contract("owner")}) != 1 {
		t.Fatal("valid contract rejected")
	}
	good := ps6135Contract("owner")
	if config.UsableActiveBoundFallbackContractCount([]config.ActiveBoundFallbackContract{good, good}) != 0 {
		t.Fatal("duplicate wrapper accepted")
	}
	for _, change := range []func(*config.ActiveBoundFallbackContract){
		func(c *config.ActiveBoundFallbackContract) { c.ProviderArgument = 7 },
		func(c *config.ActiveBoundFallbackContract) { c.OperationArgument = -1 },
		func(c *config.ActiveBoundFallbackContract) { c.RowsArgument = 8 },
		func(c *config.ActiveBoundFallbackContract) { c.WidthArgument = -1 },
		func(c *config.ActiveBoundFallbackContract) { c.BufferArguments[0] = 9 },
		func(c *config.ActiveBoundFallbackContract) { c.BufferArguments[2] = -1 },
	} {
		invalid := ps6135Contract("owner")
		change(&invalid)
		if invalid.Valid() || config.UsableActiveBoundFallbackContractCount([]config.ActiveBoundFallbackContract{invalid}) != 0 {
			t.Fatal("impossible role counted as usable")
		}
	}
	if PS6135.AutoFix || !PS6135.NeedsConfig {
		t.Fatal("unsafe metadata")
	}
}

func TestPS6135AnalysisFixture(t *testing.T) {
	t.Parallel()
	analyzer := *PS6135.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6135WithContracts(pass, []config.ActiveBoundFallbackContract{ps6135Contract("ps6135")})
	}
	analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6135")
}
