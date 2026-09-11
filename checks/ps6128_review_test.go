package checks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/packages"
)

const ps6128ReviewBase = `package casepkg
import "strings"
var gemm func(int)
func brandString() string { return "Apple M1" }
func amxSupported() bool { brand := brandString(); return strings.Contains(brand,"Apple M1") || strings.Contains(brand,"Apple M2") || strings.Contains(brand,"Apple M3") }
func init(){ if amxSupported(){ gemm=compute } }
func parallelWork(work func()){ work() }
func compute(k int){ parallelWork(func(){ tile(k) }) }
func caller(k int){ gemm(k) }
func safe(int){}
func tile(k int)
`

const ps6128ReviewAsm = `#include "textflag.h"
#define AMX_LDY_R5 WORD $0x00201025
TEXT ·tile(SB), NOSPLIT, $0-8
 MOVD k+0(FP), R3
 MOVD $0x5000000000000000, R16
 ORR R16, R0, R5
 LSR $2, R3, R15
 CBZ R15, tail
 AMX_LDY_R5
tail:
 EOR $0x1000000000000000, R5, R5
 RET
`

var ps6128ReviewGoRoot = sync.OnceValues(func() (string, error) {
	output, err := exec.Command("go", "env", "GOROOT").Output()
	return strings.TrimSpace(string(output)), err
})

func ps6128ReviewRun(t *testing.T, source, assembly string, finding bool) {
	ps6128ReviewRunWithNativeArgument(t, source, assembly, finding, 0)
}

func ps6128ReviewRunWithNativeArgument(t *testing.T, source, assembly string, finding bool, nativeArgument int) {
	t.Helper()
	if finding {
		source = strings.Replace(source, "func amxSupported() bool {", "func amxSupported() bool { // want \"native dispatch accepts Apple M1\"\n", 1)
	}
	dir, cleanup, err := analysistest.WriteFiles(map[string]string{"casepkg/case.go": source, "casepkg/kernel.s": assembly})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	// Parse the native fixture with the real ARM64 assembler without running it.
	goRoot, err := ps6128ReviewGoRoot()
	if err != nil || goRoot == "" {
		t.Fatalf("resolve active Go toolchain root: %v", err)
	}
	asm := exec.Command("go", "tool", "asm", "-I", filepath.Join(goRoot, "pkg", "include"), "-o", filepath.Join(t.TempDir(), "kernel.o"), filepath.Join(dir, "src", "casepkg", "kernel.s"))
	asm.Env = append(os.Environ(), "GOARCH=arm64", "GOOS=darwin")
	if output, err := asm.CombinedOutput(); err != nil {
		t.Fatalf("native source fixture does not assemble: %v\n%s", err, output)
	}
	c := ps6128TestContract()
	c.NativeMainLoopArgument = nativeArgument
	for _, value := range []*string{&c.CapabilityPredicate, &c.DispatchVariable, &c.InstalledFunction, &c.WorkerRunner, &c.NativeSymbol} {
		*value = strings.Replace(*value, "ps6128.", "casepkg.", 1)
	}
	a := &analysis.Analyzer{Name: "PS6128", Doc: "root independent", Run: func(pass *analysis.Pass) (any, error) {
		return runPS6128WithContracts(pass, []config.NativeGenerationDispatchContract{c})
	}}
	analysistest.Run(t, dir, a, "casepkg")
}

