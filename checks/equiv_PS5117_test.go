package checks

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

func ps5117CanonicalString(input, separator string) string {
	return strings.Join(strings.Fields(input), separator)
}

func ps5117CanonicalBytes(input, separator []byte) []byte {
	return bytes.Join(bytes.Fields(input), separator)
}

func TestEquiv_PS5117WhitespacePipelinesRandom(t *testing.T) {
	random := rand.New(rand.NewSource(5117))
	separators := []string{" ", "\t", "\r\n", "\v\f", "\u00a0", "\u2003", "\u2028"}
	for iteration := range 50_000 {
		data := make([]byte, random.Intn(256))
		for index := range data {
			data[index] = byte(random.Intn(256))
		}
		separator := separators[random.Intn(len(separators))]
		input := string(data)
		beforeString := ps5117CanonicalString(ps5117CanonicalString(ps5117CanonicalString(input, separator), separator), separator)
		afterString := ps5117CanonicalString(input, separator)
		if beforeString != afterString {
			t.Fatalf("iteration %d string differs for separator %q: before=%q after=%q", iteration, separator, beforeString, afterString)
		}

		beforeBytes := ps5117CanonicalBytes(ps5117CanonicalBytes(ps5117CanonicalBytes(data, []byte(separator)), []byte(separator)), []byte(separator))
		afterBytes := ps5117CanonicalBytes(data, []byte(separator))
		if !bytes.Equal(beforeBytes, afterBytes) ||
			(beforeBytes == nil) != (afterBytes == nil) || cap(beforeBytes) != cap(afterBytes) {
			t.Fatalf("iteration %d bytes differ for separator %q: before=(%q,nil=%t,cap=%d) after=(%q,nil=%t,cap=%d)",
				iteration, separator, beforeBytes, beforeBytes == nil, cap(beforeBytes), afterBytes, afterBytes == nil, cap(afterBytes))
		}
	}
}

func TestEquiv_PS5117NoWhitespaceStringTerminalRandom(t *testing.T) {
	random := rand.New(rand.NewSource(51_170))
	terminalSeparators := []string{"", ",", "::", "\x00", "\xff", "日本語"}
	outerSeparators := []string{" ", "\t", " - ", "x y", "\u00a0", "|"}
	for iteration := range 50_000 {
		data := make([]byte, random.Intn(192))
		for index := range data {
			data[index] = byte(random.Intn(256))
		}
		input := string(data)
		terminal := terminalSeparators[random.Intn(len(terminalSeparators))]
		middle := outerSeparators[random.Intn(len(outerSeparators))]
		outer := outerSeparators[random.Intn(len(outerSeparators))]
		before := ps5117CanonicalString(ps5117CanonicalString(ps5117CanonicalString(input, terminal), middle), outer)
		after := ps5117CanonicalString(input, terminal)
		if before != after {
			t.Fatalf("iteration %d no-space terminal %q differs through %q/%q: before=%q after=%q", iteration, terminal, middle, outer, before, after)
		}
	}
}

func TestEquiv_PS5117RejectedSeparatorCounterexamples(t *testing.T) {
	input := "alpha beta gamma"
	innerSpace := ps5117CanonicalString(input, " ")
	if outerTab := ps5117CanonicalString(innerSpace, "\t"); outerTab == innerSpace {
		t.Fatal("different whitespace separators must be observable")
	}
	mixedOnce := ps5117CanonicalString(input, " - ")
	if mixedTwice := ps5117CanonicalString(mixedOnce, " - "); mixedTwice == mixedOnce {
		t.Fatal("mixed whitespace/non-whitespace separator must not be treated as idempotent")
	}
}

func TestEquiv_PS5117InputEvaluatedOnce(t *testing.T) {
	beforeCalls := 0
	beforeSource := func() string {
		beforeCalls++
		return " alpha\tbeta "
	}
	before := ps5117CanonicalString(ps5117CanonicalString(beforeSource(), " "), " ")
	afterCalls := 0
	afterSource := func() string {
		afterCalls++
		return " alpha\tbeta "
	}
	after := ps5117CanonicalString(afterSource(), " ")
	if before != after || beforeCalls != 1 || afterCalls != 1 {
		t.Fatalf("evaluation differs: before=%q/%d calls after=%q/%d calls", before, beforeCalls, after, afterCalls)
	}
}
