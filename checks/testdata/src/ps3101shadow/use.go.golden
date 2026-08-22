package ps3101shadow

func use(bool) {}

// Regression (bytes-aliasing audit): "bytes.Contains" here is a METHOD on the
// package-level var bytes declared in decl.go — NOT stdlib bytes.Contains —
// and it mutates its argument. The parser leaves Ident.Obj nil for cross-file
// references, so a syntactic "is it spelled bytes?" guard would emit a hoist
// fix that shares one mutable buffer across iterations. The guard must
// type-resolve the qualifier to the stdlib package: advisory only, no fix.
func shadowed(lines [][]byte, sep string) {
	for _, line := range lines {
		use(bytes.Contains(line, []byte(sep))) // want `\[\]byte\(sep\) copies its operand on every iteration but sep is loop-invariant; hoist the conversion above the loop`
	}
}
