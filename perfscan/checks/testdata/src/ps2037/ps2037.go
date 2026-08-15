package ps2037

func use(string) {}

func next() rune { return 'x' }

// Basic assignment.
func single(r rune) string {
	s := string([]rune{r}) // want `string\(\[\]rune\{r\}\) builds a throwaway single-element rune slice just to encode one rune; string\(rune\(r\)\) encodes it directly with no slice`
	return s
}

// Inside a larger expression: the rewrite happens inside the unchanged
// argument slot, so no parentheses are ever needed.
func expr(r rune, s string) string {
	return s + string([]rune{r}) + "!" // want `string\(\[\]rune\{r\}\) builds a throwaway single-element rune slice just to encode one rune; string\(rune\(r\)\) encodes it directly with no slice`
}

// Call-argument context.
func arg(r rune) {
	use(string([]rune{r})) // want `string\(\[\]rune\{r\}\) builds a throwaway single-element rune slice just to encode one rune; string\(rune\(r\)\) encodes it directly with no slice`
}

// []int32 spells the identical type; the fix reuses the spelling verbatim.
func int32Spelling(r int32) string {
	return string([]int32{r}) // want `string\(\[\]int32\{r\}\) builds a throwaway single-element rune slice just to encode one rune; string\(int32\(r\)\) encodes it directly with no slice`
}

// An untyped constant element takes the element type from context.
func constant() string {
	return string([]rune{65}) // want `string\(\[\]rune\{65\}\) builds a throwaway single-element rune slice just to encode one rune; string\(rune\(65\)\) encodes it directly with no slice`
}

// A rune literal element.
func runeLit() string {
	return string([]rune{'日'}) // want `string\(\[\]rune\{'日'\}\) builds a throwaway single-element rune slice just to encode one rune; string\(rune\('日'\)\) encodes it directly with no slice`
}

// A side-effecting element stays byte-verbatim in place: evaluated
// exactly once in both spellings.
func sideEffect() string {
	return string([]rune{next()}) // want `string\(\[\]rune\{next\(\)\}\) builds a throwaway single-element rune slice just to encode one rune; string\(rune\(next\(\)\)\) encodes it directly with no slice`
}

// A trailing comma is scaffolding too — it becomes part of the closing
// parenthesis edit.
func trailingComma(r rune) string {
	return string([]rune{r,}) // want `string\(\[\]rune\{r\}\) builds a throwaway single-element rune slice just to encode one rune; string\(rune\(r\)\) encodes it directly with no slice`
}

// A parenthesized argument: the parentheses stay, the literal inside is
// rewritten.
func parens(r rune) string {
	return string(([]rune{r})) // want `string\(\[\]rune\{r\}\) builds a throwaway single-element rune slice just to encode one rune; string\(rune\(r\)\) encodes it directly with no slice`
}

// Reported but NOT fixed: a comment inside the deleted scaffolding would
// be destroyed by the edits.
func commented(r rune) string {
	return string([]rune{ // keep me // want `string\(\[\]rune\{r\}\) builds a throwaway single-element rune slice just to encode one rune; string\(rune\(r\)\) encodes it directly with no slice`
		r})
}

// NEGATIVES — no report.

type RuneSlice []rune

type MyString string

// Two elements: a two-rune string, out of scope.
func twoElems(a, b rune) string {
	return string([]rune{a, b})
}

// Empty literal: the empty string, out of scope.
func empty() string {
	return string([]rune{})
}

// A keyed element pads the slice with zero runes — a six-rune string,
// NOT the single-rune encoding.
func keyed(r rune) string {
	return string([]rune{5: r})
}

// A named slice type: deleting its spelling is a different shape (and
// could orphan an import elsewhere); out of scope.
func namedSlice(r rune) string {
	return string(RuneSlice{r})
}

// A named string conversion target is not the predeclared string.
func namedString(r rune) MyString {
	return MyString([]rune{r})
}

// A conversion (not a composite literal) is a different pattern.
func conversion(s string) string {
	return string([]rune(s))
}

// The slice stored in a variable first may have other consumers.
func stored(r rune) string {
	rs := []rune{r}
	return string(rs)
}

// A shadowed string identifier is not the predeclared conversion.
func shadowed(r rune) []rune {
	type string []rune
	return string([]rune{r})
}

// []byte is a different element type entirely.
func bytes(b byte) string {
	return string([]byte{b})
}
