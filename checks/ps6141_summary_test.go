package checks

import (
	"fmt"
	"go/types"
	"strings"
	"testing"
)

func TestPS6141SummaryProtocolNamespace(t *testing.T) {
	t.Parallel()
	for _, count := range []int{999, 1001} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			t.Parallel()
			args := make([]string, count)
			for i := range args {
				args[i] = fmt.Sprintf("a%d int8", i+1)
			}
			source := fmt.Sprintf("package fixture;func wide(%s)int8{return a%d};func owner(){}", strings.Join(args, ","), count)
			pass, _ := ps6141TypedFixture(t, source)
			declarations := ps6099LocalFunctionDeclarations(pass)
			index := &ps6141SummaryIndex{pass: pass, declarations: declarations, memo: map[*types.Func]ps6141SourceSummary{}, active: map[*types.Func]bool{}, remaining: 20000}
			found := false
			for fn := range declarations {
				if fn.Name() == "wide" {
					found = true
					summary := index.function(fn)
					want := count < 1000
					if summary.valid != want {
						t.Fatalf("formal namespace summary=%+v wantvalid=%v", summary, want)
					}
					if want && (!summary.resultDeps[ps6141Root(count)] || summary.resultDeps[3001]) {
						t.Fatalf("formal origin overlaps protocol namespace: %+v", summary)
					}
				}
			}
			if !found {
				t.Fatal("typed prerequisite missing")
			}
		})
	}
}

// Synthetic typed multi-function packed-layout evidence, not a retained Q8_K
// owner pilot, numerical oracle or whole-boundary performance measurement.
const ps6141SummarySynthetic = `package fixture
type Block struct { Q int8; Scale float32; Sum int16 }
type Packed []Block
var cache Packed
func storage(n int) Packed {p:=make(Packed,n);return p}
func fill(p Packed,x []float32) {for i:=range x {p[i].Scale=x[i]*0.125;p[i].Q=int8(x[i]*8);p[i].Sum=int16(x[i])+3}}
func pack(x []float32) Packed {p:=storage(len(x));fill(p,x);return p}
func leaf(p,w Packed) float32 {if len(p)!=len(w){panic("shape")};sum:=float32(0);for i:=range p {sum+=float32(p[i].Q)*float32(w[i].Q)*p[i].Scale*w[i].Scale};return sum}
func dot(p,w Packed) float32 {return leaf(p,w)}
func owner(x []float32,w Packed,m,n int) float32 {p:=pack(x);return dot(p,w)}
`

