package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6121ReachabilityAndLoopProof(t *testing.T) {
	t.Parallel()
	const loop = `for i := lo; i < hi; i++ { out[i] = fn(out[i], 1) }`
	const fanout = `parallelFor(len(out), func(lo, hi int) { LOOP })`
	cases := []struct {
		name, body, caller string
		want               int
	}{
		{"positive", fanout, "", 1},
		{"uninvokedOuterClosure", `unused := func() { FANOUT }; _ = unused`, "", 0},
		{"fanoutInDeadBranch", `if false { FANOUT }`, "", 0},
		{"fanoutAfterReturn", `return; FANOUT`, "", 0},
		{"callbackLoopAfterReturn", `parallelFor(len(out), func(lo, hi int) { return; LOOP })`, "", 0},
		{"deadCallInsideLoop", `parallelFor(len(out), func(lo, hi int) { for i := lo; i < hi; i++ { if false { out[i] = fn(out[i], 1) } } })`, "", 0},
		{"callAfterContinue", `parallelFor(len(out), func(lo, hi int) { for i := lo; i < hi; i++ { _ = i; continue; out[i] = fn(out[i], 1) } })`, "", 0},
		{"callAfterBreak", `parallelFor(len(out), func(lo, hi int) { for i := lo; i < hi; i++ { _ = i; break; out[i] = fn(out[i], 1) } })`, "", 0},
		{"oneIterationByBreak", `parallelFor(len(out), func(lo, hi int) { for i := lo; i < hi; i++ { out[i] = fn(out[i], 1); break } })`, "", 0},
		{"oneIterationByReturn", `parallelFor(len(out), func(lo, hi int) { for i := lo; i < hi; i++ { out[i] = fn(out[i], 1); return } })`, "", 0},
		{"oneIterationBandRelativeBound", `parallelFor(len(out), func(lo, hi int) { for i := lo; i < lo+1; i++ { out[i] = fn(out[i], 1) }; _ = hi })`, "", 0},
		{"oneIterationByPost", `parallelFor(len(out), func(lo, hi int) { for i := lo; i < hi; i = hi { out[i] = fn(out[i], 1) } })`, "", 0},
		{"fixedBoundAfterStart", `parallelFor(len(out), func(lo, hi int) { for i := lo; i < 2; i++ { out[i] = fn(out[i], 1) }; _ = hi })`, "", 0},
		{"reassignedBand", `parallelFor(len(out), func(lo, hi int) { lo, hi = 0, 1; LOOP })`, "", 0},
		{"deferredFanout", `defer FANOUT`, "", 0},
		{"goroutineFanout", `go FANOUT`, "", 0},
		{"deferredHelper", fanout, `func caller(out []float64) { defer helper(out, scalar) }`, 0},
		{"goroutineHelper", fanout, `func caller(out []float64) { go helper(out, scalar) }`, 0},
		{"namedSecondCallback", `parallelFor(len(out), func(lo, hi int) { LOOP }, noop)`, "", 0},
		{"nilSecondCallback", `parallelFor(len(out), func(lo, hi int) { LOOP }, nil)`, "", 0},
		{"indexAssignedAfterCall", `parallelFor(len(out), func(lo, hi int) { for i := lo; i < hi; i++ { out[i] = fn(out[i], 1); i = hi } })`, "", 0},
		{"endAssignedAfterCall", `parallelFor(len(out), func(lo, hi int) { for i := lo; i < hi; i++ { out[i] = fn(out[i], 1); hi = i } })`, "", 0},
		{"gotoAfterCall", `parallelFor(len(out), func(lo, hi int) { for i := lo; i < hi; i++ { out[i] = fn(out[i], 1); goto done }; done: _ = hi })`, "", 0},
		{"panicAfterCall", `parallelFor(len(out), func(lo, hi int) { for i := lo; i < hi; i++ { out[i] = fn(out[i], 1); panic("stop") } })`, "", 0},
		{"addressedEndBeforeLoop", `parallelFor(len(out), func(lo, hi int) { p := &hi; *p = lo+1; LOOP })`, "", 0},
		{"addressedIndexInLoop", `parallelFor(len(out), func(lo, hi int) { for i := lo; i < hi; i++ { out[i] = fn(out[i], 1); p := &i; *p = hi } })`, "", 0},
		{"rangeWritesEnd", `parallelFor(len(out), func(lo, hi int) { for _, hi = range []int{lo+1} {}; LOOP })`, "", 0},
		{"iifeEndMutation", `parallelFor(len(out), func(lo, hi int) { func(){ hi=lo+1 }(); LOOP })`, "", 0},
		{"iifeIndexMutation", `parallelFor(len(out), func(lo, hi int) { for i:=lo; i<hi; i++ { out[i]=fn(out[i],1); func(){ i=hi }() } })`, "", 0},
		{"iifeEscapesBoundAddress", `parallelFor(len(out), func(lo, hi int) { p:=func()*int{return &hi}(); *p=lo+1; LOOP })`, "", 0},
		{"blockingSelectAfterCall", `parallelFor(len(out), func(lo, hi int) { for i:=lo; i<hi; i++ { out[i]=fn(out[i],1); select{} } })`, "", 0},
		{"infiniteLoopAfterCall", `parallelFor(len(out), func(lo, hi int) { for i:=lo; i<hi; i++ { out[i]=fn(out[i],1); for{} } })`, "", 0},
		{"labelledContinueExitsInnerLoop", `parallelFor(len(out), func(lo, hi int) { outer: for j:=0; j<1; j++ { for i:=lo; i<hi; i++ { out[i]=fn(out[i],1); continue outer } } })`, "", 0},
		{"constantTrueIfBlocksCycle", `parallelFor(len(out), func(lo, hi int) { for i:=lo; i<hi; i++ { out[i]=fn(out[i],1); if true { select{} } } })`, "", 0},
		{"constantTrueLoopBlocksCycle", `parallelFor(len(out), func(lo, hi int) { for i:=lo; i<hi; i++ { out[i]=fn(out[i],1); for true {} } })`, "", 0},
		{"constantSwitchBlocksCycle", `parallelFor(len(out), func(lo, hi int) { for i:=lo; i<hi; i++ { out[i]=fn(out[i],1); switch 1 { case 1: select{} } } })`, "", 0},
		{"constantFalseIfUnsupported", `parallelFor(len(out), func(lo, hi int) { for i:=lo; i<hi; i++ { out[i]=fn(out[i],1); if false { select{} } } })`, "", 0},
		{"constantNonmatchingSwitchUnsupported", `parallelFor(len(out), func(lo, hi int) { for i:=lo; i<hi; i++ { out[i]=fn(out[i],1); switch 1 { case 2: select{} } } })`, "", 0},
		{"helperConstantReturnPrefix", `if true { return }; FANOUT`, "", 0},
		{"helperDynamicReturnPrefix", `if len(out)==0 { return }; FANOUT`, "", 1},
		{"callbackConstantReturnPrefix", `parallelFor(len(out), func(lo, hi int) { if true { return }; LOOP })`, "", 0},
		{"callbackFalseReturnPrefix", `parallelFor(len(out), func(lo, hi int) { if false { return }; LOOP })`, "", 1},
		{"callbackDynamicReturnPrefix", `parallelFor(len(out), func(lo, hi int) { if len(out)==0 { return }; LOOP })`, "", 1},
		{"callbackConstantBlockingLoopPrefix", `parallelFor(len(out), func(lo, hi int) { for true {}; LOOP })`, "", 0},
		{"callbackFalseLoopPrefix", `parallelFor(len(out), func(lo, hi int) { for false {}; LOOP })`, "", 1},
		{"callbackSwitchPrefixUnsupported", `parallelFor(len(out), func(lo, hi int) { switch 1 { case 1: return }; LOOP })`, "", 0},
		{"callbackNonmatchingSwitchPrefixLive", `parallelFor(len(out), func(lo, hi int) { switch 1 { case 2: return }; LOOP })`, "", 1},
		{"callbackConstantElseReturnPrefix", `parallelFor(len(out), func(lo, hi int) { if false {} else { return }; LOOP })`, "", 0},
		{"callbackDeadElseReturnPrefix", `parallelFor(len(out), func(lo, hi int) { if true {} else { return }; LOOP })`, "", 1},
		{"helperCandidateInLiveElse", `if false { return } else { FANOUT }`, "", 1},
		{"callbackCandidateInLiveElse", `parallelFor(len(out), func(lo, hi int) { if false { return } else { LOOP } })`, "", 1},
		{"helperShortCircuitReturn", `if true || len(out)>0 { return }; FANOUT`, "", 0},
		{"callbackShortCircuitReturn", `parallelFor(len(out), func(lo, hi int) { if true || len(out)>0 { return }; LOOP })`, "", 0},
		{"helperTrueLoopBreak", `for true { break }; FANOUT`, "", 1},
		{"callbackTrueLoopBreak", `parallelFor(len(out), func(lo, hi int) { for true { break }; LOOP })`, "", 1},
		{"helperConstantBreak", `for true { if true { break } }; FANOUT`, "", 1},
		{"callbackConstantBreak", `parallelFor(len(out), func(lo, hi int) { for true { if true { break } }; LOOP })`, "", 1},
		{"helperUnmatchedSwitch", `switch 1 { case 2: return }; FANOUT`, "", 1},
		{"callbackUnmatchedSwitch", `parallelFor(len(out), func(lo, hi int) { switch 1 { case 2: return }; LOOP })`, "", 1},
		{"helperSwitchBreak", `switch 1 { case 1: break; default: return }; FANOUT`, "", 1},
		{"callbackSwitchBreak", `parallelFor(len(out), func(lo, hi int) { switch 1 { case 1: break; default: return }; LOOP })`, "", 1},
		{"helperUnselectedFallthrough", `switch 1 { case 2: fallthrough; case 3: return }; FANOUT`, "", 1},
		{"callbackUnselectedFallthrough", `parallelFor(len(out), func(lo, hi int) { switch 1 { case 2: fallthrough; case 3: return }; LOOP })`, "", 1},
		{"helperSelectDefault", `select { default: }; FANOUT`, "", 1},
		{"callbackSelectDefault", `parallelFor(len(out), func(lo, hi int) { select { default: }; LOOP })`, "", 1},
		{"helperNilSelectCaseUnreachable", `select { case <-(chan int)(nil): FANOUT; default: }`, "", 0},
		{"callbackNilSelectCaseUnreachable", `parallelFor(len(out), func(lo, hi int) { select { case (chan int)(nil) <- 1: LOOP; default: } })`, "", 0},
		{"callbackSelectDefaultExpressionBlocks", `parallelFor(len(out), func(lo, hi int) { select { default: _ = <-(chan int)(nil); LOOP } })`, "", 0},
		{"callbackSelectDefaultCycleBlocks", `parallelFor(len(out), func(lo, hi int) { select { default: for i:=lo; i<hi; i++ { out[i]=fn(out[i],1); <-(chan int)(nil) } } })`, "", 0},
		{"callbackSelectDefaultShortCircuitLive", `parallelFor(len(out), func(lo, hi int) { select { default: _ = false && <-(chan bool)(nil); LOOP } })`, "", 1},
		{"helperLabelBreak", `outer: for true { for true { break outer } }; FANOUT`, "", 1},
		{"callbackLabelBreak", `parallelFor(len(out), func(lo, hi int) { outer: for true { for true { break outer } }; LOOP })`, "", 1},
		{"helperGotoLive", `goto live; return; live: FANOUT`, "", 1},
		{"callbackGotoLive", `parallelFor(len(out), func(lo, hi int) { goto live; return; live: LOOP })`, "", 1},
		{"helperDynamicSwitchAllTerminate", `switch len(out) { case 1: if true { select{} }; default: if true { return } }; FANOUT`, "", 0},
		{"callbackDynamicSwitchAllTerminate", `parallelFor(len(out), func(lo, hi int) { switch len(out) { case 1: if true { select{} }; default: if true { return } }; LOOP })`, "", 0},
		{"helperBlockingBeforeFallthrough", `switch 1 { case 1: select{}; fallthrough; case 2: }; FANOUT`, "", 0},
		{"callbackBlockingBeforeFallthrough", `parallelFor(len(out), func(lo, hi int) { switch 1 { case 1: select{}; fallthrough; case 2: }; LOOP })`, "", 0},
		{"helperReturnBeforeFallthrough", `switch 1 { case 1: if true { return }; fallthrough; case 2: }; FANOUT`, "", 0},
		{"callbackReturnBeforeFallthrough", `parallelFor(len(out), func(lo, hi int) { switch 1 { case 1: if true { return }; fallthrough; case 2: }; LOOP })`, "", 0},
		{"helperLiveFallthrough", `switch 1 { case 1: _=out; fallthrough; case 2: }; FANOUT`, "", 1},
		{"callbackLiveFallthrough", `parallelFor(len(out), func(lo, hi int) { switch 1 { case 1: _=out; fallthrough; case 2: }; LOOP })`, "", 1},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			body := strings.ReplaceAll(strings.ReplaceAll(testCase.body, "FANOUT", fanout), "LOOP", loop)
			caller := testCase.caller
			if caller == "" {
				caller = `func caller(out []float64) { helper(out, scalar) }`
			}
			source := `package repro
func parallelFor(n int, bodies ...func(int,int)) { for _, body := range bodies { body(0,n) } }
func noop(lo,hi int) {}
func scalar(x,y float64) float64 { return x*y }
func helper(out []float64, fn func(float64,float64)float64) { ` + body + ` }
` + caller
			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, "repro.go", source, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}, Instances: map[*ast.Ident]types.Instance{}}
			pkg, err := new(types.Config).Check("repro", fileSet, []*ast.File{file}, info)
			if err != nil {
				t.Fatal(err)
			}
			var diagnostics []analysis.Diagnostic
			pass := &analysis.Pass{Analyzer: PS6121.Analyzer, Fset: fileSet, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info, Report: func(diagnostic analysis.Diagnostic) { diagnostics = append(diagnostics, diagnostic) }}
			if _, err := runPS6121WithFanout(pass, map[string]bool{"parallelFor": true}); err != nil {
				t.Fatal(err)
			}
			if len(diagnostics) != testCase.want {
				t.Errorf("got %d diagnostics, want %d: %v", len(diagnostics), testCase.want, diagnostics)
			}
		})
	}
}

