package ps5021neg

// mb is a defined byte type: []mb's element type is NOT uint8, so
// copy(dst, s) would not even type-check ([]mb is not assignable to
// []byte) — the shape must not match at all.
type mb byte

// A plain slice source has no conversion to drop.
func plain(dst, src []byte) int { return copy(dst, src) }

// A slice->slice conversion: the operand is not a string, and dropping it
// would be a different (non-perf) cleanup.
func sliceConv(dst, src []byte) int { return copy(dst, []byte(src)) }

// []byte(nil): the operand is untyped nil, not a string.
func nilConv(dst []byte) int { return copy(dst, []byte(nil)) }

// []rune(s) is a different conversion entirely; copy([]rune, string) does
// not exist.
func runes(dst []rune, s string) int { return copy(dst, []rune(s)) }

// A slice of a NAMED byte type: legal conversion, but no legal rewrite.
func namedElem(dst []mb, s string) int { return copy(dst, []mb(s)) }

// A shadowed copy is not the builtin.
func shadowed(dst []byte, s string) int {
	copy := func(d, b []byte) int { return len(b) }
	return copy(dst, []byte(s))
}

// A method named copy is not the builtin either.
type buffer struct{}

func (buffer) copy(d, s []byte) int { return 0 }

func method(b buffer, dst []byte, s string) int { return b.copy(dst, []byte(s)) }

// The already-rewritten form must stay silent.
func direct(dst []byte, s string) int { return copy(dst, s) }
