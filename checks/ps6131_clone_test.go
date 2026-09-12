package checks

import (
	"bytes"
	"go/format"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestPS6131ForcedCloneChangesOnlyObservedPredicate(t *testing.T) {
	t.Parallel()
	pass := ps6131SourcePass(t, ps6131SourceFixture)
	c := ps6131TestContract()
	source, ok := ps6131SourceBound(pass, &c)
	if !ok {
		t.Fatal("missing source binding")
	}
	var original bytes.Buffer
	if err := format.Node(&original, pass.Fset, source.owner.Body); err != nil {
		t.Fatal(err)
	}
	var predicate bytes.Buffer
	if err := format.Node(&predicate, pass.Fset, source.guard.Cond); err != nil {
		t.Fatal(err)
	}
	for _, serial := range []bool{true, false} {
		body, err := ps6131ForcedBody(pass.Fset, source, serial)
		if err != nil {
			t.Fatal(err)
		}
		value := "false"
		if serial {
			value = "true"
		}
		expected := strings.Replace(original.String(), "if "+predicate.String()+" {", "if "+value+" {", 1)
		// Normalize both bodies using identical parsed positions, then compare ALL
		// remaining AST syntax: allocation, calls, return and unreachable arm stay.
		normalize := func(text string) string {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "copy.go", "package copy\nfunc copied() "+text, parser.AllErrors)
			if err != nil {
				t.Fatal(err)
			}
			var formatted bytes.Buffer
			if err := format.Node(&formatted, fset, file); err != nil {
				t.Fatal(err)
			}
			return formatted.String()
		}
		if normalize(body) != normalize(expected) {
			t.Fatalf("clone changed more than predicate: %s", body)
		}
	}
	if _, err := ps6131ForcedBody(pass.Fset, nil, true); err == nil {
		t.Fatal("accepted unobserved clone")
	}
	// Shared production AST is immutable across both regenerations.
	var after bytes.Buffer
	if err := format.Node(&after, pass.Fset, source.owner.Body); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original.Bytes(), after.Bytes()) {
		t.Fatal("generator mutated production source")
	}
}
