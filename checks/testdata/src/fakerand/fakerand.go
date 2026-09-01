package fakerand

type Rand struct{}

func (Rand) NormFloat64() float64 { return -1 }
