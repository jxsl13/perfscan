package ps6080

import "testing"

var testFileRootQuant TestFileRootQuant

func testFileQMatMulLayerOne(quant TestFileRootQuant) bool {
	switch quant {
	case TestFileRootA, TestFileRootC:
		return true
	default:
		return false
	}
}

func testFileQMatMulLayerTwo(quant TestFileRootQuant) bool {
	switch quant {
	case TestFileRootA, TestFileRootC:
		return true
	default:
		return false
	}
}

func TestMatMul(t *testing.T) {
	t.Parallel()
	_ = testFileQMatMulLayerOne(testFileRootQuant)
	_ = testFileQMatMulLayerTwo(testFileRootQuant)
}

func BenchmarkQMatMul(b *testing.B) {
	for range b.N {
		_ = testFileQMatMulLayerOne(testFileRootQuant)
		_ = testFileQMatMulLayerTwo(testFileRootQuant)
	}
}