func TestPS6121NamedFunctionTypesStaySilent(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"callbackAmbiguity": `package repro
type namedCallback func(int,int)
func parallelFor(n int, body func(int,int), after namedCallback) { body(0,n); after(0,n) }
func noop(int,int) {}
func scalar(x,y float64) float64 { return x*y }
func helper(out []float64, fn func(float64,float64)float64) {
	parallelFor(len(out), func(lo,hi int) { for i:=lo; i<hi; i++ { out[i]=fn(out[i],1) } }, noop)
}
func caller(out []float64) { helper(out, scalar) }`,
		"extraCapture": `package repro
type observer func(float64,float64) float64
func parallelFor(n int, body func(int,int)) { body(0,n) }
func scalar(x,y float64) float64 { return x*y }
func helper(out []float64, fn func(float64,float64)float64, observe observer) {
	parallelFor(len(out), func(lo,hi int) { for i:=lo; i<hi; i++ { out[i]=observe(fn(out[i],1),1) } })
}
func caller(out []float64) { helper(out, scalar, scalar) }`,
	}
	for name, source := range cases {
		name, source := name, source
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, "repro.go", source, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}, Instances: map[*ast.Ident]types.Instance{}}
			pkg, err := new(types.Config).Check("repro", fileSet, []*ast.File{file}, info)
			if err != nil {
				t.Fatal(err)
			}
			var diagnostics []analysis.Diagnostic
			pass := &analysis.Pass{Analyzer: PS6121.Analyzer, Fset: fileSet, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info, Report: func(diagnostic analysis.Diagnostic) { diagnostics = append(diagnostics, diagnostic) }}
			if _, err := runPS6121WithFanout(pass, map[string]bool{"parallelFor": true}); err != nil {
				t.Fatal(err)
			}
			if len(diagnostics) != 0 {
				t.Errorf("got unexpected diagnostics: %v", diagnostics)
			}
		})
	}
}

