package ps5086

import (
	"bytes"
	"crypto/sha3"
	"encoding/hex"
	"slices"
	"strings"
	"unicode"
	"unicode/utf16"
)

func upperDeep(data []byte) []byte {
	return bytes.ToUpper(bytes.Clone(slices.Clone(bytes.Clone(data)))) // want `bytes.ToUpper already creates independent output but receives 3 throwaway Clone layer`
}

func replaceEveryInput(data, old, replacement []byte) []byte {
	return bytes.Replace(bytes.Clone(slices.Clone(data)), bytes.Clone(old), slices.Clone(bytes.Clone(replacement)), -1) // want `bytes.Replace already creates independent output but receives 5 throwaway Clone layer[(]s[)] across 3 argument`
}

func joinInputs(parts [][]byte, separator []byte) []byte {
	return bytes.Join(slices.Clone(parts), bytes.Clone(separator)) // want `bytes.Join already creates independent output but receives 2 throwaway Clone layer[(]s[)] across 2 argument`
}

func runeCopy(data []byte) []rune {
	return bytes.Runes(bytes.Clone(data)) // want `bytes.Runes already creates independent output but receives 1 throwaway Clone layer`
}

func specialCase(data []byte) []byte {
	return bytes.ToUpperSpecial(unicode.TurkishCase, slices.Clone(data)) // want `bytes.ToUpperSpecial already creates independent output but receives 1 throwaway Clone layer`
}

func concatInputs(left, right []int) []int {
	return slices.Concat(slices.Clone(left), slices.Clone(right)) // want `slices.Concat already creates independent output but receives 2 throwaway Clone layer[(]s[)] across 2 argument`
}

func concatExpanded(parts [][]int) []int {
	return slices.Concat(slices.Clone(parts)...) // want `slices.Concat already creates independent output but receives 1 throwaway Clone layer`
}

type namedInts []int

func repeatNamed(values namedInts) namedInts {
	return slices.Repeat(slices.Clone[namedInts](values), 3) // want `slices.Repeat already creates independent output but receives 1 throwaway Clone layer`
}

func utf16Encode(values []rune) []uint16 {
	return utf16.Encode(slices.Clone(values)) // want `unicode/utf16.Encode already creates independent output but receives 1 throwaway Clone layer`
}

func utf16Decode(values []uint16) []rune {
	return utf16.Decode(slices.Clone(values)) // want `unicode/utf16.Decode already creates independent output but receives 1 throwaway Clone layer`
}

func shake(data []byte, size int) []byte {
	return sha3.SumSHAKE256(bytes.Clone(data), size) // want `crypto/sha3.SumSHAKE256 already creates independent output but receives 1 throwaway Clone layer`
}

func dump(data []byte) string {
	return hex.Dump(bytes.Clone(data)) // want `encoding/hex.Dump already creates independent output but receives 1 throwaway Clone layer`
}

func commentPreserved(data []byte) []byte {
	return bytes.ToLower(bytes.Clone( /* required snapshot */ data)) // want `bytes.ToLower already creates independent output but receives 1 throwaway Clone layer`
}

func mutateAndNeedle(data []byte) []byte {
	if len(data) != 0 {
		data[0] = 'x'
	}
	return []byte("a")
}

// The first Clone is an observable pre-mutation snapshot and must remain. The
// final Clone is still redundant and can be removed independently.
func laterArgumentMutation(data, replacement []byte) []byte {
	return bytes.Replace(bytes.Clone(data), mutateAndNeedle(data), bytes.Clone(replacement), -1) // want `bytes.Replace already creates independent output but receives 1 throwaway Clone layer`
}

func channelReceiveNeedsSnapshot(parts [][]byte, separators <-chan []byte) []byte {
	return bytes.Join(slices.Clone(parts), <-separators)
}

// Callback and retaining/aliasing APIs are intentionally excluded.
func callbackMayMutate(data []byte) []byte {
	return bytes.Map(func(r rune) rune {
		if len(data) != 0 {
			data[0]++
		}
		return r
	}, bytes.Clone(data))
}

func fieldsRetainInput(data []byte) [][]byte {
	return bytes.Fields(bytes.Clone(data))
}

func stringsMayRetainBacking(value string) string {
	return strings.ToUpper(strings.Clone(value))
}

type transformer struct{}

func (transformer) ToUpper(data []byte) []byte { return data }

func userMethod(value transformer, data []byte) []byte {
	return value.ToUpper(bytes.Clone(data))
}

var _ = []any{
	upperDeep, replaceEveryInput, joinInputs, runeCopy, specialCase,
	concatInputs, concatExpanded, repeatNamed, utf16Encode, utf16Decode, shake, dump, commentPreserved,
	laterArgumentMutation, channelReceiveNeedsSnapshot, callbackMayMutate,
	fieldsRetainInput, stringsMayRetainBacking, userMethod,
}
