package ps2109

import (
	"fmt"
	format "fmt"
)

// Sprintf whose format contains a literal byte: the output can never be
// empty, so the rewrite is bit-identical (nil-ness included).
func tagged(id int) []byte {
	return []byte(fmt.Sprintf("user=%d", id)) // want `\[\]byte\(fmt\.Sprintf\(\.\.\.\)\) allocates an intermediate string and copies it; fmt\.Appendf\(nil, \.\.\.\) formats the bytes directly`
}

// A spread call forwards its arguments verbatim.
func spread(args ...any) []byte {
	return []byte(fmt.Sprintf("n=%d", args...)) // want `\[\]byte\(fmt\.Sprintf\(\.\.\.\)\) allocates an intermediate string and copies it; fmt\.Appendf\(nil, \.\.\.\) formats the bytes directly`
}

// An aliased fmt import keeps its qualifier: only the selected name changes.
func aliased(v any) []byte {
	return []byte(format.Sprintf("v=%v", v)) // want `\[\]byte\(fmt\.Sprintf\(\.\.\.\)\) allocates an intermediate string and copies it; fmt\.Appendf\(nil, \.\.\.\) formats the bytes directly`
}

// %% is a literal percent sign: proven non-empty.
func percent(p int) []byte {
	return []byte(fmt.Sprintf("%d%%", p)) // want `\[\]byte\(fmt\.Sprintf\(\.\.\.\)\) allocates an intermediate string and copies it; fmt\.Appendf\(nil, \.\.\.\) formats the bytes directly`
}

// Sprintln always appends a newline: never empty.
func line(a, b string) []byte {
	return []byte(fmt.Sprintln(a, b)) // want `\[\]byte\(fmt\.Sprintln\(\.\.\.\)\) allocates an intermediate string and copies it; fmt\.Appendln\(nil, \.\.\.\) formats the bytes directly`
}

// Zero-operand Sprintln still renders "\n".
func bareLine() []byte {
	return []byte(fmt.Sprintln()) // want `\[\]byte\(fmt\.Sprintln\(\.\.\.\)\) allocates an intermediate string and copies it; fmt\.Appendln\(nil, \.\.\.\) formats the bytes directly`
}

// Sprint with an unnamed numeric operand always renders at least one byte.
func joined(x string, n int) []byte {
	return []byte(fmt.Sprint(x, n)) // want `\[\]byte\(fmt\.Sprint\(\.\.\.\)\) allocates an intermediate string and copies it; fmt\.Append\(nil, \.\.\.\) formats the bytes directly`
}

// --- advisory: reported but NOT rewritten ---

// A pure-verb format can render zero bytes (s == ""), and []byte("") is a
// non-nil empty slice while fmt.Appendf(nil, ...) would return nil —
// nil-ness is observable, so no fix.
func pureVerb(s string) []byte {
	return []byte(fmt.Sprintf("%s", s)) // want `\[\]byte\(fmt\.Sprintf\(\.\.\.\)\) allocates an intermediate string and copies it; fmt\.Appendf\(nil, \.\.\.\) formats the bytes directly`
}

// A non-literal format proves nothing about the output length.
func dynFormat(f string, v any) []byte {
	return []byte(fmt.Sprintf(f, v)) // want `\[\]byte\(fmt\.Sprintf\(\.\.\.\)\) allocates an intermediate string and copies it; fmt\.Appendf\(nil, \.\.\.\) formats the bytes directly`
}

// A lone string operand can render empty: advisory only.
func sprintUnproven(s string) []byte {
	return []byte(fmt.Sprint(s)) // want `\[\]byte\(fmt\.Sprint\(\.\.\.\)\) allocates an intermediate string and copies it; fmt\.Append\(nil, \.\.\.\) formats the bytes directly`
}

// --- guards: no report at all ---

type blob []byte

// A named byte-slice conversion changes the expression's type: out of scope.
func named(id int) blob {
	return blob(fmt.Sprintf("user=%d", id))
}

// A plain string conversion has no fmt call inside.
func plain(s string) []byte {
	return []byte(s)
}

// []rune is not []byte.
func runes(id int) []rune {
	return []rune(fmt.Sprintf("user=%d", id))
}

// The intermediate string used on its own is not the pattern.
func keepString(id int) string {
	return fmt.Sprintf("user=%d", id)
}

// A local Sprintf on a value named fmt is not the standard library.
type fake struct{}

func (fake) Sprintf(f string, args ...any) string { return f }

func local(id int) []byte {
	var fmt fake
	return []byte(fmt.Sprintf("user=%d", id))
}
