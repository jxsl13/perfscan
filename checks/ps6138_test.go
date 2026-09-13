package checks

import (
	"context"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6138ConventionalFixture(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), PS6138.Analyzer, "ps6138")
}

func ps6138Synthetic(t *testing.T) string {
	t.Helper()
	b, e := os.ReadFile("testdata/src/ps6138/fields_arm64.s")
	if e != nil {
		t.Fatal(e)
	}
	if strings.ContainsRune(string(b), '\r') || len(b) == 0 || b[len(b)-1] != '\n' {
		t.Fatal("synthetic assembly fixture must be nonempty LF text with a final newline")
	}
	return string(b)
}

func ps6138Replace(t *testing.T, source, old, replacement string, n int) string {
	t.Helper()
	changed := strings.Replace(source, old, replacement, n)
	if changed == source {
		t.Fatalf("fixture mutation did not change source: %q", old)
	}
	return changed
}

func TestPS6138TypedNativeBinding(t *testing.T) {
	t.Parallel()
	for name, source := range map[string]string{"body": "package binding\nfunc fields(ptr *byte) uint64{return 0}", "missing": "package binding\nfunc unrelated(){}", "variadic": "package binding\nfunc fields(ptr ...*byte) uint64", "method": "package binding\ntype T struct{}\nfunc(T) fields(ptr *byte) uint64", "linkname": "package binding\nimport _ \"unsafe\"\n//go:linkname fields other.fields\nfunc fields(ptr *byte) uint64"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir, cleanup, e := analysistest.WriteFiles(map[string]string{"binding/binding.go": source, "binding/fields_arm64.s": ps6138Synthetic(t)})
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(cleanup)
			a := *PS6138.Analyzer
			a.Run = func(pass *analysis.Pass) (any, error) {
				pass.Report = func(d analysis.Diagnostic) { t.Errorf("unsupported typed binding reported: %s", d.Message) }
				return runPS6138(pass)
			}
			analysistest.Run(t, dir, &a, "binding")
		})
	}
}

func TestPS6138LocalHeaderCannotStandInForSDK(t *testing.T) {
	t.Parallel()
	dir, cleanup, e := analysistest.WriteFiles(map[string]string{"binding/binding.go": "package binding\nfunc fields(ptr *byte) uint64", "binding/fields_arm64.s": ps6138Synthetic(t), "binding/textflag.h": "#define NOSPLIT 4\n#define AND ORR\n"})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(cleanup)
	a := *PS6138.Analyzer
	a.Run = func(pass *analysis.Pass) (any, error) {
		pass.Report = func(d analysis.Diagnostic) { t.Errorf("unmodeled local header reported: %s", d.Message) }
		return runPS6138(pass)
	}
	analysistest.Run(t, dir, &a, "binding")
}

