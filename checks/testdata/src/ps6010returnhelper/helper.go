package ps6010returnhelper

func CallFactory(factory func() func(int), output int) {
	factory()(output)
}

func CallFactoryGeneric[F ~func() func(int)](factory F, output int) {
	factory()(output)
}

func CallSecondFactory(factory func() (func(int), func(int)), output int) {
	_, callback := factory()
	callback(output)
}

type CallbackBox struct {
	Callback func(int)
}

func CallBoxFactory(factory func() CallbackBox, output int) {
	factory().Callback(output)
}

func InvokeCallback(callback func(int), output int) {
	callback(output)
}

func InvokeCallbackGeneric[F ~func(int)](callback F, output int) {
	callback(output)
}

func MutateExpandedSlice(_ int, values []float64) {
	values[0]++
}

func MutateExpandedVariadic(_ int, values ...[]float64) {
	values[len(values)-1][0]++
}

type ExpandedCarrier struct {
	Values []float64
}

func (carrier ExpandedCarrier) MutateExpanded(_ int) {
	carrier.Values[0]++
}