func TestPS6141SourceSummaryComposition(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, old, new string
		want           int
	}{
		{"namedBlocksAndArbitraryArithmetic", "", "", 1},
		{"extraWrapper", "return leaf(p,w)", "return wrapper(p,w)};func wrapper(p,w Packed) float32{return leaf(p,w)", 1},
		{"scalarArithmeticHelper", "int8(x[i]*8)", "int8(scale(x[i]))", 1},
		{"freshForwardingHelper", "p:=storage(len(x))", "p:=allocate(len(x))", 1},
		{"hiddenFanout", "return leaf(p,w)", "return leaf(p,w)+leaf(p,w)", 0},
		{"aliasedPackedArguments", "return leaf(p,w)", "return leaf(p,p)", 0},
		{"repeatedTraversal", "return sum}", "for i:=range p{sum+=float32(p[i].Q)*float32(w[i].Q)};return sum}", 0},
		{"NRowsAtMOne", "return leaf(p,w)", "sum:=float32(0);for row:=0;row<4096;row++{sum+=leaf(p,w)};return sum", 0},
		{"dynamicNRows", "return dot(p,w)", "m=1;sum:=float32(0);for row:=0;row<n*m;row++{sum+=dot(p,w)};return sum", 0},
		{"scratchOrigin", "p:=make(Packed,n)", "p:=cache", 0},
		{"cachedReturn", "p:=make(Packed,n);return p", "return cache", 0},
		{"independentReductions", "sum+=float32(p[i].Q)*float32(w[i].Q)*p[i].Scale*w[i].Scale", "sum+=float32(p[i].Q)*float32(w[i].Q);sum+=p[i].Scale*w[i].Scale", 0},
		{"inputMutation", "for i:=range x {", "for i:=range x {x[i]=0;", 0},
		{"fillRetains", "for i:=range x {", "cache=p;for i:=range x {", 0},
		{"consumerRetains", "return leaf(p,w)", "cache=p;return leaf(p,w)", 0},
		{"producerPanicEscapes", "fill(p,x);return p", "fill(p,x);if len(x)>0{panic(p)};return p", 0},
		{"consumerPanicEscapes", "return leaf(p,w)", "if len(p)>0{panic(p)};return leaf(p,w)", 0},
		{"constantPackedIndexReuse", "float32(p[i].Q)", "float32(p[0].Q)", 0},
		{"mutatedTraversalIndex", "for i:=range p {", "for i:=range p {i=0;", 0},
		{"compoundTraversalIndexMutation", "for i:=range p {", "for i:=range p {i+=1;", 0},
		{"helperReturnedTraversalIndex", "for i:=range p {", "for i:=range p {i=reset(i);", 0},
		{"pointerHelperTraversalMutation", "for i:=range p {", "for i:=range p {setIndex(&i);", 0},
		{"scalarIndexAlias", "float32(p[i].Q)", "float32(p[identity(i)].Q)", 0},
		{"scalarAliasReassignment", "for i:=range p {", "for i:=range p {j:=i;j=0;i=j;", 0},
		{"differentScalarIndexObject", "sum+=float32(p[i].Q)", "j:=i;sum+=float32(p[j].Q)", 0},
		{"reassignedScalarIndexAlias", "sum+=float32(p[i].Q)", "j:=i;j=0;sum+=float32(p[j].Q)", 0},
		{"producerTraversalIndexMutation", "for i:=range x {", "for i:=range x {i=0;", 0},
		{"differentPackedIndexReuse", "float32(p[i].Q)", "float32(p[i+1].Q)", 0},
		{"opaquePackedIndex", "float32(p[i].Q)", "float32(p[opaqueIndex()].Q)", 0},
		{"consumerMutates", "for i:=range p {", "for i:=range p {p[i].Q=0;", 0},
		{"conflictingOrigins", "p:=storage(len(x));fill(p,x);return p", "if len(x)>0{return storage(len(x))};return cache", 0},
		{"returnedInputAlias", "p:=storage(len(x));fill(p,x);return p", "p:=cache;fill(p,x);return p", 0},
		{"closureCapture", "fill(p,x);return p", "fill(p,x);defer func(){cache=p}();return p", 0},
		{"addressExposure", "fill(p,x);return p", "fill(p,x);expose(&p[0]);return p", 0},
		{"storageAlias", "fill(p,x);return p", "alias:=p;fill(alias,x);return p", 0},
		{"producerRecursion", "p:=storage(len(x));fill(p,x);return p", "return pack(x)", 0},
		{"consumerRecursion", "return leaf(p,w)", "return dot(p,w)", 0},
		{"allocationInLoop", "p:=storage(len(x));fill(p,x);return p", "for i:=range x{p:=storage(i);fill(p,x);return p};return cache", 0},
		{"fusedBoundary", "p:=pack(x);return dot(p,w)", "return fused(x,w)", 0},
		{"opaqueHelper", "fill(p,x);return p", "opaque(p,x);return p", 0},
		{"meaningUnreviewed", "", "", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := ps6141SummarySynthetic + `func allocate(n int) Packed{return storage(n)}
func expose(p *Block){}
func fused(x []float32,w Packed) float32{return dot(pack(x),w)}
func opaque(p Packed,x []float32)
func scale(v float32) float32{return (v*v+3)/8}
func opaqueIndex() int
func reset(i int) int{i=0;return i}
func identity(i int) int{return i}
func setIndex(i *int){*i=0}
`
			if tc.old != "" {
				before := source
				source = strings.ReplaceAll(source, tc.old, tc.new)
				if source == before {
					t.Fatal("mutation did not change source")
				}
			}
			pass, owner := ps6141TypedFixture(t, source)
			c := ps6141TestContract()
			c.ConsumerForm = "sourceSummary"
			c.WeightArgument = 1
			c.RowsArgument = -1
			if tc.name == "meaningUnreviewed" {
				c.QuantizationAndDotMeaningReviewed = false
			}
			if got := len(ps6141Candidates(pass, owner, &c)); got != tc.want {
				t.Fatalf("candidates=%d want=%d", got, tc.want)
			}
		})
	}
}

