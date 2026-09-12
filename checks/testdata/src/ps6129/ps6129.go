package ps6129

func gemm(a, b, c []float32, start, end, depth, columns int) {}
func notGEMM(a, b, c []float32, start, end, depth, columns int) {}
func escape(a []float32) {}

func contraction(n, seq, live, width int, b, out []float32) {
	a := make([]float32, n*seq)
	for r := range n { clear(a[r*seq+live:(r+1)*seq]) }
	gemm(a, b, out, 0, n, seq, width) // want "dense GEMM contracts over explicitly cleared matrix suffix"
}

// Multiple matrices and consumers model dQ/dK/dV flow after normalized clears.
func backward(n, seq, live, width int, b, out, k, v []float32) {
	p := make([]float32, n*seq)
	s := make([]float32, n*seq)
	pt := make([]float32, seq*n)
	st := make([]float32, seq*n)
	for r := range n { clear(p[r*seq+live:(r+1)*seq]) }
	for r := range n { clear(s[r*seq+live:(r+1)*seq]) }
	gemm(s, b, out, 0, n, seq, width) // want "dense GEMM contracts over explicitly cleared matrix suffix"
	for r := range n { for j := range seq { pt[j*n+r] = p[r*seq+j] } }
	for r := range n { for j := range seq { st[j*n+r] = s[r*seq+j] } }
	gemm(pt, b, v, 0, seq, n, width) // want "dense GEMM computes zero suffix output rows"
	gemm(st, b, k, 0, seq, n, width) // want "dense GEMM computes zero suffix output rows"
}

func trimmed(n, seq, live, width int, b, out []float32) {
	a := make([]float32, n*seq)
	for r := range n { clear(a[r*seq+live:(r+1)*seq]) }
	gemm(a, b, out, 0, n, live, width)
}

func trimmedRows(n, seq, live, width int, b, out []float32) {
	a := make([]float32, n*seq)
	t := make([]float32, n*seq)
	for r := range n { clear(a[r*seq+live:(r+1)*seq]) }
	for r := range n { for j := range seq { t[j*n+r] = a[r*seq+j] } }
	gemm(t, b, out, 0, live, n, width)
}

func arbitraryMask(n, seq, live, width int, mask []bool, b, out []float32) {
	a := make([]float32, n*seq)
	for r := range n { if mask[r] { clear(a[r*seq+live:(r+1)*seq]) } }
	gemm(a, b, out, 0, n, seq, width)
}

func stalePooledTail(n, seq, live, width int, a, b, out []float32) {
	for r := range n { clear(a[r*seq+live:(r+1)*seq]) }
	gemm(a, b, out, 0, n, seq, width)
}

func unproven(n, seq, live, width int, b, out []float32) {
	a := make([]float32, n*seq)
	gemm(a, b, out, 0, n, seq, width)
}

func overwritten(n, seq, live, width int, b, out []float32) {
	a := make([]float32, n*seq)
	for r := range n { clear(a[r*seq+live:(r+1)*seq]) }
	a[live] = 2
	gemm(a, b, out, 0, n, seq, width)
}

func escapeBeforeClear(n, seq, live, width int, b, out []float32) {
	a := make([]float32, n*seq)
	escape(a)
	for r := range n { clear(a[r*seq+live:(r+1)*seq]) }
	gemm(a, b, out, 0, n, seq, width)
}

func aliased(n, seq, live, width int, b, out []float32) {
	a := make([]float32, n*seq)
	alias := a
	for r := range n { clear(a[r*seq+live:(r+1)*seq]) }
	gemm(a, b, out, 0, n, seq, width)
	_ = alias
}

func wrongTranspose(n, seq, live, width int, b, out []float32) {
	a := make([]float32, n*seq)
	t := make([]float32, n*seq)
	for r := range n { clear(a[r*seq+live:(r+1)*seq]) }
	for r := range n { for j := range seq { t[j*n+r] = a[j*n+r] } }
	gemm(t, b, out, 0, seq, n, width)
}

func geometryRebound(n, seq, live, width int, b, out []float32) {
	a := make([]float32, n*seq)
	for r := range n { clear(a[r*seq+live:(r+1)*seq]) }
	seq++
	gemm(a, b, out, 0, n, seq, width)
}

func differentIdentity(n, seq, live, width int, b, out []float32) {
	a := make([]float32, n*seq)
	for r := range n { clear(a[r*seq+live:(r+1)*seq]) }
	notGEMM(a, b, out, 0, n, seq, width)
}

func outputOverwritten(n, seq, live, width int, b, out []float32) {
	a := make([]float32, n*seq)
	for r := range n { clear(a[r*seq+live:(r+1)*seq]) }
	gemm(b, b, a, 0, n, seq, width)
	gemm(a, b, out, 0, n, seq, width)
}

func noSuffix(n, seq, width int, b, out []float32) {
	a := make([]float32, n*seq)
	for r := range n { clear(a[r*seq+seq:(r+1)*seq]) }
	gemm(a, b, out, 0, n, seq, width)
}
