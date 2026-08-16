// Package benchmarks holds the Before/After micro-benchmark pairs for
// every perfscan rule whose remedy can be isolated — see README.md for the
// policy and the documented exemptions.
package benchmarks

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Sinks defeat dead-code elimination.
var (
	sinkS  string
	sinkI  int
	sinkF  float64
	sinkSS []string
)

const n = 1024

var (
	words = func() []string {
		out := make([]string, n)
		for i := range out {
			out[i] = "word-" + strconv.Itoa(i)
		}
		return out
	}()
	ints = func() []int {
		out := make([]int, n)
		for i := range out {
			out[i] = i * 7 % n
		}
		return out
	}()
	floats = func() []float64 {
		out := make([]float64, n)
		for i := range out {
			out[i] = float64(i%97) * 1.31
		}
		return out
	}()
)

// PS2002 — builder written in a loop without Grow.
func BenchmarkPS2002_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		var sb strings.Builder
		for _, s := range words {
			sb.WriteString(s)
		}
		sinkS = sb.String()
	}
}

func BenchmarkPS2002_After(b *testing.B) {
	b.ReportAllocs()
	total := 0
	for _, s := range words {
		total += len(s)
	}
	for range b.N {
		var sb strings.Builder
		sb.Grow(total)
		for _, s := range words {
			sb.WriteString(s)
		}
		sinkS = sb.String()
	}
}

// PS2003 — allocating strings transform per iteration vs a Replacer.
func BenchmarkPS2003_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		for _, s := range words {
			sinkS = strings.ReplaceAll(s, "-", "_")
		}
	}
}

func BenchmarkPS2003_After(b *testing.B) {
	b.ReportAllocs()
	r := strings.NewReplacer("-", "_")
	for range b.N {
		for _, s := range words {
			sinkS = r.Replace(s)
		}
	}
}

// PS2005 — regexp compiled per iteration vs hoisted.
func BenchmarkPS2005_Before(b *testing.B) {
	for range b.N {
		hits := 0
		for _, s := range words[:64] {
			if regexp.MustCompile(`^word-1`).MatchString(s) {
				hits++
			}
		}
		sinkI = hits
	}
}

func BenchmarkPS2005_After(b *testing.B) {
	re := regexp.MustCompile(`^word-1`)
	for range b.N {
		hits := 0
		for _, s := range words[:64] {
			if re.MatchString(s) {
				hits++
			}
		}
		sinkI = hits
	}
}

// PS2101 — append without/with preallocation.
func BenchmarkPS2101_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		out := []string{}
		for _, s := range words {
			out = append(out, s)
		}
		sinkSS = out
	}
}

func BenchmarkPS2101_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		out := make([]string, 0, len(words))
		for _, s := range words {
			out = append(out, s)
		}
		sinkSS = out
	}
}

// PS2102 — string += in a loop vs strings.Builder.
func BenchmarkPS2102_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		s := ""
		for _, w := range words[:128] {
			s += w
		}
		sinkS = s
	}
}

func BenchmarkPS2102_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		var sb strings.Builder
		for _, w := range words[:128] {
			sb.WriteString(w)
		}
		sinkS = sb.String()
	}
}

// PS2103 — Sprintf for simple splicing vs concatenation.
func BenchmarkPS2103_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		for i, w := range words[:128] {
			sinkS = fmt.Sprintf("%s:%d", w, i)
		}
	}
}

func BenchmarkPS2103_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		for i, w := range words[:128] {
			sinkS = w + ":" + strconv.Itoa(i)
		}
	}
}

// PS2104 — map fill without/with size hint.
func BenchmarkPS2104_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		m := map[string]int{}
		for i, s := range words {
			m[s] = i
		}
		sinkI = len(m)
	}
}

func BenchmarkPS2104_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		m := make(map[string]int, len(words))
		for i, s := range words {
			m[s] = i
		}
		sinkI = len(m)
	}
}

// PS2008 — one make per row vs one slab with capped views.
func BenchmarkPS2008_Before(b *testing.B) {
	b.ReportAllocs()
	const rows, d = 256, 16
	for range b.N {
		m := make([][]float64, rows)
		for i := range m {
			m[i] = make([]float64, d)
		}
		sinkF = m[rows-1][d-1]
	}
}

func BenchmarkPS2008_After(b *testing.B) {
	b.ReportAllocs()
	const rows, d = 256, 16
	for range b.N {
		m := make([][]float64, rows)
		slab := make([]float64, rows*d)
		for i := range m {
			m[i] = slab[i*d : (i+1)*d : (i+1)*d]
		}
		sinkF = m[rows-1][d-1]
	}
}

