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

func SkipBenchmark(benchmark *testing.B) {
	benchmark.SkipNow()
}
