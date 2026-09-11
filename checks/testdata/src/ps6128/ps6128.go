package ps6128

import (
	"os"
	"strings"
)

var gemm func(int)

func amxSupported() bool { // want "native dispatch accepts Apple M1"
	brand := os.Getenv("PS6128_BRAND")
	return strings.Contains(brand, "Apple M1") || strings.Contains(brand, "Apple M2") || strings.Contains(brand, "Apple M3")
}

func init() {
	if amxSupported() {
		gemm = compute
	}
}

func parallelWork(work func()) { work() }

func compute(k int) {
	parallelWork(func() {
		for i := 0; i < 1; i++ {
			tile(k)
		}
	})
}

func caller(k int) { gemm(k) }

func tile(k int)