func TestPS6141SourceSummaryRawLinknamePartition(t *testing.T) {
	t.Parallel()
	for _, symbol := range []string{"pack", "storage", "fill", "dot", "leaf"} {
		t.Run(symbol, func(t *testing.T) {
			t.Parallel()
			source := strings.Replace(ps6141SummarySynthetic, "package fixture", "package fixture\nimport _ \"unsafe\"", 1)
			source = strings.Replace(source, "func "+symbol+"(", fmt.Sprintf("//go:linkname %s external.%s\nfunc %s(", symbol, symbol, symbol), 1)
			pass, owner := ps6141TypedFixture(t, source)
			found := false
			for _, file := range pass.Files {
				for _, group := range file.Comments {
					for _, comment := range group.List {
						if strings.HasPrefix(comment.Text, "//go:linkname ") {
							found = true
						}
					}
				}
			}
			if !found {
				t.Fatal("raw linkage directive prerequisite missing")
			}
			c := ps6141TestContract()
			c.ConsumerForm = "sourceSummary"
			c.WeightArgument = 1
			c.RowsArgument = -1
			if len(ps6141Candidates(pass, owner, &c)) != 0 {
				t.Fatal("linkage directive overrode source-body proof")
			}
		})
	}
}

func TestPS6141SourceSummaryWholeArrayExpressionEffects(t *testing.T) {
	t.Parallel()
	source := strings.ReplaceAll(ps6141SummarySynthetic, "Q int8", "Q [2]int8")
	source = strings.ReplaceAll(source, ".Q", ".Q[0]") + "\nfunc opaqueIndex() int\n"
	for _, tc := range []struct {
		name, old, new string
		want           int
	}{
		{"independentArrayPrerequisite", "", "", 1},
		{"opaqueRangeIndex", "for i:=range p {", "for i:=range p[opaqueIndex()].Q {", 0},
		{"opaqueLengthIndex", "if len(p)!=len(w)", "if len(p[opaqueIndex()].Q)!=2", 0},
		{"opaqueCapacityIndex", "if len(p)!=len(w)", "if cap(p[opaqueIndex()].Q)!=2", 0},
		{"opaqueNestedReadIndex", "float32(p[i].Q[0])", "float32(p[opaqueIndex()].Q[0])", 0},
		{"wrongIterationOrigin", "for i:=range p {", "for i:=range p[0].Q {", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mutated := source
			if tc.old != "" {
				mutated = strings.ReplaceAll(source, tc.old, tc.new)
				if mutated == source {
					t.Fatal("mutation did not change exact fixture")
				}
			}
			pass, owner := ps6141TypedFixture(t, mutated)
			c := ps6141TestContract()
			c.ConsumerForm = "sourceSummary"
			c.WeightArgument = 1
			c.RowsArgument = -1
			if got := len(ps6141Candidates(pass, owner, &c)); got != tc.want {
				t.Fatalf("candidates=%d want=%d", got, tc.want)
			}
		})
	}
}

func TestPS6141SourceSummaryArrayLayoutAndPermutedRoles(t *testing.T) {
	t.Parallel()
	source := strings.ReplaceAll(ps6141SummarySynthetic, "Q int8", "Q [2]int8")
	source = strings.ReplaceAll(source, ".Q", ".Q[0]")
	source = strings.ReplaceAll(source, "func dot(p,w Packed)", "func dot(w,p Packed)")
	source = strings.ReplaceAll(source, "return dot(p,w)", "return dot(w,p)")
	pass, owner := ps6141TypedFixture(t, source)
	c := ps6141TestContract()
	c.ConsumerForm = "sourceSummary"
	c.PackedArgument = 1
	c.WeightArgument = 0
	c.RowsArgument = -1
	if len(ps6141Candidates(pass, owner, &c)) != 1 {
		t.Fatal("array-valued packed layout with permuted roles rejected")
	}
}

func TestPS6141SourceSummaryBudgetFailsClosed(t *testing.T) {
	t.Parallel()
	pass, _ := ps6141TypedFixture(t, ps6141SummarySynthetic)
	declarations := ps6099LocalFunctionDeclarations(pass)
	index := &ps6141SummaryIndex{pass: pass, declarations: declarations, memo: map[*types.Func]ps6141SourceSummary{}, active: map[*types.Func]bool{}, remaining: 1}
	for fn := range declarations {
		if fn.Name() == "pack" && index.function(fn).valid {
			t.Fatal("exhausted summary budget admitted producer")
		}
	}
}

func TestPS6141SourceSummaryPointerLayoutUnknown(t *testing.T) {
	t.Parallel()
	source := strings.Replace(ps6141SummarySynthetic, "Sum int16", "Sum int16; Alias []int8", 1)
	pass, owner := ps6141TypedFixture(t, source)
	c := ps6141TestContract()
	c.ConsumerForm = "sourceSummary"
	c.WeightArgument = 1
	c.RowsArgument = -1
	if len(ps6141Candidates(pass, owner, &c)) != 0 {
		t.Fatal("fresh outer slice certified referenced block storage")
	}
}
