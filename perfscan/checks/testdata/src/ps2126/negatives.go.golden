package ps2126

import (
	"fmt"
)

type fakeFmt struct{}

func (fakeFmt) Errorf(string) error { return nil }

// A local variable shadows the package name: fakeFmt's Errorf method is
// not the fmt package function — never flagged.
func shadowedFmt() error {
	fmt := fakeFmt{}
	return fmt.Errorf("boom")
}

// Real formatting: verbs consume further arguments — never flagged.
func realFormatting(x string, n int) error {
	if n < 0 {
		return fmt.Errorf("%s", x)
	}
	return fmt.Errorf("need %d", n)
}

// A '%' in the constant breaks the equivalence even with no verb after
// it: fmt.Errorf("100% sure") yields "100%!s(MISSING)..."-style NOVERB
// mangling, errors.New would not — never flagged.
func percentLiteral() error {
	return fmt.Errorf("100% sure")
}

// A non-constant argument cannot be proven '%'-free — never flagged.
func dynamic(dynamicVar string) error {
	return fmt.Errorf(dynamicVar)
}

// Two arguments mean real formatting — never flagged.
func twoArgs() error {
	return fmt.Errorf("a", "b")
}

// Reported but NOT fixed: a comment inside the rewritten call prefix
// would be destroyed by the edit.
func commented() error {
	return fmt.Errorf( /* keep me */ "boom") // want `fmt\.Errorf on a constant verb-free message runs the whole fmt printer just to copy the string; errors\.New returns the identical \*errors\.errorString without the printer allocation or format scan`
}