func TestPS6121(t *testing.T) {
	t.Parallel()
	results := analysistest.Run(t, analysistest.TestData(), ps6121TestAnalyzer(map[string]bool{"parallelFor": true}), "ps6121")
	count := 0
	for _, result := range results {
		for _, diagnostic := range result.Diagnostics {
			count++
			if len(diagnostic.SuggestedFixes) != 0 {
				t.Fatalf("PS6121 unexpectedly supplied an automatic fix")
			}
		}
	}
	if count != 2 {
		t.Fatalf("PS6121 emitted %d diagnostics, want two", count)
	}
}

func TestPS6121RejectsBlockingNilChannelTransfers(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		before, after string
		want          int
	}{
		"receiveBefore":           {before: `<-(chan int)(nil)`},
		"receiveAfter":            {after: `<-(chan int)(nil)`},
		"assignedReceiveBefore":   {before: `_ = <-(chan int)(nil)`},
		"assignedReceiveAfter":    {after: `_ = <-(chan int)(nil)`},
		"sendBefore":              {before: `(chan int)(nil) <- 1`},
		"sendAfter":               {after: `(chan int)(nil) <- 1`},
		"nestedReceiveBefore":     {before: `{ <-(chan int)(nil) }`},
		"nestedReceiveAfter":      {after: `{ <-(chan int)(nil) }`},
		"nestedSendBefore":        {before: `{ (chan int)(nil) <- 1 }`},
		"nestedSendAfter":         {after: `{ (chan int)(nil) <- 1 }`},
		"expressionReceiveBefore": {before: `out[i] = float64(<-(chan int)(nil))`},
		"expressionReceiveAfter":  {after: `out[i] += float64(<-(chan int)(nil))`},
		"shortCircuitAnd":         {before: `_ = false && <-(chan bool)(nil)`, want: 1},
		"shortCircuitOr":          {after: `_ = true || <-(chan bool)(nil)`, want: 1},
	}
	for name, parts := range cases {
		name, parts := name, parts
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			source := `package repro
func parallelFor(n int, body func(int,int)){ body(0,n) }
func scalar(x,y float64)float64{return x*y}
func helper(out []float64,fn func(float64,float64)float64){parallelFor(len(out),func(lo,hi int){for i:=lo;i<hi;i++ {` + parts.before + `;out[i]=fn(out[i],1);` + parts.after + `}})}
func caller(out []float64){helper(out,scalar)}`
			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, "blocking.go", source, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}, Instances: map[*ast.Ident]types.Instance{}}
			pkg, err := new(types.Config).Check("repro", fileSet, []*ast.File{file}, info)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			pass := &analysis.Pass{Analyzer: PS6121.Analyzer, Fset: fileSet, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info, Report: func(analysis.Diagnostic) { count++ }}
			if _, err := runPS6121WithFanout(pass, map[string]bool{"parallelFor": true}); err != nil {
				t.Fatal(err)
			}
			if count != parts.want {
				t.Fatalf("got %d diagnostics, want %d", count, parts.want)
			}
		})
	}
}

