package checks

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestEquiv_PS5088RegexpBooleanMatches(t *testing.T) {
	patterns := []string{"", "a+", "^payload$", "[[:alpha:]]+", "["}
	byteInputs := [][]byte{nil, {}, []byte("payload"), []byte("bbb"), {0xff, 'a', 0xfe}}
	for _, pattern := range patterns {
		for _, input := range byteInputs {
			before, beforeErr := regexp.Match(pattern, bytes.Clone(slices.Clone(input)))
			after, afterErr := regexp.Match(pattern, input)
			if before != after || reflect.TypeOf(beforeErr) != reflect.TypeOf(afterErr) || fmt.Sprint(beforeErr) != fmt.Sprint(afterErr) {
				t.Fatalf("regexp.Match %q/%v differs: %v,%T %v / %v,%T %v", pattern, input, before, beforeErr, beforeErr, after, afterErr, afterErr)
			}
		}
	}

	compiled := regexp.MustCompile(`(?m)^a.*z$`)
	stringInputs := []string{"", "abcz", "a\nxyz", "世界", string([]byte{0xff, 'a'})}
	for _, input := range stringInputs {
		if before, after := compiled.MatchString(strings.Clone(strings.Clone(input))), compiled.MatchString(input); before != after {
			t.Fatalf("MatchString %q differs: %v/%v", input, before, after)
		}
		bytesInput := []byte(input)
		if before, after := compiled.Match(bytes.Clone(bytesInput)), compiled.Match(bytesInput); before != after {
			t.Fatalf("Match bytes %q differs: %v/%v", input, before, after)
		}
	}
}
