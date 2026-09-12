package checks

import (
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/jxsl13/perfscan/lint"
	"golang.org/x/tools/go/analysis"
)

var PS6138 = register(&lint.Check{
	ID: "PS6138", Category: "verify", Slug: "arm64-destructive-field-extraction", Level: lint.LevelAggressive,
	Doc: lint.Documentation{
		Title: "an ARM64 counted loop serially extracts adjacent fields with destructive shifts",
		Text: `PS6138 recognizes a bounded instruction-level pattern in package-owned
*_arm64.s files joined to a typed, non-generic, no-body Go function declaration.
A closed literal-count loop must load an unsigned packed byte/halfword/word,
then extract at least three adjacent equal-width fields through contiguous low
masks and destructive logical shifts. Every extraction is consumed before its
destination is overwritten; the source has no other observation and is dead
after the loop. The loop counter, source and destinations are distinct.

Only explicitly modeled scalar instructions are accepted. Calls, extra labels,
alternate entries/exits, unknown instructions, raw WORD opcodes, operand aliases,
and unsupported preprocessing are proof barriers. Bounded zero/one-parameter
local macros are expanded with versioned definitions; macro spelling is not
evidence of instruction semantics. This deliberately excludes many real kernels.

Consider independent UBFX operations at the original constant field positions.
Replacing mask/shift pairs can remove shifts and break their serial dependency
chain, but register pressure, source liveness, scheduling and ISA support still
require target code-generation and whole-boundary benchmarks. Preserve exact
unsigned field values and consumers. Keep the rolled form when measured evidence
favors it; ordinary //perfscan:ignore PS6138 suppression is available.

The genuine final GoAI IQ2_S owner from PR1145 is pinned as an already-optimized
negative. Its original mask/shift pilot was not available in published history:
positive fixtures are explicitly synthetic, not historical owner source or new
measured evidence. No native profiling, benchmark, autofix or universal gain is
claimed.`,
		Before:      "AND $3, R8, R11\nLSR $2, R8, R8\n// repeated for adjacent fields in a closed loop",
		After:       "// Benchmark candidate only, preserving the original packed register:\nUBFX $0, R8, $2, R11\nUBFX $2, R8, $2, R12",
		MeasuredWin: "Owner issue #807 reports Apple M2 Pro / Go 1.26.6 IQ2_S medians 895.5 ns → 875.8 ns in five paired 200 ms screens. This is attributed issue evidence, not a reproduction or a predicted gain for this rule's synthetic fixtures.",
	},
	Analyzer: &analysis.Analyzer{Name: "PS6138", Doc: "ARM64 destructive constant-width field extraction", Run: runPS6138},
})

type ps6138Instruction struct {
	op     string
	args   []string
	offset int
}

type ps6138Finding struct {
	symbol               string
	offset, width, count int
}

func runPS6138(pass *analysis.Pass) (any, error) {
	decls := make(map[string]*ast.FuncDecl)
	linked := make(map[string]bool)
	for _, f := range pass.Files {
		for _, group := range f.Comments {
			for _, comment := range group.List {
				fields := strings.Fields(comment.Text)
				if len(fields) >= 2 && fields[0] == "//go:linkname" {
					linked[fields[1]] = true
				}
			}
		}
	}
	for _, f := range pass.Files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body != nil || fn.Recv != nil || linked[fn.Name.Name] {
				continue
			}
			obj, _ := pass.TypesInfo.Defs[fn.Name].(*types.Func)
			if obj == nil {
				continue
			}
			sig, _ := obj.Type().(*types.Signature)
			if sig != nil && sig.TypeParams().Len() == 0 && !sig.Variadic() {
				decls[fn.Name.Name] = fn
			}
		}
	}
	files := slices.Concat(pass.OtherFiles, pass.IgnoredFiles)
	seen := make(map[string]bool, len(files))
	for _, name := range files {
		if !strings.HasSuffix(name, "_arm64.s") || seen[name] {
			continue
		}
		seen[name] = true
		data, err := ps6053ReadFile(pass, name)
		if err != nil || len(data) > 1<<20 {
			continue
		}
		// A package-local header could redefine registers or instructions. Do
		// not silently treat it as the SDK's flag-only header.
		if _, e := os.Lstat(filepath.Join(filepath.Dir(name), "textflag.h")); !os.IsNotExist(e) {
			continue
		}
		for _, finding := range ps6138Analyze(string(data)) {
			fn := decls[finding.symbol]
			if fn == nil {
				continue
			}
			file := pass.Fset.AddFile(name, -1, len(data))
			file.SetLinesForContent(data)
			pass.Report(analysis.Diagnostic{Pos: fn.Name.Pos(), Message: "ARM64 loop in " + filepath.Base(name) + " extracts " + strconv.Itoa(finding.count) + " adjacent " + strconv.Itoa(finding.width) + "-bit fields with destructive LSR shifts; screen independent UBFX at the original positions to remove shifts and break the serial dependency chain; source/destination liveness, register pressure, target codegen, exact consumers and whole-boundary benchmarks remain mandatory (PS6138 advisory, no automatic fix or guaranteed gain)", Related: []analysis.RelatedInformation{{Pos: file.Pos(finding.offset), Message: "first mask in the closed extraction loop"}}})
		}
	}
	return nil, nil
}

