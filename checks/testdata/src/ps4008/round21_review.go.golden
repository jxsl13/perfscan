package ps4008

func round21Retarget(target **int, replacement *int)           { *target = replacement }
func round21ForwardRetarget(target **int, replacement *int)    { round21Retarget(target, replacement) }
func round21GenericRetarget[T any](target **T, replacement *T) { *target = replacement }
func round21Reset(target *int)                                 { *target = 0 }
func round21Keep(target *int)                                  { *target += 0 }
func round21Invoke(callback func())                            { callback() }
func round21ForwardInvoke(callback func())                     { round21Invoke(callback) }
func round21GenericInvoke[F ~func()](callback F)               { callback() }

type round21PointerBox struct{ pointer *int }
type round21Invoker struct{}

func (round21Invoker) invoke(callback func()) { callback() }

func crossStatementRetargetReportsRound21(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base, other := inner, 0
				pointer := &base
				round21Retarget(&pointer, &other)
				round21Reset(pointer)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func crossStatementWrappedRetargetReportsRound21(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base, other := inner, 0
				pointer := &base
				round21ForwardRetarget(&pointer, &other)
				round21Reset(pointer)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func crossStatementGenericRetargetReportsRound21(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base, other := inner, 0
				pointer := &base
				round21GenericRetarget(&pointer, &other)
				round21Reset(pointer)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func crossStatementFieldRetargetReportsRound21(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base, other := inner, 0
				box := round21PointerBox{pointer: &base}
				round21Retarget(&box.pointer, &other)
				round21Reset(box.pointer)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func crossStatementGenericFieldRetargetReportsRound21(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base, other := inner, 0
				box := round21PointerBox{pointer: &base}
				round21GenericRetarget(&box.pointer, &other)
				round21Reset(box.pointer)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func storedCallbackRMWReportsRound21(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base := inner
				callback := func() { round21Keep(&base) }
				round21Invoke(callback)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func wrappedStoredCallbackRMWReportsRound21(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base := inner
				callback := func() { round21Keep(&base) }
				round21ForwardInvoke(callback)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func genericStoredCallbackRMWReportsRound21(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base := inner
				callback := func() { round21Keep(&base) }
				round21GenericInvoke(callback)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func methodStoredCallbackRMWReportsRound21(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base := inner
				callback := func() { round21Keep(&base) }
				round21Invoker{}.invoke(callback)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func storedCallbackOverwriteControlRound21(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := inner
				callback := func() { round21Reset(&base) }
				round21Invoke(callback)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func retargetBackControlRound21(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base, other := inner, 0
				pointer := &other
				round21Retarget(&pointer, &base)
				round21Reset(pointer)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func distinctFieldRetargetControlRound21(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base, other, final := inner, 0, 0
				baseBox := round21PointerBox{pointer: &base}
				otherBox := round21PointerBox{pointer: &other}
				round21Retarget(&otherBox.pointer, &final)
				round21Reset(baseBox.pointer)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}
