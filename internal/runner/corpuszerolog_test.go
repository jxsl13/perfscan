package runner

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestFixZerologMultiImportComposition pins the richest import composition seen
// in corpus -fix validation (rs/zerolog console.go: two checks fire in one file
// with THREE simultaneous import-state changes, and zerolog's own tests still
// PASS). PS3104 rewrites sort.Strings -> slices.Sort (sort was imported only for
// it, so it is PRUNED and slices ADDED) while PS2129 rewrites
// fmt.Fprint(buf, s) -> io.WriteString(buf, s); "fmt" must be RETAINED because a
// surviving fmt.Sprintf still uses it, and "io" stays for the WriteString. The
// runner must get all three import states right at once and the file must
// compile.
func TestFixZerologMultiImportComposition(t *testing.T) {
	const src = `package p

import (
	"fmt"
	"io"
	"sort"
)

func render(buf io.Writer, fields []string, fv func() string) {
	sort.Strings(fields)
	fmt.Fprint(buf, fv())
	_ = fmt.Sprintf("kept: %d", len(fields))
}
`
	got := string(runFixMode(t, src))

	for _, want := range []string{
		"slices.Sort(fields)",       // PS3104
		"io.WriteString(buf, fv())", // PS2129
		`fmt.Sprintf("kept: %d"`,    // untouched, keeps fmt live
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in the fixed file:\n%s", want, got)
		}
	}
	if strings.Contains(got, "sort.Strings") || strings.Contains(got, "fmt.Fprint(") {
		t.Errorf("sort.Strings/fmt.Fprint should have been rewritten:\n%s", got)
	}
	// Import states: sort PRUNED, slices ADDED, fmt+io RETAINED.
	if strings.Contains(got, `"sort"`) {
		t.Errorf("the now-orphaned sort import must be pruned:\n%s", got)
	}
	for _, imp := range []string{`"slices"`, `"fmt"`, `"io"`} {
		if !strings.Contains(got, imp) {
			t.Errorf("import %s must be present:\n%s", imp, got)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}
