package ps6099round4typeswitch

import (
	"math"

	"ps6099round4typeswitch/simdops"
)

func InitializedAsinF64(dst []float64) {
	switch value := any(1); value.(type) {
	case int:
		simdops.AsinF64(dst)
	}
}

func initializedEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Asin exactly once per independent output element.*InitializedAsinF64`
		output[index] = math.Asin(input[index])
	}
}

func AliasedAcosF64(dst []float64) {
	value := any(1)
	alias := value
	switch alias.(type) {
	case int:
		simdops.AcosF64(dst)
	}
}

func aliasedEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Acos exactly once per independent output element.*AliasedAcosF64`
		output[index] = math.Acos(input[index])
	}
}

type code int

func NamedInitializedAtanF64(dst []float64) {
	switch value := any(code(1)); value.(type) {
	case code:
		simdops.AtanF64(dst)
	}
}

func namedInitializedEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Atan exactly once per independent output element.*NamedInitializedAtanF64`
		output[index] = math.Atan(input[index])
	}
}

func ReboundExpF64(dst []float64) {
	value := any(1)
	value = any("changed")
	switch value.(type) {
	case int:
		simdops.ExpF64(dst)
	}
}

func noReboundEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Exp(input[index])
	}
}

func overwrite(*any)

func EscapedLogF64(dst []float64) {
	value := any(1)
	overwrite(&value)
	switch value.(type) {
	case int:
		simdops.LogF64(dst)
	}
}

func noEscapedEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Log(input[index])
	}
}

func SnapshotAliasCbrtF64(dst []float64) {
	value := any(1)
	alias := value
	value = any("changed after snapshot")
	switch alias.(type) {
	case int:
		simdops.CbrtF64(dst)
	}
}

func snapshotAliasEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Cbrt exactly once per independent output element.*SnapshotAliasCbrtF64`
		output[index] = math.Cbrt(input[index])
	}
}
