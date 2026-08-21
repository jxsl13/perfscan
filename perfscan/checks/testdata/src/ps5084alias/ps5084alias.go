package ps5084alias

import (
	ba "bytealias"
	b "bytes"
	s "strings"
)

func lastCloneImport(dst, src []byte) int {
	return copy(dst, b.Clone(src)) // want "copy source consumes 1 throwaway standard-library Clone layer"
}

func lastMixedAliasImports(dst []byte, text string) int {
	return copy(dst, b.Clone(ba.Bytes(s.Clone(text)))) // want "copy source consumes 1 throwaway standard-library Clone layer"
}