func TestPS6138AdversarialAssembly(t *testing.T) {
	t.Parallel()
	base := ps6138Synthetic(t)
	cases := map[string]string{
		"signedload":           strings.Replace(base, "MOVBU", "MOVB", 1),
		"too_few":              strings.Replace(base, " AND $3, R8, R11\n LSR $2, R8, R8\n ADD R11, R9, R9\n", "", 2),
		"mask_gap":             strings.Replace(base, "AND $3", "AND $5", 1),
		"dynamic_mask":         strings.Replace(base, "AND $3", "AND R10", 1),
		"dynamic_shift":        strings.Replace(base, "LSR $2", "LSR R10", 1),
		"wrong_shift":          strings.Replace(base, "LSR $2", "LSR $1", 1),
		"arithmetic_shift":     strings.Replace(base, "LSR $2", "ASR $2", 1),
		"unequal_width":        strings.Replace(base, "AND $3", "AND $7", 1),
		"over_extent":          strings.ReplaceAll(strings.ReplaceAll(base, "AND $3", "AND $7"), "LSR $2", "LSR $3"),
		"source_destination":   strings.Replace(base, "AND $3, R8, R11", "AND $3, R8, R8", 1),
		"counter_destination":  strings.Replace(base, "AND $3, R8, R11", "AND $3, R8, R7", 1),
		"discarded":            strings.Replace(base, "ADD R11, R9, R9", "MOVD $0, R11", 1),
		"self_transform_only":  strings.ReplaceAll(base, "ADD R11, R9, R9", "LSL $1, R11, R11"),
		"source_observation":   strings.Replace(base, "ADD R11, R9, R9", "ADD R8, R9, R9", 1),
		"source_copy":          strings.Replace(base, "ADD R11, R9, R9", "MOVD R8, R10\n ADD R11, R9, R9", 1),
		"source_rebind":        strings.Replace(base, "ADD R11, R9, R9", "MOVD $0, R8\n ADD R11, R9, R9", 1),
		"counter_observed":     strings.Replace(base, "ADD R11, R9, R9", "ADD R7, R9, R9", 1),
		"address_source_alias": strings.Replace(base, "MOVBU (R6), R8", "MOVBU (R8), R8", 1),
		"extra_label":          strings.Replace(base, "ADD R11, R9, R9", "other:\n ADD R11, R9, R9", 1),
		"call":                 strings.Replace(base, "ADD R11, R9, R9", "CALL ·other(SB)\n ADD R11, R9, R9", 1),
		"early_return":         strings.Replace(base, "ADD R11, R9, R9", "RET\n ADD R11, R9, R9", 1),
		"alternate_exit":       strings.Replace(base, "ADD R11, R9, R9", "CBZ R9, exit\n ADD R11, R9, R9", 1),
		"alternate_entry":      strings.Replace(base, " MOVD $8, R7", " B loop\n MOVD $8, R7", 1),
		"unknown_instruction":  strings.Replace(base, "ADD R11, R9, R9", "MYSTERY R11, R9, R9", 1),
		"raw_word":             strings.Replace(base, "ADD R11, R9, R9", "WORD $0x8b080129\n ADD R11, R9, R9", 1),
		"raw_word_comment":     strings.Replace(base, "ADD R11, R9, R9", "WORD $0x8b080129 /* harmless SIMD */\n ADD R11, R9, R9", 1),
		"postindexed_load":     strings.Replace(base, "MOVBU (R6)", "MOVBU.P 1(R6)", 1),
		"tail_source_live":     strings.Replace(base, "MOVD R9, ret+8(FP)", "MOVD R8, ret+8(FP)", 1),
		"tail_unknown":         strings.Replace(base, "MOVD R9, ret+8(FP)", "MYSTERY R9, R10", 1),
		"trip_one":             strings.Replace(base, "MOVD $8, R7", "MOVD $1, R7", 1),
		"dynamic_trip":         strings.Replace(base, "MOVD $8, R7", "MOVD R10, R7", 1),
		"wrong_backedge":       strings.Replace(base, "BNE loop", "BNE other", 1),
		"wrong_counter":        strings.Replace(base, "SUBS $1, R7, R7", "SUBS $1, R10, R10", 1),
		"unknown_include":      "#include \"aliases.h\"\n" + base,
		"register_alias":       "#define R8 R10\n" + base,
		"unknown_condition":    "#if SOME_TARGET\n" + base + "\n#endif",
		"macro_condition":      "#define FIELD() ADD R8, R9, R9\n" + strings.Replace(base, " ADD R11, R9, R9", "#ifdef FIELD\n FIELD()\n#endif\n ADD R11, R9, R9", 1),
		"inactive":             "#if 0\n" + base + "\n#endif",
		"duplicate_text":       base + base,
		"unknown_text_flags":   strings.Replace(base, "NOSPLIT", "DUPOK|NOSPLIT", 1),
		"implicit_frame":       strings.Replace(base, "$0-16", "$32-16", 1),
		"unterminated_comment": base + "/*",
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if source == base {
				t.Fatal("adversarial fixture mutation did not change source")
			}
			if got := ps6138Analyze(source); len(got) != 0 {
				t.Fatalf("unexpected finding: %+v", got)
			}
		})
	}
	if got := ps6138Analyze(base); len(got) != 1 || got[0].width != 2 || got[0].count != 4 {
		t.Fatalf("synthetic positive: %+v", got)
	}
}

