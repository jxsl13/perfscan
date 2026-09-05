package ps6099round5typeswitch

import (
	"math"
	"ps6099round5typeswitch/simdops"
)

func AliasedExpF64(dst []float64) {
	value := any(0)
	pointer := &value
	*pointer = "changed"
	switch value.(type) {
	case int:
		simdops.ExpF64(dst)
	}
}
func DeclaredLogF64(dst []float64) {
	value := any(0)
	var pointer = &value
	*pointer = "changed"
	switch value.(type) {
	case int:
		simdops.LogF64(dst)
	}
}
func change(value *any) int { *value = "changed"; return 1 }
func CalledSinF64(dst []float64) {
	value := any(0)
	ignored := change(&value)
	_ = ignored
	switch value.(type) {
	case int:
		simdops.SinF64(dst)
	}
}
func CapturedCosF64(dst []float64) {
	value := any(0)
	ignored := func() int { value = "changed"; return 1 }()
	_ = ignored
	switch value.(type) {
	case int:
		simdops.CosF64(dst)
	}
}
func SnapshotTanF64(dst []float64) {
	value := any(0)
	snapshot := value
	pointer := &value
	*pointer = "changed"
	switch snapshot.(type) {
	case int:
		simdops.TanF64(dst)
	}
}

func scalarExp(output, input []float64) {
	for index := range input {
		output[index] = math.Exp(input[index])
	}
}

func scalarLog(output, input []float64) {
	for index := range input {
		output[index] = math.Log(input[index])
	}
}

func scalarSin(output, input []float64) {
	for index := range input {
		output[index] = math.Sin(input[index])
	}
}

func scalarCos(output, input []float64) {
	for index := range input {
		output[index] = math.Cos(input[index])
	}
}

func scalarTan(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Tan exactly once per independent output element`
		output[index] = math.Tan(input[index])
	}
}
