package checks

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

func TestEquiv_PS5090StrconvQuoting(t *testing.T) {
	inputs := []string{"", "plain", "quote\"slash\\", "line\nfeed", "世界", string([]byte{0xff, 'a', 0xfe})}
	for index, input := range inputs {
		if before, after := strconv.Quote(strings.Clone(strings.Clone(input))), strconv.Quote(input); before != after {
			t.Fatalf("Quote input %d differs: %q/%q", index, before, after)
		}
		if before, after := strconv.QuoteToASCII(strings.Clone(input)), strconv.QuoteToASCII(input); before != after {
			t.Fatalf("QuoteToASCII input %d differs: %q/%q", index, before, after)
		}
		if before, after := strconv.QuoteToGraphic(strings.Clone(input)), strconv.QuoteToGraphic(input); before != after {
			t.Fatalf("QuoteToGraphic input %d differs: %q/%q", index, before, after)
		}

		dstBefore := make([]byte, 3, 256)
		dstAfter := make([]byte, 3, 256)
		copy(dstBefore, "pre")
		copy(dstAfter, "pre")
		beforeAppend := strconv.AppendQuoteToASCII(dstBefore, strings.Clone(input))
		afterAppend := strconv.AppendQuoteToASCII(dstAfter, input)
		if !bytes.Equal(beforeAppend, afterAppend) || len(beforeAppend) != len(afterAppend) || cap(beforeAppend) != cap(afterAppend) {
			t.Fatalf("AppendQuoteToASCII input %d differs: %q/%q len=%d/%d cap=%d/%d", index, beforeAppend, afterAppend, len(beforeAppend), len(afterAppend), cap(beforeAppend), cap(afterAppend))
		}
	}
}