func TestPS6138MacroBoundaries(t *testing.T) {
	t.Parallel()
	base := ps6138Synthetic(t)
	body := "AND $3, R8, R11; LSR $2, R8, R8; ADD R11, R9, R9"
	macro := "#define FIELD() " + body + "\n" + ps6138Replace(t, base, " AND $3, R8, R11\n LSR $2, R8, R8\n ADD R11, R9, R9", " FIELD()", -1)
	parameter := ps6138Replace(t, ps6138Replace(t, macro, "#define FIELD() "+body, "#define FIELD(W) AND $((1<<W)-1), R8, R11; LSR $W, R8, R8; ADD R11, R9, R9", 1), "FIELD()", "FIELD(2)", -1)
	// Expressions are intentionally unsupported; direct literal parameters work.
	literal := ps6138Replace(t, parameter, "$((1<<W)-1)", "$3", 1)
	for name, source := range map[string]string{"zero": macro, "one": literal, "nested": "#define OUTER() FIELD()\n" + ps6138Replace(t, macro, " FIELD()", " OUTER()", -1), "continued": ps6138Replace(t, macro, "; LSR", "; \\\n LSR", 1)} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := ps6138Analyze(source); len(got) != 1 {
				t.Fatalf("macro positive: %+v", got)
			}
		})
	}
	for name, source := range map[string]string{"expression": parameter, "recursive": ps6138Replace(t, macro, body, "FIELD()", 1), "nonliteral": ps6138Replace(t, literal, "FIELD(2)", "FIELD(R10)", -1), "operand_alias": "#define R8() R10\n" + base, "redefined": ps6138Replace(t, macro, "loop:", "#undef FIELD\n#define FIELD() MYSTERY\nloop:", 1)} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := ps6138Analyze(source); len(got) != 0 {
				t.Fatalf("macro negative: %+v", got)
			}
		})
	}
}

func TestPS6138PinnedOptimizedOwner(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]string{"optimized.s": "ff425caa920c4281033d794c061b51c894b3da2c0cf75fc43d35ac39158f817c", "driver.go": "45962d535d57229fcbc44de958d417cf46688b46ec02b818562817bc9195eded"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			data, e := os.ReadFile("testdata/ps6138_owner_" + name + ".txt")
			if e != nil {
				t.Fatal(e)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != want {
				t.Fatalf("digest %s", got)
			}
			if strings.ContainsRune(string(data), '\r') {
				t.Fatal("LF required")
			}
			if name == "optimized.s" {
				if got := ps6138Analyze(string(data)); len(got) != 0 {
					t.Fatalf("optimized owner reported: %+v", got)
				}
				code, ok := ps6138Expand(string(data))
				if !ok {
					t.Fatal("owner macros not parsed")
				}
				ubfx := 0
				for _, i := range code {
					if i.op == "UBFX" {
						ubfx++
					}
				}
				if ubfx != 4 {
					t.Fatalf("owner UBFX count=%d", ubfx)
				}
			} else {
				f, e := parser.ParseFile(token.NewFileSet(), name, data, parser.ParseComments)
				if e != nil {
					t.Fatal(e)
				}
				found := false
				for _, d := range f.Decls {
					if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == "dotIQ2SBlockNeon" && fn.Body == nil {
						found = true
					}
				}
				if !found {
					t.Fatal("actual native declaration absent")
				}
			}
		})
	}
}

