package ps5087

import (
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"net"
	"strconv"
	"strings"
)

func hexDeep(text string) ([]byte, error) {
	return hex.DecodeString(strings.Clone(strings.Clone(text))) // want `encoding/hex.DecodeString returns input-independent decoded data but receives 2 throwaway strings.Clone layer`
}

func base64Deep(text string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(strings.Clone(strings.Clone(strings.Clone(text)))) // want `encoding/base64.DecodeString returns input-independent decoded data but receives 3 throwaway strings.Clone layer`
}

func base32Decode(text string) ([]byte, error) {
	return base32.HexEncoding.DecodeString(strings.Clone(text)) // want `encoding/base32.DecodeString returns input-independent decoded data but receives 1 throwaway strings.Clone layer`
}

func parseIP(text string) net.IP {
	return net.ParseIP(strings.Clone(text)) // want `net.ParseIP returns input-independent decoded data but receives 1 throwaway strings.Clone layer`
}

func commentPreserved(text string) ([]byte, error) {
	return hex.DecodeString(strings.Clone( /* retention boundary */ text)) // want `encoding/hex.DecodeString returns input-independent decoded data but receives 1 throwaway strings.Clone layer`
}

// NumError retains its input string, so a Clone can be a retention boundary.
func numericErrorMayRetain(text string) (int, error) {
	return strconv.Atoi(strings.Clone(text))
}

func stringResultMayRetain(text string) string {
	return strings.TrimSpace(strings.Clone(text))
}

type decoder struct{}

func (decoder) DecodeString(text string) ([]byte, error) { return []byte(text), nil }

func userMethod(value decoder, text string) ([]byte, error) {
	return value.DecodeString(strings.Clone(text))
}

func methodValue(text string) ([]byte, error) {
	decode := base64.StdEncoding.DecodeString
	return decode(strings.Clone(text))
}

var _ = []any{hexDeep, base64Deep, base32Decode, parseIP, commentPreserved, numericErrorMayRetain, stringResultMayRetain, userMethod, methodValue}
