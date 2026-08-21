package ps2039

import (
	"maps"
	"sync"
)

var mu sync.Mutex

type holder struct {
	data map[string]int
}

func load() map[string]int { return nil }

// Basic make declaration: the hint is inserted after the type argument.
func basicMake(src map[string]int) map[string]int {
	dst := make(map[string]int) // want `dst is populated by maps\.Copy from src but declared without a size hint; pre-size it with make\(map\[\.\.\.\]\.\.\., len\(src\)\) to reserve the buckets once`
	maps.Copy(dst, src)
	return dst
}

// Empty composite literal: rewritten into the make form, the type
// spelling kept byte-verbatim.
func basicLiteral(src map[string]int) map[string]int {
	dst := map[string]int{} // want `dst is populated by maps\.Copy from src but declared without a size hint; pre-size it with make\(map\[\.\.\.\]\.\.\., len\(src\)\) to reserve the buckets once`
	maps.Copy(dst, src)
	return dst
}

// Other key/value types.
func otherTypes(src map[int][]string) map[int][]string {
	dst := make(map[int][]string) // want `dst is populated by maps\.Copy from src but declared without a size hint; pre-size it with make\(map\[\.\.\.\]\.\.\., len\(src\)\) to reserve the buckets once`
	maps.Copy(dst, src)
	return dst
}

// Explicit type instantiation still resolves to maps.Copy.
func instantiated(src map[string]int) map[string]int {
	dst := make(map[string]int) // want `dst is populated by maps\.Copy from src but declared without a size hint; pre-size it with make\(map\[\.\.\.\]\.\.\., len\(src\)\) to reserve the buckets once`
	maps.Copy[map[string]int, map[string]int](dst, src)
	return dst
}

// A trailing comma in the make call survives: the hint is inserted after
// the type argument, before the comma.
func trailingComma(src map[string]int) map[string]int {
	dst := make(map[string]int,) // want `dst is populated by maps\.Copy from src but declared without a size hint; pre-size it with make\(map\[\.\.\.\]\.\.\., len\(src\)\) to reserve the buckets once`
	maps.Copy(dst, src)
	return dst
}

// A parenthesized destination argument still matches; the fix only edits
// the declaration.
func parenDst(src map[string]int) map[string]int {
	dst := make(map[string]int) // want `dst is populated by maps\.Copy from src but declared without a size hint; pre-size it with make\(map\[\.\.\.\]\.\.\., len\(src\)\) to reserve the buckets once`
	maps.Copy((dst), src)
	return dst
}

// ADVISORY — reported without a fix.

// Statements between the declaration and the copy: injecting len(src) at
// the declaration would hoist the read across them (here: across a lock
// acquisition — a new data race).
func gap(src map[string]int) map[string]int {
	dst := make(map[string]int) // want `dst is populated by maps\.Copy from src but declared without a size hint; pre-size it with make\(map\[\.\.\.\]\.\.\., len\(src\)\) to reserve the buckets once \(no auto-fix: statements between the declaration and the copy; hoisting the len read across them is a human decision\)`
	mu.Lock()
	defer mu.Unlock()
	maps.Copy(dst, src)
	return dst
}

// A field-chain source: len(h.data) advice stands, but only a plain
// identifier source gets the mechanical fix.
func fieldSource(h holder) map[string]int {
	dst := make(map[string]int) // want `dst is populated by maps\.Copy from h\.data but declared without a size hint; pre-size it with make\(map\[\.\.\.\]\.\.\., len\(h\.data\)\) to reserve the buckets once \(no auto-fix: the size hint is only injected for a plain identifier source\)`
	maps.Copy(dst, h.data)
	return dst
}

// A comment inside the literal's braces would be destroyed by the
// rewrite.
func commented(src map[string]int) map[string]int {
	dst := map[string]int{ /* keep me */ } // want `dst is populated by maps\.Copy from src but declared without a size hint; pre-size it with make\(map\[\.\.\.\]\.\.\., len\(src\)\) to reserve the buckets once \(no auto-fix: a comment inside the braces would be destroyed by the rewrite\)`
	maps.Copy(dst, src)
	return dst
}

// NEGATIVES — no report.

type indexMap map[string]int

// Already sized.
func sized(src map[string]int) map[string]int {
	dst := make(map[string]int, len(src))
	maps.Copy(dst, src)
	return dst
}

// The map is touched between the declaration and the copy: not a fresh
// empty destination, len(src) would be the wrong count anyway.
func touched(src map[string]int) map[string]int {
	dst := make(map[string]int)
	dst["seed"] = 1
	maps.Copy(dst, src)
	return dst
}

// A call source: len advice would double-evaluate it.
func callSource() map[string]int {
	dst := make(map[string]int)
	maps.Copy(dst, load())
	return dst
}

// Copying a map into itself is a no-op over a fresh map, and len(dst) is
// not in scope on the declaration's right-hand side.
func selfCopy() map[string]int {
	dst := make(map[string]int)
	maps.Copy(dst, dst)
	return dst
}

// A named map type is a different declaration shape.
func namedType(src map[string]int) indexMap {
	dst := indexMap{}
	maps.Copy(dst, src)
	return dst
}

// A go statement runs under different timing.
func goStmt(src map[string]int) map[string]int {
	dst := make(map[string]int)
	go maps.Copy(dst, src)
	return dst
}

// A defer statement runs at function exit, after anything may have
// touched the map.
func deferStmt(src map[string]int) map[string]int {
	dst := make(map[string]int)
	defer maps.Copy(dst, src)
	return dst
}

// A non-identifier destination.
func nonIdentDst(h holder, src map[string]int) {
	maps.Copy(h.data, src)
}

// A shadowed maps is not the stdlib package.
func shadowed(src map[string]int) map[string]int {
	type mapsT struct{}
	var maps struct {
		Copy func(dst, src map[string]int)
	}
	maps.Copy = func(dst, src map[string]int) {}
	_ = mapsT{}
	dst := make(map[string]int)
	maps.Copy(dst, src)
	return dst
}

// The declaration is in a different (outer) block than the copy: the
// pairing is per-block, like PS2104.
func differentBlock(src map[string]int, cond bool) map[string]int {
	dst := make(map[string]int)
	if cond {
		maps.Copy(dst, src)
	}
	return dst
}