func ps6138Register(s string) int {
	if len(s) < 2 || s[0] != 'R' {
		return -1
	}
	n, e := strconv.Atoi(s[1:])
	if e != nil || n < 0 || n > 25 || n == 18 || s != "R"+strconv.Itoa(n) {
		return -1
	}
	return n
}
func ps6138Immediate(s string) (uint64, bool) {
	if !strings.HasPrefix(s, "$") {
		return 0, false
	}
	n, e := strconv.ParseUint(s[1:], 0, 64)
	return n, e == nil
}

// Instruction effects are deliberately explicit. No last-operand heuristic is
// used for an unknown opcode; unsupported memory, register lists and raw words
// fail closed. Only ordinary R0..R25 are modeled (not SP/ZR/link/frame registers).
func ps6138Effects(i ps6138Instruction) (reads, writes [26]bool, ok bool) {
	reg := func(s string, write bool) bool {
		n := ps6138Register(s)
		if n < 0 {
			return false
		}
		if write {
			writes[n] = true
		} else {
			reads[n] = true
		}
		return true
	}
	value := func(s string) bool {
		if _, yes := ps6138Immediate(s); yes {
			return true
		}
		return reg(s, false)
	}
	mem := func(s string) bool {
		p := strings.IndexByte(s, '(')
		if p < 0 || !strings.HasSuffix(s, ")") {
			return false
		}
		if p > 0 {
			if _, e := strconv.ParseInt(s[:p], 0, 64); e != nil {
				return false
			}
		}
		return reg(s[p+1:len(s)-1], false)
	}
	switch i.op {
	case "MOVBU", "MOVHU", "MOVWU":
		ok = len(i.args) == 2 && mem(i.args[0]) && reg(i.args[1], true)
	case "MOVD":
		if len(i.args) == 2 {
			if ps6138FrameSlot(i.args[0]) {
				ok = reg(i.args[1], true)
			} else if ps6138FrameSlot(i.args[1]) {
				ok = reg(i.args[0], false)
			} else {
				ok = value(i.args[0]) && reg(i.args[1], true)
			}
		}
	case "AND", "LSR", "LSL", "ADD", "SUB", "ORR", "EOR":
		ok = len(i.args) == 3 && value(i.args[0]) && reg(i.args[1], false) && reg(i.args[2], true)
	}
	return
}

func ps6138FrameSlot(s string) bool {
	if !strings.HasSuffix(s, "(FP)") {
		return false
	}
	p := strings.IndexByte(s, '+')
	if p < 1 || !ps6138Identifier(s[:p]) {
		return false
	}
	_, e := strconv.ParseUint(s[p+1:len(s)-4], 10, 32)
	return e == nil
}