func TestPS6128ReviewGoFlow(t *testing.T) {
	t.Parallel()
	replace := func(old, new string) string { return strings.Replace(ps6128ReviewBase, old, new, 1) }
	for _, tc := range []struct {
		name, source string
		finding      bool
	}{
		{"live", ps6128ReviewBase, true},
		{"narrow_control", replace("strings.Contains(brand,\"Apple M1\") || ", ""), false},
		{"helper_returns_supported_brand", replace("return \"Apple M1\"", "return \"Apple M2\""), false},
		{"predicate_equality_rejects_m1", replace("brand := brandString(); return", "brand := brandString(); if brand==\"Apple M1\" {return false}; return"), false},
		{"predicate_constant_false", replace("brand := brandString(); return", "brand := brandString(); if true {return false}; return"), false},
		{"predicate_helper_rejects_m1", replace("brand := brandString(); return", "brand := brandString(); if rejected(brand) {return false}; return") + "\nfunc rejected(s string)bool{return s==\"Apple M1\"}\n", false},
		{"predicate_pointer_overwrite", replace("brand := brandString(); return", "brand := brandString(); p:=&brand; *p=\"Apple M2\"; return"), false},
		{"predicate_deferred_named_result", replace("func amxSupported() bool {", "func amxSupported() (enabled bool) { defer func(){enabled=false}();"), false},
		{"dead_init_install", replace("func init(){", "func init(){ return;"), false},
		{"pointer_overwritten_dispatch", replace("func caller(k int){ gemm(k) }", "func caller(k int){ p:=&gemm; *p=safe; gemm(k) }"), false},
		{"pointer_overwritten_worker", replace("func parallelWork(work func()){ work() }", "func parallelWork(work func()){ p:=&work; *p=func(){}; work() }"), false},
		{"worker_known_false_flag", replace("func parallelWork(work func()){ work() }", "func parallelWork(work func()){ run:=false; if run {work()} }"), false},
		{"dead_installed_branch", replace("func compute(k int){ parallelWork(func(){ tile(k) }) }", "func compute(k int){ if false {parallelWork(func(){tile(k)})} }"), false},
		{"returned_before_worker", replace("func compute(k int){", "func compute(k int){ return;"), false},
		{"dead_native_callback_branch", replace("parallelWork(func(){ tile(k) })", "parallelWork(func(){ if false {tile(k)} })"), false},
		{"unused_nested_native_closure", replace("parallelWork(func(){ tile(k) })", "parallelWork(func(){ _=func(){tile(k)} })"), false},
		{"tail_only_dispatch", replace("func caller(k int){ gemm(k) }", "func caller(k int){ gemm(3) }"), false},
		{"native_tail_only_argument", replace("func compute(k int){ parallelWork(func(){ tile(k) }) }", "func compute(k int){ parallelWork(func(){ tile(3) }) }"), false},
	} {
		t.Run(tc.name, func(t *testing.T) { t.Parallel(); ps6128ReviewRun(t, tc.source, ps6128ReviewAsm, tc.finding) })
	}
}

func TestPS6128ReviewAssemblyFlow(t *testing.T) {
	t.Parallel()
	replace := func(old, new string) string { return strings.Replace(ps6128ReviewAsm, old, new, 1) }
	for _, tc := range []struct {
		name, assembly string
		finding        bool
	}{
		{"live", ps6128ReviewAsm, true},
		{"pair_control", replace("0x5000000000000000", "0x4000000000000000"), false},
		{"line_comment_control", replace(" MOVD $0x5000000000000000, R16", " // MOVD $0x5000000000000000, R16"), false},
		{"destination_overwritten", replace(" LSR $2", " MOVD $0x4000000000000000, R5\n LSR $2"), false},
		{"source_bit_cleared", replace(" ORR R16", " AND $0x4000000000000000, R16, R16\n ORR R16"), false},
		{"macro_is_nop", replace("WORD $0x00201025", "WORD $0xd503201f"), false},
		{"unconditional_skip_quad", replace(" CBZ R15, tail", " B tail"), false},
		{"block_comment_only_quad", "/*\n" + ps6128ReviewAsm + "*/\n" + replace("0x5000000000000000", "0x4000000000000000"), false},
		{"inactive_preprocessor_quad", "#ifdef OMIT_QUAD\n" + ps6128ReviewAsm + "#endif\n" + replace("0x5000000000000000", "0x4000000000000000"), false},
		{"add_clobber", replace(" LSR $2", " ADD R2, R5, R5\n LSR $2"), false},
		{"sub_clobber", replace(" LSR $2", " SUB $1, R5, R5\n LSR $2"), false},
		{"bic_clobber", replace(" LSR $2", " BIC $0x1000000000000000, R5, R5\n LSR $2"), false},
		{"unbound_k_load", replace("MOVD k+0(FP), R3", "MOVD k+0(FP), R2"), false},
		{"unbound_k_guard", replace("CBZ R15, tail", "CBZ R14, tail"), false},
	} {
		t.Run(tc.name, func(t *testing.T) { t.Parallel(); ps6128ReviewRun(t, ps6128ReviewBase, tc.assembly, tc.finding) })
	}
}

func TestPS6128ReviewOwnerPortability(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/ps6128_owner_driver.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Replace(string(data), "//go:build darwin && goexperiment.simd\n\n", "", 1)
	for _, goos := range []string{"linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			t.Parallel()
			dir, cleanup, err := analysistest.WriteFiles(map[string]string{"owner987/owner_driver.go": source})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(cleanup)
			env := append(os.Environ(), "GO111MODULE=off", "GOPATH="+dir, "GOOS="+goos, "GOARCH=amd64", "GOWORK=off")
			pkgs, err := packages.Load(&packages.Config{Mode: packages.NeedName | packages.NeedFiles, Dir: filepath.Join(dir, "src"), Env: env}, "owner987")
			if err != nil {
				t.Fatal(err)
			}
			if len(pkgs) != 1 || len(pkgs[0].GoFiles) == 0 {
				t.Fatalf("permanent owner replay has no selected Go files on %s/amd64; packages=%+v", goos, pkgs)
			}
		})
	}
}