// PS3001 — fmt.Sscanf vs manual parsing.
func BenchmarkPS3001_Before(b *testing.B) {
	line := "42 1337"
	for range b.N {
		var x, y int
		fmt.Sscanf(line, "%d %d", &x, &y)
		sinkI = x + y
	}
}

func BenchmarkPS3001_After(b *testing.B) {
	line := "42 1337"
	for range b.N {
		sp := strings.IndexByte(line, ' ')
		x, _ := strconv.Atoi(line[:sp])
		y, _ := strconv.Atoi(line[sp+1:])
		sinkI = x + y
	}
}

// PS3002 is benchmarked accurately (both fix forms: slices.Sort for a
// whole-element compare and slices.SortFunc with cmp.Compare for a field
// compare) in ps3002_test.go.

// PS3003 — dense int-keyed map vs slice.
func BenchmarkPS3003_Before(b *testing.B) {
	m := make(map[int]bool, n)
	for _, v := range ints {
		m[v] = true
	}
	for range b.N {
		hits := 0
		for _, v := range ints {
			if m[v] {
				hits++
			}
		}
		sinkI = hits
	}
}

func BenchmarkPS3003_After(b *testing.B) {
	s := make([]bool, n)
	for _, v := range ints {
		s[v] = true
	}
	for range b.N {
		hits := 0
		for _, v := range ints {
			if s[v] {
				hits++
			}
		}
		sinkI = hits
	}
}

// PS3007 — small membership set as map vs direct scan.
func BenchmarkPS3007_Before(b *testing.B) {
	small := ints[:8]
	for range b.N {
		set := make(map[int]bool, len(small))
		for _, v := range small {
			set[v] = true
		}
		hits := 0
		for _, v := range ints {
			if set[v] {
				hits++
			}
		}
		sinkI = hits
	}
}

func BenchmarkPS3007_After(b *testing.B) {
	small := ints[:8]
	for range b.N {
		hits := 0
		for _, v := range ints {
			if slices.Contains(small, v) {
				hits++
			}
		}
		sinkI = hits
	}
}

// PS3077 — math.Min(math.Max(…)) clamp vs comparison chain.
func BenchmarkPS3077_Before(b *testing.B) {
	for range b.N {
		s := 0.0
		for _, v := range floats {
			s += math.Min(math.Max(v, 10), 100)
		}
		sinkF = s
	}
}

func BenchmarkPS3077_After(b *testing.B) {
	for range b.N {
		s := 0.0
		for _, v := range floats {
			r := v
			if r <= 10 {
				r = 10
			}
			if r >= 100 {
				r = 100
			}
			s += r
		}
		sinkF = s
	}
}

// PS3082 — math.Max call vs NaN-correct builtin wrapper.
func fmax(a, c float64) float64 {
	if r := max(a, c); r == r {
		return r
	}
	return math.Max(a, c)
}

func BenchmarkPS3082_Before(b *testing.B) {
	for range b.N {
		hi := math.Inf(-1)
		for _, v := range floats {
			hi = math.Max(hi, v)
		}
		sinkF = hi
	}
}

func BenchmarkPS3082_After(b *testing.B) {
	for range b.N {
		hi := math.Inf(-1)
		for _, v := range floats {
			hi = fmax(hi, v)
		}
		sinkF = hi
	}
}

// PS3083 — per-pass int-keyed map vs generation-stamped slice.
func BenchmarkPS3083_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		for pass := 0; pass < 16; pass++ {
			sel := make(map[int]bool, 8)
			for _, v := range ints[:8] {
				sel[v] = true
			}
			hits := 0
			for _, v := range ints {
				if sel[v] {
					hits++
				}
			}
			sinkI = hits
		}
	}
}

func BenchmarkPS3083_After(b *testing.B) {
	b.ReportAllocs()
	stamp := make([]uint32, n)
	gen := uint32(0)
	for range b.N {
		for pass := 0; pass < 16; pass++ {
			gen++
			for _, v := range ints[:8] {
				stamp[v] = gen
			}
			hits := 0
			for _, v := range ints {
				if stamp[v] == gen {
					hits++
				}
			}
			sinkI = hits
		}
	}
}

// PS3101 — loop-invariant conversion per iteration vs hoisted.
func BenchmarkPS3101_Before(b *testing.B) {
	b.ReportAllocs()
	sep := "separator-string"
	lines := make([][]byte, 64)
	for i := range lines {
		lines[i] = []byte(words[i])
	}
	for range b.N {
		hits := 0
		for _, line := range lines {
			if bytes.Contains(line, []byte(sep)) {
				hits++
			}
		}
		sinkI = hits
	}
}

