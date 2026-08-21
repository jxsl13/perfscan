package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6058 implements owner issue #720: resolved Metal counter timestamps need
// paired CPU/GPU clock calibration, not a direct frequency/nanosecond cast.
var PS6058 = register(&lint.Check{
	ID:       "PS6058",
	Category: "verify",
	Slug:     "metal-counter-timestamp-needs-clock-calibration",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "resolved Metal counter timestamps are converted without paired CPU/GPU calibration",
		Text: `MTLCounterResultTimestamp values come from a GPU clock and do not
share the CPU nanosecond timebase. Directly treating their deltas as
nanoseconds—or applying MTLDevice.queryTimestampFrequency to resolved counter
samples—can produce implausible stage durations.

This check implements owner issue #720. It inspects typed Go wrappers,
embedded Objective-C/Swift/C-family profiler source, and package-owned native
files (including go:embed .m, .mm, .swift, .c/.cc/.cpp, and headers). It reports
only when Metal timestamp-counter context (MTLCommonCounterSetTimestamp or
MTLCounterResultTimestamp) is combined with:

  - queryTimestampFrequency, or
  - a direct timestamp/delta-to-nanoseconds or time.Duration conversion.

Comments and quoted examples in native source are ignored. A frequency query
outside counter-result context, a profiler that merely resolves raw values,
and a correctly calibrated converter stay silent.

Apple's documented workflow is to call sampleTimestamps()/
sampleTimestamps:gpuTimestamp: before and after the measured command, then
convert each GPU delta by cpuTimestampSpan / gpuTimestampSpan. Also reject
MTLCounterErrorValue samples and compare summed converted intervals with
MTLCommandBuffer GPUStartTime/GPUEndTime as a plausibility gate. The diagnostic
names whichever of those controls are missing.

There is NO automatic fix. Correct sample placement must bracket the actual
asynchronous command, completion-handler lifetime matters, and the source may
use wrappers whose CPU/GPU timestamp ownership cannot be inferred locally.`,
		Before: `let frequency = device.queryTimestampFrequency()
let ns = Double(end.timestamp - begin.timestamp) * 1e9 / Double(frequency)`,
		After: `let start = device.sampleTimestamps()
// execute and complete the measured command
let end = device.sampleTimestamps()
guard sample != MTLCounterErrorValue else { reject() }
let ns = gpuDelta / Double(end.gpu-start.gpu) * Double(end.cpu-start.cpu)
plausibility(sumIntervals: intervals, command: commandBuffer.GPUStartTime...commandBuffer.GPUEndTime)`,
		MeasuredWin: `The Apple-M2-Pro/macOS-26 trace behind issue #720 produced
1.486 seconds of summed encoder time when resolved timestamps were converted
with queryTimestampFrequency. Paired CPU/GPU span calibration corrected the
same trace to 35.70 ms of encoder intervals against 37.97 ms from
MTLCommandBuffer GPUStartTime/GPUEndTime.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6058",
		Doc:  "Metal counter timestamp conversion bypasses paired CPU/GPU clock calibration",
		Run:  runPS6058,
	},
})

var (
	ps6058NativeNanoseconds = regexp.MustCompile(`(?is)\b(?:timestamp|delta|elapsed|duration|timespan)\w*\b[^;\n]{0,160}(?:\*|/)\s*(?:1e9|1000000000|1_000_000_000)\b`)
	ps6058NativeDuration    = regexp.MustCompile(`(?is)\b(?:duration|nanoseconds?)\s*\(\s*(?:[A-Za-z_]\w*\.)?(?:timestamp|delta|elapsed|duration)\w*`)
	ps6058NativeDeclaration = regexp.MustCompile(`(?m)\b(?:func|let|var|guard|void|double|float|uint64_t|struct|class)\b`)
)

type ps6058Problem struct {
	offset      int
	frequency   bool
	nanoseconds bool
	calibrated  bool
	sentinel    bool
	plausible   bool
}

func runPS6058(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if problem, found := ps6058GoProblem(pass, fn); found {
				pass.Reportf(problem.offsetPos(fn), "%s", ps6058Message(problem))
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			lit, ok := node.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			source, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if problem, found := ps6058NativeProblem(source); found {
				pass.Reportf(lit.Pos(), "%s", ps6058Message(problem))
			}
			return true
		})
	}

	seen := make(map[string]bool, len(pass.OtherFiles)+len(pass.IgnoredFiles))
	for _, filename := range slices.Concat(pass.OtherFiles, pass.IgnoredFiles) {
		if seen[filename] || !ps6058NativeExtension(filename) {
			continue
		}
		seen[filename] = true
		source, err := ps6058ReadFile(pass, filename)
		if err != nil {
			continue
		}
		problem, found := ps6058NativeProblem(string(source))
		if !found {
			continue
		}
		file := pass.Fset.AddFile(filename, -1, len(source))
		file.SetLinesForContent(source)
		offset := min(max(problem.offset, 0), len(source))
		pass.Reportf(file.Pos(offset), "%s", ps6058Message(problem))
	}
	return nil, nil
}

// offsetPos uses a negative offset sentinel for the function name and a token
// position encoded as an int for a suspicious Go AST node.
func (problem ps6058Problem) offsetPos(fn *ast.FuncDecl) token.Pos {
	if problem.offset <= 0 {
		return fn.Name.Pos()
	}
	return token.Pos(problem.offset)
}

func ps6058GoProblem(pass *analysis.Pass, fn *ast.FuncDecl) (ps6058Problem, bool) {
	problem := ps6058Problem{offset: int(fn.Name.Pos())}
	counterContext := false
	ast.Inspect(fn.Type, func(node ast.Node) bool {
		id, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		normalized := ps6007NormalizeName(id.Name)
		counterContext = counterContext || ps6007ContainsAny(normalized, "mtlcommoncountersettimestamp", "mtlcounterresulttimestamp")
		return true
	})
	sampleCalls := 0
	cpuSpan, gpuSpan := false, false
	gpuStart, gpuEnd, intervalSum := false, false, false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.Ident:
			normalized := ps6007NormalizeName(value.Name)
			counterContext = counterContext || ps6007ContainsAny(normalized, "mtlcommoncountersettimestamp", "mtlcounterresulttimestamp")
			problem.sentinel = problem.sentinel || strings.Contains(normalized, "mtlcountererrorvalue")
			cpuSpan = cpuSpan || ps6007ContainsAny(normalized, "cputimestampspan", "cputimespan")
			gpuSpan = gpuSpan || ps6007ContainsAny(normalized, "gputimestampspan", "gputimespan")
			gpuStart = gpuStart || strings.Contains(normalized, "gpustarttime")
			gpuEnd = gpuEnd || strings.Contains(normalized, "gpuendtime")
			intervalSum = intervalSum || ps6007ContainsAny(normalized, "suminterval", "totalinterval", "encoderinterval")
		case *ast.CallExpr:
			name, pos := ps6058CallName(value)
			normalized := ps6007NormalizeName(name)
			if strings.Contains(normalized, "querytimestampfrequency") {
				problem.frequency = true
				problem.offset = int(pos)
			}
			if strings.Contains(normalized, "sampletimestamps") {
				sampleCalls++
			}
			if ps6058TimeDuration(pass, value) {
				problem.nanoseconds = true
				if !problem.frequency {
					problem.offset = int(value.Pos())
				}
			}
		case *ast.BinaryExpr:
			if ps6058NanosecondBinary(pass, value) {
				problem.nanoseconds = true
				if !problem.frequency {
					problem.offset = int(value.Pos())
				}
			}
		}
		return true
	})
	problem.calibrated = sampleCalls >= 2 && cpuSpan && gpuSpan
	problem.plausible = gpuStart && gpuEnd && intervalSum
	return problem, counterContext && (problem.frequency || problem.nanoseconds)
}

func ps6058CallName(call *ast.CallExpr) (string, token.Pos) {
	switch value := ps2110Unparen(call.Fun).(type) {
	case *ast.Ident:
		return value.Name, value.Pos()
	case *ast.SelectorExpr:
		return value.Sel.Name, value.Sel.Pos()
	default:
		return "", call.Pos()
	}
}

func ps6058TimeDuration(pass *analysis.Pass, call *ast.CallExpr) bool {
	if len(call.Args) != 1 {
		return false
	}
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Duration" {
		return false
	}
	object, ok := pass.TypesInfo.Uses[selector.Sel].(*types.TypeName)
	if !ok || object.Pkg() == nil || object.Pkg().Path() != "time" {
		return false
	}
	return ps6058TimestampExpr(call.Args[0])
}

func ps6058NanosecondBinary(pass *analysis.Pass, binary *ast.BinaryExpr) bool {
	if binary.Op != token.MUL && binary.Op != token.QUO {
		return false
	}
	return ps6058TimestampExpr(binary.X) && ps6058Billion(pass, binary.Y) || ps6058TimestampExpr(binary.Y) && ps6058Billion(pass, binary.X)
}

func ps6058TimestampExpr(expression ast.Expr) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if found {
			return false
		}
		id, ok := node.(*ast.Ident)
		if ok {
			name := ps6007NormalizeName(id.Name)
			found = ps6007ContainsAny(name, "timestamp", "delta", "elapsed", "duration", "timespan")
		}
		return !found
	})
	return found
}

func ps6058Billion(pass *analysis.Pass, expression ast.Expr) bool {
	value := pass.TypesInfo.Types[ps2110Unparen(expression)].Value
	if value == nil {
		return false
	}
	float, _ := constant.Float64Val(value)
	return float == 1e9
}

func ps6058NativeProblem(source string) (ps6058Problem, bool) {
	clean := ps6053BlankCommentsAndStrings(source)
	if !ps6058NativeDeclaration.MatchString(clean) {
		return ps6058Problem{}, false
	}
	normalized := ps6058Compact(clean)
	counterContext := ps6007ContainsAny(normalized, "mtlcommoncountersettimestamp", "mtlcounterresulttimestamp")
	frequencyOffset := strings.Index(strings.ToLower(clean), "querytimestampfrequency")
	frequency := frequencyOffset >= 0
	nsLocation := ps6058NativeNanoseconds.FindStringIndex(clean)
	if nsLocation == nil {
		nsLocation = ps6058NativeDuration.FindStringIndex(clean)
	}
	nanoseconds := nsLocation != nil
	if !counterContext || !frequency && !nanoseconds {
		return ps6058Problem{}, false
	}
	offset := frequencyOffset
	if offset < 0 && nanoseconds {
		offset = nsLocation[0]
	}
	samples := strings.Count(normalized, "sampletimestamps")
	calibrated := samples >= 2 && ps6007ContainsAny(normalized, "cputimestampspan", "cputimespan") && ps6007ContainsAny(normalized, "gputimestampspan", "gputimespan")
	sentinel := strings.Contains(normalized, "mtlcountererrorvalue")
	plausible := strings.Contains(normalized, "gpustarttime") && strings.Contains(normalized, "gpuendtime") && ps6007ContainsAny(normalized, "suminterval", "totalinterval", "encoderinterval")
	return ps6058Problem{offset: offset, frequency: frequency, nanoseconds: nanoseconds, calibrated: calibrated, sentinel: sentinel, plausible: plausible}, true
}

func ps6058Compact(value string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		default:
			return -1
		}
	}, value)
}

func ps6058Message(problem ps6058Problem) string {
	misuse := make([]string, 0, 2)
	if problem.frequency {
		misuse = append(misuse, "queryTimestampFrequency applied in resolved-counter context")
	}
	if problem.nanoseconds {
		misuse = append(misuse, "GPU timestamp delta treated directly as nanoseconds/time.Duration")
	}
	missing := make([]string, 0, 3)
	if !problem.calibrated {
		missing = append(missing, "paired before/after sampleTimestamps CPU/GPU span calibration")
	}
	if !problem.sentinel {
		missing = append(missing, "MTLCounterErrorValue sentinel rejection")
	}
	if !problem.plausible {
		missing = append(missing, "sum-of-intervals versus GPUStartTime/GPUEndTime plausibility gate")
	}
	if len(missing) == 0 {
		return "invalid Metal counter timestamp conversion: " + strings.Join(misuse, " and ") + "; paired calibration controls are present, but the direct conversion remains invalid (advisory, no automatic fix)"
	}
	return "invalid Metal counter timestamp conversion: " + strings.Join(misuse, " and ") + "; missing " + strings.Join(missing, ", ") + " — convert each GPU delta by cpuTimestampSpan/gpuTimestampSpan (advisory, no automatic fix)"
}

func ps6058NativeExtension(filename string) bool {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".m", ".mm", ".swift", ".c", ".cc", ".cpp", ".h", ".hpp":
		return true
	default:
		return false
	}
}

func ps6058ReadFile(pass *analysis.Pass, filename string) ([]byte, error) {
	if pass.ReadFile != nil {
		return pass.ReadFile(filename)
	}
	return os.ReadFile(filename)
}
