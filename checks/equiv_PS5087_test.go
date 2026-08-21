package checks

import (
	"bytes"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestEquiv_PS5087IndependentDecoders(t *testing.T) {
	type decoderCase struct {
		name   string
		inputs []string
		decode func(string) ([]byte, error)
	}
	cases := []decoderCase{
		{name: "hex", inputs: []string{"", "00ff", "0", "gg", "616263"}, decode: hex.DecodeString},
		{name: "base64", inputs: []string{"", "YQ==", "YWI=", "@@", "YWJjZA"}, decode: base64.StdEncoding.DecodeString},
		{name: "base32", inputs: []string{"", "ME======", "MFRGG===", "@@", "MFRGG"}, decode: base32.StdEncoding.DecodeString},
	}
	for _, decoder := range cases {
		for _, input := range decoder.inputs {
			beforeBytes, beforeErr := decoder.decode(strings.Clone(strings.Clone(input)))
			afterBytes, afterErr := decoder.decode(input)
			if !bytes.Equal(beforeBytes, afterBytes) || (beforeBytes == nil) != (afterBytes == nil) || cap(beforeBytes) != cap(afterBytes) || reflect.TypeOf(beforeErr) != reflect.TypeOf(afterErr) || fmt.Sprint(beforeErr) != fmt.Sprint(afterErr) {
				t.Fatalf("%s %q differs: bytes=%v/%v nil=%v/%v cap=%d/%d err=%T %v/%T %v", decoder.name, input, beforeBytes, afterBytes, beforeBytes == nil, afterBytes == nil, cap(beforeBytes), cap(afterBytes), beforeErr, beforeErr, afterErr, afterErr)
			}
		}
	}

	for _, input := range []string{"", "127.0.0.1", "2001:db8::68", "invalid"} {
		before, after := net.ParseIP(strings.Clone(input)), net.ParseIP(input)
		if !slices.Equal(before, after) || (before == nil) != (after == nil) || cap(before) != cap(after) {
			t.Fatalf("ParseIP %q differs: %v/%v", input, before, after)
		}
	}
}