func BenchmarkPS3101_After(b *testing.B) {
	b.ReportAllocs()
	sep := "separator-string"
	lines := make([][]byte, 64)
	for i := range lines {
		lines[i] = []byte(words[i])
	}
	bsep := []byte(sep)
	for range b.N {
		hits := 0
		for _, line := range lines {
			if bytes.Contains(line, bsep) {
				hits++
			}
		}
		sinkI = hits
	}
}

// PS4101 — element-copy loop vs the copy builtin.
func BenchmarkPS4101_Before(b *testing.B) {
	dst := make([]float64, n)
	for range b.N {
		for i := range floats {
			dst[i] = floats[i]
		}
		sinkF = dst[n-1]
	}
}

func BenchmarkPS4101_After(b *testing.B) {
	dst := make([]float64, n)
	for range b.N {
		copy(dst, floats)
		sinkF = dst[n-1]
	}
}

// PS5002 — full symmetric accumulation vs triangle + mirror.
func BenchmarkPS5002_Before(b *testing.B) {
	const d = 64
	m := make([][]float64, d)
	for i := range m {
		m[i] = make([]float64, d)
	}
	x := floats[:d]
	for range b.N {
		for i := 0; i < d; i++ {
			for j := 0; j < d; j++ {
				m[i][j] += x[i] * x[j]
			}
		}
	}
	sinkF = m[d-1][d-1]
}

func BenchmarkPS5002_After(b *testing.B) {
	const d = 64
	m := make([][]float64, d)
	for i := range m {
		m[i] = make([]float64, d)
	}
	x := floats[:d]
	for range b.N {
		for i := 0; i < d; i++ {
			for j := 0; j <= i; j++ {
				m[i][j] += x[i] * x[j]
			}
		}
		for i := 0; i < d; i++ {
			for j := i + 1; j < d; j++ {
				m[i][j] = m[j][i]
			}
		}
	}
	sinkF = m[d-1][d-1]
}

// PS5008 — separate Sin and Cos vs fused Sincos.
func BenchmarkPS5008_Before(b *testing.B) {
	for range b.N {
		s, c := 0.0, 0.0
		for _, v := range floats {
			s += math.Sin(v)
			c += math.Cos(v)
		}
		sinkF = s + c
	}
}

func BenchmarkPS5008_After(b *testing.B) {
	for range b.N {
		s, c := 0.0, 0.0
		for _, v := range floats {
			sv, cv := math.Sincos(v)
			s += sv
			c += cv
		}
		sinkF = s + c
	}
}

// PS1006 — inner-strided reduction vs interchanged contiguous walk.
func BenchmarkPS1006_Before(b *testing.B) {
	const rows, cols = 128, 128
	a := make([]float64, rows*cols)
	copy(a, floats)
	out := make([]float64, cols)
	for range b.N {
		for c := 0; c < cols; c++ {
			s := 0.0
			for r := 0; r < rows; r++ {
				s += a[r*cols+c]
			}
			out[c] = s
		}
	}
	sinkF = out[cols-1]
}

func BenchmarkPS1006_After(b *testing.B) {
	const rows, cols = 128, 128
	a := make([]float64, rows*cols)
	copy(a, floats)
	out := make([]float64, cols)
	for range b.N {
		clear(out)
		for r := 0; r < rows; r++ {
			base := r * cols
			for c := 0; c < cols; c++ {
				out[c] += a[base+c]
			}
		}
	}
	sinkF = out[cols-1]
}

// PS1007 — restreamed output row vs outer loop unrolled by two.
func BenchmarkPS1007_Before(b *testing.B) {
	const rows, dim = 128, 128
	in := make([]float64, rows*dim)
	copy(in, floats)
	w := floats[:rows]
	out := make([]float64, dim)
	for range b.N {
		clear(out)
		for i := 0; i < rows; i++ {
			v := w[i]
			for d := 0; d < dim; d++ {
				out[d] += v * in[i*dim+d]
			}
		}
	}
	sinkF = out[dim-1]
}

func BenchmarkPS1007_After(b *testing.B) {
	const rows, dim = 128, 128
	in := make([]float64, rows*dim)
	copy(in, floats)
	w := floats[:rows]
	out := make([]float64, dim)
	for range b.N {
		clear(out)
		for i := 0; i+1 < rows; i += 2 {
			v0, v1 := w[i], w[i+1]
			r0 := in[i*dim : (i+1)*dim]
			r1 := in[(i+1)*dim : (i+2)*dim]
			for d := 0; d < dim; d++ {
				out[d] += v0 * r0[d]
				out[d] += v1 * r1[d]
			}
		}
	}
	sinkF = out[dim-1]
}

