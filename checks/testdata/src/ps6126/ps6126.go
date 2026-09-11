package ps6126

var saved func(int, int)

func parallelWork(workers, grain int, work func(int, int)) { saved = work }

func growing(a0, a1, a2, a3, a4, a5, a6, a7, a8 []byte, width byte) {
	parallelWork(4, 64, func(lo, hi int) { // want `escaping callback argument 3 to repeated worker helper ps6126.parallelWork grew from 224 to 232 environment bytes and crossed allocator class 224 to 240 after compiler captures width were added`
		_, _, _, _, _, _, _, _, _, _, _ = a0, a1, a2, a3, a4, a5, a6, a7, a8, width, lo+hi
	})
}

func cold(a0, a1, a2, a3, a4, a5, a6, a7, a8 []byte, width byte) func() {
	return func() { _, _, _, _, _, _, _, _, _, _ = a0, a1, a2, a3, a4, a5, a6, a7, a8, width }
}

func wrongHelper(a0, a1, a2, a3, a4, a5, a6, a7, a8 []byte, width byte) {
	_ = func() { _, _, _, _, _, _, _, _, _, _ = a0, a1, a2, a3, a4, a5, a6, a7, a8, width }
}
