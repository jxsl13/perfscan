package checks

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2126 reports fmt.Errorf calls whose single argument is a string literal
// with no % in it — a constant error message pushed through fmt's printf
// machinery when errors.New builds the identical error directly.
var PS2126 = register(&lint.Check{
	ID:       "PS2126",
	Category: "alloc",
	Slug:     "errorf-constant",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: `fmt.Errorf of a verbless constant message is errors.New behind the printf machinery`,
		Text: `fmt.Errorf spins up a printer, walks its format string through
doPrintf, and assembles the result in a pooled pp buffer — the whole
reflection-priced formatting path — even when the format is a constant
string with nothing to format. For that shape fmt does exactly what
errors.New does and no more: with no %w wrap verb it returns
errors.New(s), and with no verbs at all s is the format string verbatim.
So fmt.Errorf("constant message") is byte-for-byte errors.New("constant
message") — the same *errorString dynamic type wrapping the same message
— reached the slow way.

The match is deliberately narrow — it is the whole safety story. The
callee must resolve through type information to the package-level
fmt.Errorf (an aliased fmt import is honored; a shadowed fmt or a method
named Errorf is not), the call must pass EXACTLY one argument and not
spread it (no ...), and that argument must be a string LITERAL whose
unquoted value contains NO '%' byte anywhere. A single '%' disqualifies
it: fmt would interpret it (%%->%, a verb consumes an argument that is
not there and prints %!verb(MISSING)), while errors.New keeps the bytes
verbatim — so any '%' breaks bit-identity. Extra arguments, a
non-literal format, or a spread all mean the format is doing real work
and stay out.

The fix replaces the whole call with errors.New keeping the message
literal byte-verbatim, reusing the file's existing "errors" import
(honoring an alias) or adding one in sorted position when absent. The
new import edit is carried once per file. Like PS2107, the fix still
applies when it removes the file's last fmt reference — the fix pipeline
prunes the now-unused fmt import — except in a cgo file (import "C"),
whose import block must never be edited, so there the report stays
advisory. The fix is also withheld when "errors" is shadowed at the call
site or when a comment overlaps the call (the rewrite would drop it).`,
		Before: `return fmt.Errorf("database connection failed")`,
		After:  `return errors.New("database connection failed")`,
		MeasuredWin: `BenchmarkPS2126 (a verbless "database connection
failed" message, Apple M2 Pro, -count=5):
fmt.Errorf("database connection failed") ~18.5 ns/op, 16 B/op, 1 alloc/op vs
errors.New("database connection failed") ~15.2 ns/op, 16 B/op, 1 alloc/op
(~1.2x faster; both allocate only the errorString because fmt pools its
printer, so the saving is the format walk plus the printer get/put — it
widens as the message grows and fmt scans more bytes). The primary win is
clarity: errors.New says exactly what the constant case does.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2126",
		Doc:  "fmt.Errorf of a verbless constant message; errors.New builds the identical error directly",
		Run:  runPS2126,
	},
})

// ps2126Msg is the diagnostic text (shared by the fixed and advisory paths).
const ps2126Msg = "fmt.Errorf with a verbless constant message runs fmt's formatter machinery to hand the string verbatim to errors.New; errors.New builds the identical error directly"

func runPS2126(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first: applying all fixes may rewrite the file's last fmt
		// reference and orphan the import. The fix pipeline prunes such an
		// orphan afterwards — except in a cgo file, whose import block is
		// never edited, so there the fixes are withheld and the reports stay
		// advisory (same rule as PS2107).
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		// All fixes of a run apply together, so only the first fix that needs
		// the errors import carries the import edit.
		importAdded := false
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
				return true
			}
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "fmt", map[string]bool{"Errorf": true}); !ok {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			msg, err := strconv.Unquote(lit.Value)
			if err != nil || strings.IndexByte(msg, '%') >= 0 {
				return true
			}
			var fix *analysis.SuggestedFix
			if !ps2107CommentsOverlap(f, call) {
				// errors.New needs the "errors" package usable at the call site:
				// reuse an existing import (honoring its alias) or add one, unless
				// the name is shadowed or the file is cgo (import block untouched).
				useName, needImport, usable := ps2107PkgUsable(pass, f, call.Pos(), "errors", "errors")
				if usable && !(needImport && ps2107ImportsC(f)) {
					edits := []analysis.TextEdit{
						// The message literal is spliced back byte-verbatim.
						{Pos: call.Pos(), End: call.End(), NewText: []byte(useName + ".New(" + lit.Value + ")")},
					}
					if needImport && !importAdded {
						edits = append(edits, ps2107ImportEdit(f, "errors"))
						importAdded = true
					}
					fix = &analysis.SuggestedFix{
						Message:   "replace fmt.Errorf(" + lit.Value + ") with errors.New",
						TextEdits: edits,
					}
					fixable++
				}
			}
			sites = append(sites, site{call, fix})
			return true
		})
		emitFixes := fixable > 0 && (pkgRefCount(pass, f, "fmt") > fixable || !ps2107ImportsC(f))
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: ps2126Msg,
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}