func ps6138Analyze(source string) []ps6138Finding {
	code, ok := ps6138Expand(source)
	if !ok {
		return nil
	}
	var findings []ps6138Finding
	for start := 0; start < len(code); start++ {
		if code[start].op != "TEXT" {
			continue
		}
		end := start + 1
		for end < len(code) && code[end].op != "TEXT" {
			end++
		}
		if len(code[start].args) != 3 {
			continue
		}
		frame := strings.TrimPrefix(code[start].args[2], "$0-")
		if code[start].args[1] != "NOSPLIT" || code[start].args[2] != "$0-"+frame {
			continue
		}
		if _, e := strconv.ParseUint(frame, 10, 32); e != nil {
			continue
		}
		symbol := strings.TrimSuffix(strings.TrimPrefix(code[start].args[0], "·"), "(SB)")
		if code[start].args[0] != "·"+symbol+"(SB)" || !ps6138Identifier(symbol) {
			continue
		}
		// A symbol must have one unambiguous owning TEXT definition.
		dups := 0
		for _, i := range code {
			if i.op == "TEXT" && len(i.args) > 0 && i.args[0] == code[start].args[0] {
				dups++
			}
		}
		if dups != 1 {
			continue
		}
		body := code[start+1 : end]
		for label := 1; label < len(body); label++ {
			if body[label].op != "LABEL" || len(body[label].args) != 1 {
				continue
			}
			back := label + 1
			for back < len(body) && body[back].op != "BNE" {
				back++
			}
			if back == len(body) || len(body[back].args) != 1 || body[back].args[0] != body[label].args[0] {
				continue
			}
			if f, yes := ps6138Loop(body, label, back); yes {
				f.symbol = symbol
				findings = append(findings, f)
			}
		}
		start = end - 1
	}
	return findings
}

func ps6138Loop(body []ps6138Instruction, label, back int) (ps6138Finding, bool) {
	f := ps6138Finding{}
	if back-label < 8 {
		return f, false
	}
	setup := body[label-1]
	dec := body[back-1]
	if setup.op != "MOVD" || len(setup.args) != 2 || dec.op != "SUBS" || len(dec.args) != 3 {
		return f, false
	}
	trips, ok := ps6138Immediate(setup.args[0])
	counter := ps6138Register(setup.args[1])
	if !ok || trips < 2 || counter < 0 || dec.args[0] != "$1" || dec.args[1] != setup.args[1] || dec.args[2] != setup.args[1] {
		return f, false
	}
	load := body[label+1]
	bits := map[string]int{"MOVBU": 8, "MOVHU": 16, "MOVWU": 32}[load.op]
	if bits == 0 || len(load.args) != 2 {
		return f, false
	}
	src := ps6138Register(load.args[1])
	if src < 0 || src == counter {
		return f, false
	}
	r, w, ok := ps6138Effects(load)
	if !ok || r[src] || r[counter] || !w[src] {
		return f, false
	}
	// No branch/label before the loop may bypass setup or create another entry.
	for _, i := range body[:label-1] {
		if _, _, ok := ps6138Effects(i); !ok {
			return f, false
		}
	}
	var live [26]bool
	pending := -1
	for index := label + 2; index < back-1; index++ {
		i := body[index]
		if i.op == "AND" && len(i.args) == 3 && i.args[1] == load.args[1] {
			mask, yes := ps6138Immediate(i.args[0])
			dst := ps6138Register(i.args[2])
			width := 0
			for mask > 0 && mask&1 != 0 {
				width++
				mask >>= 1
			}
			if !yes || mask != 0 || width == 0 || width >= bits || dst < 0 || dst == src || dst == counter || pending >= 0 || live[dst] {
				return f, false
			}
			if f.width != 0 && f.width != width {
				return f, false
			}
			f.width = width
			if (f.count+1)*width > bits {
				return f, false
			}
			if f.count == 0 {
				f.offset = i.offset
			}
			live[dst] = true
			pending = dst
			f.count++
			continue
		}
		if i.op == "LSR" && len(i.args) == 3 && i.args[1] == load.args[1] && i.args[2] == load.args[1] {
			distance, yes := ps6138Immediate(i.args[0])
			if !yes || distance != uint64(f.width) || pending < 0 {
				return f, false
			}
			pending = -1
			continue
		}
		reads, writes, yes := ps6138Effects(i)
		if !yes || reads[src] || writes[src] || reads[counter] || writes[counter] {
			return f, false
		}
		for reg := range live {
			if live[reg] && writes[reg] && !reads[reg] {
				return f, false
			}
			if reads[reg] && !writes[reg] {
				live[reg] = false
			}
		}
	}
	if f.count < 3 || pending >= 0 {
		return f, false
	}
	for _, yes := range live {
		if yes {
			return f, false
		}
	}
	// No alternate entry/backedge anywhere in this function. Exit source value
	// must be killed before observation, or the function returns without reading
	// it. Unknown tail instructions are not presumed to preserve this fact.
	dead := false
	returned := false
	for _, i := range body[back+1:] {
		if i.op == "RET" && len(i.args) == 0 {
			returned = true
			break
		}
		reads, writes, yes := ps6138Effects(i)
		if !yes || (!dead && reads[src]) {
			return f, false
		}
		if writes[src] {
			dead = true
		}
	}
	return f, returned
}
