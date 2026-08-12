package ps3106

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

// --- positives ---

// Large struct value receiver: 4104 bytes copied per call.
func (b Big) Sum() int64 { // want `receiver b of type Big is 4104 bytes; passing it by value copies 4104 bytes on every call — take a \*Big for large values \(advisory\)`
	return b.ID
}

// Large struct value parameter.
func processBig(b Big) { // want `parameter b of type Big is 4104 bytes; passing it by value copies 4104 bytes on every call — take a \*Big for large values \(advisory\)`
	sinkInt(b.ID)
}

// Large array value parameter: [64]int64 is 512 bytes everywhere.
func processArr(arr [64]int64) { // want `parameter arr of type \[64\]int64 is 512 bytes; passing it by value copies 512 bytes on every call — take a \*\[64\]int64 for large values \(advisory\)`
	sinkInt(arr[0])
}

// Multiple names on one field: each named copy is reported.
func processPair(a, b Big) { // want `parameter a of type Big is 4104 bytes; passing it by value copies 4104 bytes on every call — take a \*Big for large values \(advisory\)` `parameter b of type Big is 4104 bytes; passing it by value copies 4104 bytes on every call — take a \*Big for large values \(advisory\)`
	sinkInt(a.ID + b.ID)
}

// Unnamed parameter: the caller still copies at the call boundary.
func unnamedParam(Big) { // want `parameter of type Big is 4104 bytes; passing it by value copies 4104 bytes on every call — take a \*Big for large values \(advisory\)`
}

// --- negatives ---

// Pointer receiver: 8-byte pointer, not reported.
func (b *Big) PtrSum() int64 {
	return b.ID
}

// Small struct value param: a couple of MOVs, not reported.
func processSmall(s Small) {
	sinkInt(s.A)
}

// Exactly at the 128-byte threshold: not reported (strictly greater only).
func atThreshold(e Edge) {
	sinkInt(int64(e.B[0]))
}

// Slice of large elements: the param is a 24-byte header, not reported.
func sliceParam(xs []Big) {
	for i := range xs {
		sinkInt(xs[i].ID)
	}
}

// Map param: a word-sized header, not reported.
func mapParam(m map[string]Big) {
	sinkInt(int64(len(m)))
}

// Generic param: Sizeof is meaningless per instantiation, not reported.
func genericParam[T any](t T) {
	_ = t
}

// Generic struct containing a type parameter: also skipped.
type pair[T any] struct {
	a, b T
	pad  [200]byte
}

func genericPair[T any](p pair[T]) {
	_ = p.a
}

// String param: a 16-byte header regardless of length, not reported.
func stringParam(s string) {
	sinkInt(int64(len(s)))
}

// Blank param: the compiler may elide a copy nobody reads, not reported.
func blankParam(_ Big) {}

// Blank receiver: same reasoning, not reported.
func (_ Big) BlankRecv() {}

// Pointer param to a large struct: 8 bytes, not reported.
func ptrParam(b *Big) {
	sinkInt(b.ID)
}

// Variadic large elements: arrives as a slice header, not reported.
func variadicParam(xs ...Big) {
	for i := range xs {
		sinkInt(xs[i].ID)
	}
}

// Channel and func params: word-sized, not reported.
func chanFuncParam(ch chan Big, f func(Big)) {
	_ = ch
	_ = f
}

// Interface param: two words, not reported.
func ifaceParam(v interface{ Sum() int64 }) {
	sinkInt(v.Sum())
}