// PS6010 / PS4008 — serial dot accumulator vs four independent
// accumulators sharing one pass (unroll-and-jam / axpy family).
func BenchmarkPS6010_Before(b *testing.B) {
	const out, inn = 64, 256
	a := make([]float64, inn)
	copy(a, floats)
	w := make([]float64, inn*out)
	copy(w, floats)
	dst := make([]float64, out)
	for range b.N {
		for o := 0; o < out; o++ {
			acc := 0.0
			for i := 0; i < inn; i++ {
				acc += a[i] * w[i*out+o]
			}
			dst[o] = acc
		}
	}
	sinkF = dst[out-1]
}

func BenchmarkPS6010_After(b *testing.B) {
	const out, inn = 64, 256
	a := make([]float64, inn)
	copy(a, floats)
	w := make([]float64, inn*out)
	copy(w, floats)
	dst := make([]float64, out)
	for range b.N {
		for o := 0; o+3 < out; o += 4 {
			var a0, a1, a2, a3 float64
			for i := 0; i < inn; i++ {
				ai := a[i]
				a0 += ai * w[i*out+o]
				a1 += ai * w[i*out+o+1]
				a2 += ai * w[i*out+o+2]
				a3 += ai * w[i*out+o+3]
			}
			dst[o], dst[o+1], dst[o+2], dst[o+3] = a0, a1, a2, a3
		}
	}
	sinkF = dst[out-1]
}

// PS1010 — column walk over [][]T vs row-major pass.
func BenchmarkPS1010_Before(b *testing.B) {
	const rows, d = 128, 64
	m := make([][]float64, rows)
	for i := range m {
		m[i] = make([]float64, d)
		copy(m[i], floats[:d])
	}
	mean := make([]float64, d)
	for range b.N {
		for j := 0; j < d; j++ {
			s := 0.0
			for i := range m {
				s += m[i][j]
			}
			mean[j] = s / rows
		}
	}
	sinkF = mean[d-1]
}

func BenchmarkPS1010_After(b *testing.B) {
	const rows, d = 128, 64
	m := make([][]float64, rows)
	for i := range m {
		m[i] = make([]float64, d)
		copy(m[i], floats[:d])
	}
	mean := make([]float64, d)
	for range b.N {
		clear(mean)
		for i := range m {
			for j, v := range m[i] {
				mean[j] += v
			}
		}
		for j := range mean {
			mean[j] /= rows
		}
	}
	sinkF = mean[d-1]
}

// PS3066 — consecutive passes over one buffer vs a merged pass.
func BenchmarkPS3066_Before(b *testing.B) {
	state := make([]float64, n)
	copy(state, floats)
	k := floats
	for range b.N {
		s := 0.0
		for i := range state {
			state[i] *= 0.99
		}
		for i := range state {
			s += state[i] * k[i]
		}
		for i := range state {
			state[i] += 0.001
		}
		sinkF = s
	}
}

func BenchmarkPS3066_After(b *testing.B) {
	state := make([]float64, n)
	copy(state, floats)
	k := floats
	for range b.N {
		s := 0.0
		for i := range state {
			state[i] *= 0.99
			s += state[i] * k[i]
			state[i] += 0.001
		}
		sinkF = s
	}
}

// PS3005 — comparator dereferencing a 2-D structure vs a flat key column.
func BenchmarkPS3005_Before(b *testing.B) {
	const rows = 512
	m := make([][]float64, rows)
	for i := range m {
		m[i] = []float64{floats[i%n], 1}
	}
	idx := make([]int, rows)
	for range b.N {
		for i := range idx {
			idx[i] = i
		}
		sort.Slice(idx, func(a, c int) bool { return m[idx[a]][0] < m[idx[c]][0] })
		sinkI = idx[0]
	}
}

func BenchmarkPS3005_After(b *testing.B) {
	const rows = 512
	m := make([][]float64, rows)
	for i := range m {
		m[i] = []float64{floats[i%n], 1}
	}
	idx := make([]int, rows)
	key := make([]float64, rows)
	for range b.N {
		for i := range idx {
			idx[i] = i
			key[i] = m[i][0]
		}
		sort.Slice(idx, func(a, c int) bool { return key[idx[a]] < key[idx[c]] })
		sinkI = idx[0]
	}
}

