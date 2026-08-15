package ps2028

import (
	str "strings"
	"strings"
)

// Every blankness-equivalent comparison shape is rewritten, with the
// literal on either side of the operator.
func forms(s string) {
	_ = len(strings.Fields(s)) == 0 // want `len\(strings\.Fields\(s\)\) == 0 allocates every whitespace-separated field just to test blankness; strings\.TrimSpace\(s\) == "" is the identical boolean with zero allocations and stops at the first non-space byte`
	_ = len(strings.Fields(s)) < 1  // want `len\(strings\.Fields\(s\)\) < 1 allocates every whitespace-separated field just to test blankness; strings\.TrimSpace\(s\) == ""`
	_ = len(strings.Fields(s)) <= 0 // want `len\(strings\.Fields\(s\)\) <= 0 allocates every whitespace-separated field just to test blankness; strings\.TrimSpace\(s\) == ""`
	_ = len(strings.Fields(s)) != 0 // want `len\(strings\.Fields\(s\)\) != 0 allocates every whitespace-separated field just to test blankness; strings\.TrimSpace\(s\) != ""`
	_ = len(strings.Fields(s)) > 0  // want `len\(strings\.Fields\(s\)\) > 0 allocates every whitespace-separated field just to test blankness; strings\.TrimSpace\(s\) != ""`
	_ = len(strings.Fields(s)) >= 1 // want `len\(strings\.Fields\(s\)\) >= 1 allocates every whitespace-separated field just to test blankness; strings\.TrimSpace\(s\) != ""`
	_ = 0 == len(strings.Fields(s)) // want `0 == len\(strings\.Fields\(s\)\) allocates every whitespace-separated field just to test blankness; strings\.TrimSpace\(s\) == ""`
	_ = 0 < len(strings.Fields(s))  // want `0 < len\(strings\.Fields\(s\)\) allocates every whitespace-separated field just to test blankness; strings\.TrimSpace\(s\) != ""`
	_ = 1 > len(strings.Fields(s))  // want `1 > len\(strings\.Fields\(s\)\) allocates every whitespace-separated field just to test blankness; strings\.TrimSpace\(s\) == ""`
	_ = 0 != len(strings.Fields(s)) // want `0 != len\(strings\.Fields\(s\)\) allocates every whitespace-separated field just to test blankness; strings\.TrimSpace\(s\) != ""`
	_ = 1 <= len(strings.Fields(s)) // want `1 <= len\(strings\.Fields\(s\)\) allocates every whitespace-separated field just to test blankness; strings\.TrimSpace\(s\) != ""`
	_ = 0 >= len(strings.Fields(s)) // want `0 >= len\(strings\.Fields\(s\)\) allocates every whitespace-separated field just to test blankness; strings\.TrimSpace\(s\) == ""`
}

// An aliased import keeps its qualifier verbatim; TrimSpace lives in
// the same package, so the import is never orphaned.
func aliased(s string) bool {
	return len(str.Fields(s)) == 0 // want `len\(str\.Fields\(s\)\) == 0 allocates every whitespace-separated field just to test blankness; str\.TrimSpace\(s\) == ""`
}

// The argument expression carries over byte-verbatim: struct fields,
// index expressions, calls, concatenations, and redundant parens all
// stay exactly as written and are evaluated exactly once either way.
type payload struct{ text string }

func args(p payload, parts []string, get func() string) {
	if len(strings.Fields(p.text)) == 0 { // want `len\(strings\.Fields\(p\.text\)\) == 0 allocates every whitespace-separated field just to test blankness; strings\.TrimSpace\(p\.text\) == ""`
		return
	}
	for len(strings.Fields(parts[0])) > 0 { // want `len\(strings\.Fields\(parts\[0\]\)\) > 0 allocates every whitespace-separated field just to test blankness; strings\.TrimSpace\(parts\[0\]\) != ""`
		break
	}
	_ = len(strings.Fields(get())) != 0        // want `len\(strings\.Fields\(get\(\)\)\) != 0 allocates every whitespace-separated field just to test blankness; strings\.TrimSpace\(get\(\)\) != ""`
	_ = len(strings.Fields("a "+p.text)) == 0  // want `allocates every whitespace-separated field just to test blankness`
	_ = len(strings.Fields((p.text))) == 0     // want `allocates every whitespace-separated field just to test blankness`
}

// Redundant parens around the len call, the Fields call, or the Fields
// selector are scaffolding and are consumed with it.
func parens(s string) {
	_ = (len(strings.Fields(s))) == 0     // want `allocates every whitespace-separated field just to test blankness`
	_ = len((strings.Fields(s))) > 0      // want `allocates every whitespace-separated field just to test blankness`
	_ = len((strings.Fields)(s)) == 0     // want `allocates every whitespace-separated field just to test blankness`
}

// A comparison replaces a comparison of the same precedence, so the
// rewrite is parse-identical inside a larger condition.
func condition(ok bool, s string) bool {
	return ok && len(strings.Fields(s)) == 0 // want `allocates every whitespace-separated field just to test blankness`
}

// Both the Before and the After shape are untyped-bool comparisons, so
// a context that materializes a named bool type keeps compiling.
type myBool bool

func namedBoolContext(s string) myBool {
	var b myBool = len(strings.Fields(s)) == 0 // want `allocates every whitespace-separated field just to test blankness`
	return b
}

// A comment inside the scaffolding the fix would delete withholds the
// automatic fix: the report stays advisory and the comment survives.
func commentAdvisory(s string) {
	_ = len( /* keep me */ strings.Fields(s)) == 0 // want `a comment inside the rewritten syntax withholds the automatic fix — rewrite by hand`
	_ = len(strings.Fields(s)) == /* zero */ 0     // want `a comment inside the rewritten syntax withholds the automatic fix — rewrite by hand`
}
