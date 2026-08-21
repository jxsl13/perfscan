package checks

import (
	"hash/maphash"
	"mime"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEquiv_PS5082StringCloneFedObservers(t *testing.T) {
	seed := maphash.MakeSeed()
	inputs := []string{
		"",
		"payload",
		"alpha-beta-alpha",
		"Straße",
		"STRASSE",
		string([]byte{0xff, 'a', 0xfe}),
	}
	for ai, a := range inputs {
		for bi, b := range inputs {
			if before, after := strings.Compare(strings.Clone(strings.Clone(a)), strings.Clone(strings.Clone(b))), strings.Compare(a, b); before != after {
				t.Fatalf("Compare input %d/%d: clone=%d direct=%d", ai, bi, before, after)
			}
			if before, after := strings.Contains(strings.Clone(a), strings.Clone(b)), strings.Contains(a, b); before != after {
				t.Fatalf("Contains input %d/%d: clone=%v direct=%v", ai, bi, before, after)
			}
			if before, after := strings.EqualFold(strings.Clone(a), strings.Clone(b)), strings.EqualFold(a, b); before != after {
				t.Fatalf("EqualFold input %d/%d: clone=%v direct=%v", ai, bi, before, after)
			}
		}
		if before, after := utf8.ValidString(strings.Clone(a)), utf8.ValidString(a); before != after {
			t.Fatalf("ValidString input %d: clone=%v direct=%v", ai, before, after)
		}
		if before, after := maphash.String(seed, strings.Clone(a)), maphash.String(seed, a); before != after {
			t.Fatalf("maphash.String input %d: clone=%d direct=%d", ai, before, after)
		}
		beforeRune, beforeSize := utf8.DecodeRuneInString(strings.Clone(a))
		afterRune, afterSize := utf8.DecodeRuneInString(a)
		if beforeRune != afterRune || beforeSize != afterSize {
			t.Fatalf("DecodeRuneInString input %d differs", ai)
		}
	}
}

func TestEquiv_PS5082PathMatchers(t *testing.T) {
	cases := []struct {
		pattern string
		name    string
	}{
		{"", ""},
		{"*.go", "main.go"},
		{"a/?/c", "a/b/c"},
		{"[", "name"},
		{"a\\*b", "a*b"},
		{"**/*.go", "a/b.go"},
	}
	for _, test := range cases {
		beforePath, beforePathErr := path.Match(strings.Clone(test.pattern), strings.Clone(test.name))
		afterPath, afterPathErr := path.Match(test.pattern, test.name)
		if beforePath != afterPath || !sameError(beforePathErr, afterPathErr) {
			t.Fatalf("path.Match %q/%q differs: %v,%v / %v,%v", test.pattern, test.name, beforePath, beforePathErr, afterPath, afterPathErr)
		}
		beforeFilepath, beforeFilepathErr := filepath.Match(strings.Clone(test.pattern), strings.Clone(test.name))
		afterFilepath, afterFilepathErr := filepath.Match(test.pattern, test.name)
		if beforeFilepath != afterFilepath || !sameError(beforeFilepathErr, afterFilepathErr) {
			t.Fatalf("filepath.Match %q/%q differs: %v,%v / %v,%v", test.pattern, test.name, beforeFilepath, beforeFilepathErr, afterFilepath, afterFilepathErr)
		}
	}
}

func TestEquiv_PS5082LookupAndClassificationObservers(t *testing.T) {
	paths := []string{"", ".", "relative/file", "/absolute/file", "../escape", string([]byte{0xff, '/', 'x'})}
	for index, value := range paths {
		if before, after := path.IsAbs(strings.Clone(value)), path.IsAbs(value); before != after {
			t.Fatalf("path.IsAbs input %d differs: clone=%v direct=%v", index, before, after)
		}
		if before, after := filepath.IsAbs(strings.Clone(value)), filepath.IsAbs(value); before != after {
			t.Fatalf("filepath.IsAbs input %d differs: clone=%v direct=%v", index, before, after)
		}
		if before, after := filepath.IsLocal(strings.Clone(value)), filepath.IsLocal(value); before != after {
			t.Fatalf("filepath.IsLocal input %d differs: clone=%v direct=%v", index, before, after)
		}
		if before, after := strconv.CanBackquote(strings.Clone(value)), strconv.CanBackquote(value); before != after {
			t.Fatalf("strconv.CanBackquote input %d differs: clone=%v direct=%v", index, before, after)
		}
	}

	versions := []string{"HTTP/1.0", "HTTP/1.1", "HTTP/2", "http/1.1", "HTTP/9.9", ""}
	for _, version := range versions {
		beforeMajor, beforeMinor, beforeOK := http.ParseHTTPVersion(strings.Clone(version))
		afterMajor, afterMinor, afterOK := http.ParseHTTPVersion(version)
		if beforeMajor != afterMajor || beforeMinor != afterMinor || beforeOK != afterOK {
			t.Fatalf("http.ParseHTTPVersion %q differs: %d.%d,%v / %d.%d,%v", version, beforeMajor, beforeMinor, beforeOK, afterMajor, afterMinor, afterOK)
		}
	}

	for _, extension := range []string{".html", ".JSON", ".unknown-perfscan", ""} {
		if before, after := mime.TypeByExtension(strings.Clone(extension)), mime.TypeByExtension(extension); before != after {
			t.Fatalf("mime.TypeByExtension %q differs: clone=%q direct=%q", extension, before, after)
		}
	}

	const envKey = "PERFSCAN_PS5082_LOOKUP"
	t.Setenv(envKey, "observer-value")
	if before, after := os.Getenv(strings.Clone(envKey)), os.Getenv(envKey); before != after {
		t.Fatalf("os.Getenv differs: clone=%q direct=%q", before, after)
	}
	beforeEnv, beforeFound := os.LookupEnv(strings.Clone(envKey))
	afterEnv, afterFound := os.LookupEnv(envKey)
	if beforeEnv != afterEnv || beforeFound != afterFound {
		t.Fatalf("os.LookupEnv differs: clone=%q,%v direct=%q,%v", beforeEnv, beforeFound, afterEnv, afterFound)
	}

	httpHeader := http.Header{"X-Observer": {"header-value"}}
	if before, after := httpHeader.Get(strings.Clone("x-observer")), httpHeader.Get("x-observer"); before != after {
		t.Fatalf("http.Header.Get differs: clone=%q direct=%q", before, after)
	}
	mimeHeader := textproto.MIMEHeader{"X-Observer": {"mime-value"}}
	if before, after := mimeHeader.Get(strings.Clone("x-observer")), mimeHeader.Get("x-observer"); before != after {
		t.Fatalf("textproto.MIMEHeader.Get differs: clone=%q direct=%q", before, after)
	}
	values := url.Values{"observer": {"query-value"}}
	if before, after := values.Get(strings.Clone("observer")), values.Get("observer"); before != after {
		t.Fatalf("url.Values.Get differs: clone=%q direct=%q", before, after)
	}
	if before, after := values.Has(strings.Clone("observer")), values.Has("observer"); before != after {
		t.Fatalf("url.Values.Has differs: clone=%v direct=%v", before, after)
	}
}