func TestPS6121SilentWithoutVocabulary(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), ps6121TestAnalyzer(nil), "ps6121silent")
}

func TestPS6121OwnerEvidenceAndMetadata(t *testing.T) {
	t.Parallel()
	evidence := strings.Join(strings.Fields(PS6121.Doc.MeasuredWin), " ")
	for _, fragment := range []string{"rejected", "1.0114x", "0.382867", "1.0049x", "0.710373", "1.0400x", "0.620047", "1.05x", "p<0.05", "no rewrite"} {
		if !strings.Contains(evidence, fragment) {
			t.Errorf("PS6121 owner evidence missing %q", fragment)
		}
	}
	doc := strings.Join(strings.Fields(PS6121.Doc.Text), " ")
	for _, fragment := range []string{"does NOT prove", "does not claim a speedup", "NO automatic fix", "finite result bits", "signed zeros", "nonfinite classes", "views", "input immutability", "one-ULP mutation", "three alternating count-seven campaigns", "no allocation increase"} {
		if !strings.Contains(doc, fragment) {
			t.Errorf("PS6121 documentation missing %q", fragment)
		}
	}
	if PS6121.AutoFix || !PS6121.NeedsConfig || PS6121.Level != 3 || PS6121.Category != "verify" || len(PS6121.Vocab) != 1 || PS6121.Vocab[0] != "fanOutHelpers" {
		t.Fatalf("PS6121 metadata drift: AutoFix=%v NeedsConfig=%v Level=%d Category=%q Vocab=%v", PS6121.AutoFix, PS6121.NeedsConfig, PS6121.Level, PS6121.Category, PS6121.Vocab)
	}
}

func ps6121TestAnalyzer(fanout map[string]bool) *analysis.Analyzer {
	analyzer := *PS6121.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) { return runPS6121WithFanout(pass, fanout) }
	return &analyzer
}
