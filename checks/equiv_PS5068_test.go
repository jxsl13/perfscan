package checks

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"testing"
)

// TestEquivPS5068 proves the rewrite is bit-identical: fmt.Appendf of a bare
// float verb and strconv.AppendFloat with the matching (format byte, precision,
// bit size) produce the same bytes for every float32/float64 — including -0,
// NaN, both infinities, subnormals, and MaxFloat.
func TestEquivPS5068(t *testing.T) {
	type vc struct {
		v string
		b byte
		p int
	}
	cases := []vc{{"%e", 'e', 6}, {"%E", 'E', 6}, {"%f", 'f', 6}, {"%F", 'f', 6}, {"%G", 'G', -1}}
	edge := []float64{0, math.Copysign(0, -1), 1, -1, 3.14159265358979, 1e-300, 1e300,
		math.SmallestNonzeroFloat64, math.MaxFloat64, math.NaN(), math.Inf(1), math.Inf(-1), 0.1, 100, 123.456}
	for _, c := range cases {
		c64 := func(f float64) {
			a := fmt.Appendf([]byte("s"), c.v, f)
			b := strconv.AppendFloat([]byte("s"), f, c.b, c.p, 64)
			if string(a) != string(b) {
				t.Fatalf("f64 %s f=%v: appendf=%q strconv=%q", c.v, f, a, b)
			}
		}
		c32 := func(f float32) {
			a := fmt.Appendf([]byte("s"), c.v, f)
			b := strconv.AppendFloat([]byte("s"), float64(f), c.b, c.p, 32)
			if string(a) != string(b) {
				t.Fatalf("f32 %s f=%v: appendf=%q strconv=%q", c.v, f, a, b)
			}
		}
		for _, f := range edge {
			c64(f)
			c32(float32(f))
		}
		for i := 0; i < 30000; i++ {
			f := (rand.Float64() - 0.5) * math.Pow(10, float64(rand.Intn(20)-10))
			c64(f)
			c32(float32(f))
		}
	}
}