// PS2004 — per-iteration scratch make vs a reused buffer.
func BenchmarkPS2004_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		for range 64 {
			scratch := make([]float64, 256)
			scratch[0] = 1
			sinkF = scratch[0]
		}
	}
}

func BenchmarkPS2004_After(b *testing.B) {
	b.ReportAllocs()
	scratch := make([]float64, 256)
	for range b.N {
		for range 64 {
			scratch[0] = 1
			sinkF = scratch[0]
		}
	}
}

// PS2006 — self-concat per step vs an amortized row buffer.
func BenchmarkPS2006_Before(b *testing.B) {
	b.ReportAllocs()
	const steps, width = 128, 64
	row := floats[:width]
	for range b.N {
		var cache []float64
		for range steps {
			next := make([]float64, len(cache)+width)
			copy(next, cache)
			copy(next[len(cache):], row)
			cache = next
		}
		sinkF = cache[len(cache)-1]
	}
}

func BenchmarkPS2006_After(b *testing.B) {
	b.ReportAllocs()
	const steps, width = 128, 64
	row := floats[:width]
	for range b.N {
		cache := make([]float64, 0, 8*width)
		for range steps {
			cache = append(cache, row...) // doubling backing store, prefix view
		}
		sinkF = cache[len(cache)-1]
	}
}

// PS5101 — bytes.Compare(...)==0 for equality vs bytes.Equal. One pair has
// equal content (both must scan; Equal's memequal is tuned harder), one is
// a proper prefix (Equal short-circuits on the length mismatch, Compare
// scans the whole common prefix).
func BenchmarkPS5101_Before(b *testing.B) {
	b.ReportAllocs()
	x := []byte(strings.Repeat("perfscan-", 128))
	y := append([]byte(nil), x...)
	p := x[:len(x)-1]
	for range b.N {
		eq := 0
		if bytes.Compare(x, y) == 0 {
			eq++
		}
		if bytes.Compare(x, p) != 0 {
			eq++
		}
		sinkI = eq
	}
}

func BenchmarkPS5101_After(b *testing.B) {
	b.ReportAllocs()
	x := []byte(strings.Repeat("perfscan-", 128))
	y := append([]byte(nil), x...)
	p := x[:len(x)-1]
	for range b.N {
		eq := 0
		if bytes.Equal(x, y) {
			eq++
		}
		if !bytes.Equal(x, p) {
			eq++
		}
		sinkI = eq
	}
}

// PS2105 — range over []rune(s) vs ranging the string directly. Lines
// longer than the compiler's 32-rune stack buffer force the []rune
// conversion to heap-allocate, as real-world text does.
func BenchmarkPS2105_Before(b *testing.B) {
	b.ReportAllocs()
	line := strings.Join(words[:16], " ")
	for range b.N {
		total := 0
		for range 128 {
			for _, r := range []rune(line) {
				total += int(r)
			}
		}
		sinkI = total
	}
}

func BenchmarkPS2105_After(b *testing.B) {
	b.ReportAllocs()
	line := strings.Join(words[:16], " ")
	for range b.N {
		total := 0
		for range 128 {
			for _, r := range line {
				total += int(r)
			}
		}
		sinkI = total
	}
}

// PS2106 — consecutive single-value appends vs one combined call.
func BenchmarkPS2106_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		var out []string
		out = append(out, words[0])
		out = append(out, words[1])
		out = append(out, words[2])
		out = append(out, words[3])
		out = append(out, words[4])
		out = append(out, words[5])
		out = append(out, words[6])
		out = append(out, words[7])
		sinkSS = out
	}
}

func BenchmarkPS2106_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		var out []string
		out = append(out, words[0], words[1], words[2], words[3],
			words[4], words[5], words[6], words[7])
		sinkSS = out
	}
}

// PS2107 — single-verb Sprintf vs the direct strconv conversion.
func BenchmarkPS2107_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		for _, i := range ints {
			sinkS = fmt.Sprintf("%d", i)
		}
	}
}

func BenchmarkPS2107_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		for _, i := range ints {
			sinkS = strconv.Itoa(i)
		}
	}
}

// PS2107 float verb (%e) — fmt.Sprintf vs strconv.FormatFloat('e', 6, 64).
func BenchmarkPS2107Float_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		for _, f := range floats {
			sinkS = fmt.Sprintf("%e", f)
		}
	}
}

func BenchmarkPS2107Float_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		for _, f := range floats {
			sinkS = strconv.FormatFloat(f, 'e', 6, 64)
		}
	}
}