func TestPS6138AssembledPositive(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		load  string
		width int
	}{{"MOVBU", 2}, {"MOVHU", 4}, {"MOVWU", 8}} {
		for _, independent := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s_%v", tc.load, independent), func(t *testing.T) {
				t.Parallel()
				base := ps6138Synthetic(t)
				base = strings.Replace(base, "MOVBU", tc.load, 1)
				mask := uint64(1)<<tc.width - 1
				base = strings.ReplaceAll(base, "AND $3", fmt.Sprintf("AND $%d", mask))
				base = strings.ReplaceAll(base, "LSR $2", fmt.Sprintf("LSR $%d", tc.width))
				if independent {
					for field := 0; field < 4; field++ {
						base = ps6138Replace(t, base, fmt.Sprintf("AND $%d, R8, R11\n LSR $%d, R8, R8", mask, tc.width), fmt.Sprintf("UBFX $%d, R8, $%d, R11", field*tc.width, tc.width), 1)
					}
				}
				want := 1
				if independent {
					want = 0
				}
				if got := ps6138Analyze(base); len(got) != want {
					t.Fatalf("assembled control findings: %+v", got)
				}
				ps6138Assemble(t, base)
			})
		}
	}
}

func ps6138Assemble(t *testing.T, assembly string) {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "fields.s")
	if e := os.WriteFile(source, []byte(assembly), 0600); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	goPath, e := exec.LookPath("go")
	if e != nil {
		t.Fatal(e)
	}
	envCommand := exec.CommandContext(ctx, goPath, "env", "GOROOT")
	envCommand.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	sdk, e := envCommand.Output()
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.CommandContext(ctx, goPath, "tool", "asm", "-I", filepath.Join(strings.TrimSpace(string(sdk)), "pkg", "include"), "-o", filepath.Join(dir, "fields.o"), source)
	cmd.Env = append(os.Environ(), "GOARCH=arm64", "GOOS=linux", "GOTOOLCHAIN=local")
	if output, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("assemble supported positive: %v\n%s", e, output)
	}
	if info, e := os.Stat(filepath.Join(dir, "fields.o")); e != nil || info.Size() == 0 {
		t.Fatalf("assembly object missing: %v", e)
	}
}

func TestPS6138SupportedWidthValueEquivalence(t *testing.T) {
	t.Parallel()
	for _, bits := range []int{8, 16, 32} {
		for width := 1; width*3 <= bits; width++ {
			t.Run(fmt.Sprintf("%d_%d", bits, width), func(t *testing.T) {
				t.Parallel()
				count := bits / width
				mask := uint64(1)<<width - 1
				var assembly strings.Builder
				load := map[int]string{8: "MOVBU", 16: "MOVHU", 32: "MOVWU"}[bits]
				assembly.WriteString("TEXT ·fields(SB), NOSPLIT, $0-16\n MOVD $8, R7\nloop:\n " + load + " (R6), R8\n")
				for field := 0; field < count; field++ {
					fmt.Fprintf(&assembly, " AND $%d, R8, R11\n LSR $%d, R8, R8\n ADD R11, R9, R9\n", mask, width)
				}
				assembly.WriteString(" SUBS $1, R7, R7\n BNE loop\n RET\n")
				if got := ps6138Analyze(assembly.String()); len(got) != 1 || got[0].width != width || got[0].count != count {
					t.Fatalf("supported width recognizer: %+v", got)
				}
				limit := uint64(1) << bits
				samples := limit
				if samples > 65536 {
					samples = 65536
				}
				for sample := uint64(0); sample < samples; sample++ {
					value := sample
					if bits == 32 {
						value = (sample * 2654435761) & (limit - 1)
					}
					shifted := value
					for field := 0; field < count; field++ {
						serial := shifted & mask
						independent := (value >> uint(field*width)) & mask
						if serial != independent {
							t.Fatalf("field%d value%x", field, value)
						}
						shifted >>= width
					}
				}
			})
		}
	}
}
