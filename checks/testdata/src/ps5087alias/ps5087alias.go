package ps5087alias

import (
	b64 "encoding/base64"
	s "strings"
)

func aliasedPackages(text string) ([]byte, error) {
	return b64.RawStdEncoding.DecodeString(s.Clone(text)) // want `encoding/base64.DecodeString returns input-independent decoded data but receives 1 throwaway strings.Clone layer`
}
