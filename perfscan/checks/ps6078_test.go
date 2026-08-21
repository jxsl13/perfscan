package checks

import (
	"go/parser"
	"go/token"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6078(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6078.Analyzer, "ps6078")
}

func TestPS6078ConstantConditions(t *testing.T) {
	tests := []struct {
		expression string
		known      bool
		value      bool
	}{
		{expression: "fast", known: true, value: false},
		{expression: "!fast", known: true, value: true},
		{expression: "fast == true", known: true, value: false},
		{expression: "false != fast", known: true, value: false},
		{expression: "fast && runtimeReady", known: true, value: false},
		{expression: "!fast || runtimeReady", known: true, value: true},
		{expression: "fast || runtimeReady", known: false},
		{expression: "runtimeReady", known: false},
	}
	for _, test := range tests {
		t.Run(test.expression, func(t *testing.T) {
			expression, err := parser.ParseExpr(test.expression)
			if err != nil {
				t.Fatal(err)
			}
			known, value := ps6078Condition(expression, "fast", false)
			if known != test.known || value != test.value {
				t.Fatalf("ps6078Condition(%q) = (%v, %v), want (%v, %v)", test.expression, known, value, test.known, test.value)
			}
		})
	}
}

func TestPS6078BooleanLiteral(t *testing.T) {
	for _, test := range []struct {
		expression string
		value      bool
		ok         bool
	}{
		{expression: "true", value: true, ok: true},
		{expression: "(false)", value: false, ok: true},
		{expression: "enabled", ok: false},
		{expression: "!false", ok: false},
	} {
		expression, err := parser.ParseExprFrom(token.NewFileSet(), "condition.go", test.expression, 0)
		if err != nil {
			t.Fatal(err)
		}
		value, ok := ps6078BooleanLiteral(expression)
		if value != test.value || ok != test.ok {
			t.Fatalf("ps6078BooleanLiteral(%q) = (%v, %v), want (%v, %v)", test.expression, value, ok, test.value, test.ok)
		}
	}
}
