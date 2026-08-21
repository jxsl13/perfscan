package ps3103

// Big is 4104 bytes on every architecture: well above the 128-byte gate.
type Big struct {
	Payload [4096]byte
	ID      int64
}

// Edge is exactly 128 bytes: AT the threshold, deliberately not above it.
type Edge struct {
	B [128]byte
}

// Small is 32 bytes on every architecture.
type Small struct {
	A, B, C, D int64
}

func sinkInt(int64) {}

func sliceByValue(xs []Big) {
	for _, v := range xs { // want `ranging by value copies a 4104-byte Big element on every iteration; iterate by index and take a pointer to the element where aliasing is safe`
		sinkInt(v.ID)
	}
}

func arrayByValue(arr [4]Big) {
	for _, v := range arr { // want `ranging by value copies a 4104-byte Big element on every iteration; iterate by index and take a pointer to the element where aliasing is safe`
		sinkInt(v.ID)
	}
}

func arrayPtrByValue(arr *[4]Big) {
	for _, v := range arr { // want `ranging by value copies a 4104-byte Big element on every iteration; iterate by index and take a pointer to the element where aliasing is safe`
		sinkInt(v.ID)
	}
}

// The `for _, v = range` assignment form copies just the same.
func assignForm(xs []Big) int64 {
	var v Big
	for _, v = range xs { // want `ranging by value copies a 4104-byte Big element on every iteration; iterate by index and take a pointer to the element where aliasing is safe`
		sinkInt(v.ID)
	}
	return v.ID
}

// Named slice types and named element types resolve through Underlying.
type Bigs []Big

func namedSlice(xs Bigs) {
	for _, v := range xs { // want `ranging by value copies a 4104-byte Big element on every iteration; iterate by index and take a pointer to the element where aliasing is safe`
		sinkInt(v.ID)
	}
}

// --- negatives ---

// Small element: the copy is a couple of MOVs, not reported.
func smallElem(xs []Small) {
	for _, v := range xs {
		sinkInt(v.A)
	}
}

// Exactly at the 128-byte threshold: not reported (strictly greater only).
func atThreshold(xs []Edge) {
	for _, v := range xs {
		sinkInt(int64(v.B[0]))
	}
}

// Value variable never read in the body (only expressible in the
// assignment form; `for _, v :=` with unused v does not compile): the
// loop merely defines v's final value, so the aliasing remedy does not
// apply — not reported.
func unusedValue(xs []Big) int64 {
	var v Big
	n := 0
	for _, v = range xs {
		n++
	}
	_ = n
	_ = func(v Big) { sinkInt(v.ID) } // a DIFFERENT v; the range v stays unread in the body
	return v.ID                       // reads only the final element, after the loop
}

// No value variable at all: nothing is copied.
func indexOnly(xs []Big) {
	for i := range xs {
		sinkInt(xs[i].ID)
	}
}

// Blank value: nothing is copied.
func blankValue(xs []Big) int {
	n := 0
	for _, _ = range xs {
		n++
	}
	return n
}

// Map elements are not addressable: &m[k] does not exist, so the indexed
// remedy cannot apply — out of scope, not reported.
func mapValues(m map[string]Big) {
	for _, v := range m {
		sinkInt(v.ID)
	}
}

// A channel receive must copy — no cheaper access exists, not reported.
func chanValues(ch chan Big) {
	for v := range ch {
		sinkInt(v.ID)
	}
}

// Pointer elements are word-sized: not reported.
func ptrElems(xs []*Big) {
	for _, v := range xs {
		sinkInt(v.ID)
	}
}

// Generic element type: its size differs per instantiation, so it is
// never judged against the threshold — not reported.
func genericElem[T any](xs []T, f func(T)) {
	for _, v := range xs {
		f(v)
	}
}

// Generic container of a generic struct: also skipped.
type pair[T any] struct {
	a, b T
	pad  [200]byte
}

func genericPair[T any](xs []pair[T], f func(T)) {
	for _, v := range xs {
		f(v.a)
	}
}
