package ps6079dep

import "testing"

var callback func()

func SetCallback(function func()) {
	callback = function
}

func Invoke() {
	if callback != nil {
		callback()
	}
}

func Retain([]float64) {}
func RetainAny(any)    {}

func PositiveAsync() []float64 {
	values := []float64{1}
	go func() { values[0] = -1 }()
	return values
}

func PositiveAsyncPair() ([]float64, error) {
	return PositiveAsync(), nil
}

func SkipBenchmark(benchmark *testing.B) {
	benchmark.SkipNow()
}
