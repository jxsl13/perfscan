package runner

import (
	"strings"
	"testing"
)

func TestFixStdlibWriterCloneChainsReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import (
	"bytes"
	"slices"
	"strings"
)

func write(buffer *bytes.Buffer, data []byte, text string) {
	_, _ = buffer.Write(slices.Clone(slices.Clone(data)))
	_, _ = buffer.WriteString(strings.Clone(strings.Clone(text)))
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{"buffer.Write(data)", "buffer.WriteString(text)"} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected writer fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Contains(pass1, ".Clone(") || strings.Contains(pass1, `"slices"`) || strings.Contains(pass1, `"strings"`) {
		t.Fatalf("all clone layers and orphaned imports must disappear in pass 1:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("stdlib writer clone rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixLenCloneStringConversionReachesFixedPointInOnePass(t *testing.T) {
	const source = `package p

import (
	"bytes"
	"strings"
)

func size(text string) int {
	return len(bytes.Clone([]byte(strings.Clone(strings.Clone(text)))))
}
`
	pass1 := string(runFixMode(t, source))
	if !strings.Contains(pass1, "return len(text)") {
		t.Fatalf("expected direct string length in pass 1:\n%s", pass1)
	}
	if strings.Contains(pass1, ".Clone(") || strings.Contains(pass1, `"bytes"`) || strings.Contains(pass1, `"strings"`) {
		t.Fatalf("all clone/conversion scaffolding and orphaned imports must disappear in pass 1:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("len clone/conversion rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixIndependentTransformerCloneChainsReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import (
	"bytes"
	"slices"
)

func upper(data []byte) []byte {
	return bytes.ToUpper(bytes.Clone(slices.Clone(slices.Clone(data))))
}
`
	pass1 := string(runFixMode(t, source))
	if !strings.Contains(pass1, "return bytes.ToUpper(data)") {
		t.Fatalf("expected direct transformer input in pass 1:\n%s", pass1)
	}
	if strings.Contains(pass1, ".Clone(") || strings.Contains(pass1, `"slices"`) {
		t.Fatalf("all clone layers and the orphaned import must disappear in pass 1:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("independent transformer clone rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixIndependentDecoderCloneChainsReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import (
	"encoding/base64"
	"strings"
)

func decode(text string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(strings.Clone(strings.Clone(text)))
}
`
	pass1 := string(runFixMode(t, source))
	if !strings.Contains(pass1, "DecodeString(text)") {
		t.Fatalf("expected direct decoder input in pass 1:\n%s", pass1)
	}
	if strings.Contains(pass1, ".Clone(") || strings.Contains(pass1, `"strings"`) {
		t.Fatalf("all clone layers and the orphaned import must disappear in pass 1:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("independent decoder clone rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixRegexpSubjectCloneChainsReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import (
	"regexp"
	"strings"
)

func match(compiled *regexp.Regexp, subject string) bool {
	return compiled.MatchString(strings.Clone(strings.Clone(subject)))
}
`
	pass1 := string(runFixMode(t, source))
	if !strings.Contains(pass1, "MatchString(subject)") {
		t.Fatalf("expected direct regexp subject in pass 1:\n%s", pass1)
	}
	if strings.Contains(pass1, ".Clone(") || strings.Contains(pass1, `"strings"`) {
		t.Fatalf("all clone layers and the orphaned import must disappear in pass 1:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("regexp subject clone rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixOSWriteCloneChainsReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import (
	"bytes"
	"os"
	"slices"
)

func write(file *os.File, data []byte) (int, error) {
	return file.Write(bytes.Clone(slices.Clone(bytes.Clone(data))))
}
`
	pass1 := string(runFixMode(t, source))
	if !strings.Contains(pass1, "file.Write(data)") {
		t.Fatalf("expected direct os write input in pass 1:\n%s", pass1)
	}
	if strings.Contains(pass1, ".Clone(") || strings.Contains(pass1, `"bytes"`) || strings.Contains(pass1, `"slices"`) {
		t.Fatalf("all clone layers and orphaned imports must disappear in pass 1:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("os write clone rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixStrconvQuoteCloneChainsReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import (
	"strconv"
	"strings"
)

func quote(text string) string {
	return strconv.Quote(strings.Clone(strings.Clone(text)))
}
`
	pass1 := string(runFixMode(t, source))
	if !strings.Contains(pass1, "strconv.Quote(text)") {
		t.Fatalf("expected direct strconv quote input in pass 1:\n%s", pass1)
	}
	if strings.Contains(pass1, ".Clone(") || strings.Contains(pass1, `"strings"`) {
		t.Fatalf("all clone layers and the orphaned import must disappear in pass 1:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("strconv quote clone rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixExtendedObserverCloneChainsReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import (
	"bytes"
	"net/http"
	"path"
	"strings"
)

func contentType(data []byte) string {
	return http.DetectContentType(bytes.Clone(bytes.Clone(data)))
}

func match(pattern, name string) (bool, error) {
	return path.Match(strings.Clone(pattern), strings.Clone(name))
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{"http.DetectContentType(data)", "path.Match(pattern, name)"} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected observer fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Contains(pass1, ".Clone(") || strings.Contains(pass1, `"bytes"`) || strings.Contains(pass1, `"strings"`) {
		t.Fatalf("all clone layers and orphaned imports must disappear in pass 1:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("extended observer clone rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixValidationCloneChainsReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"slices"
)

func valid(data []byte) bool {
	return json.Valid(bytes.Clone(slices.Clone(bytes.Clone(data))))
}

func check(certificate *x509.Certificate, signed, signature []byte) error {
	return certificate.CheckSignature(x509.SHA256WithRSA, bytes.Clone(signed), slices.Clone(signature))
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{"json.Valid(data)", "CheckSignature(x509.SHA256WithRSA, signed, signature)"} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected validation fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Contains(pass1, ".Clone(") || strings.Contains(pass1, `"bytes"`) || strings.Contains(pass1, `"slices"`) {
		t.Fatalf("all clone layers and orphaned imports must disappear in pass 1:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("validation clone rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixStringLookupCloneChainsReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import (
	"net/http"
	"net/url"
	"path"
	"strings"
)

func observe(header http.Header, values url.Values, key, name string) bool {
	return header.Get(strings.Clone(key)) != values.Get(strings.Clone(key)) || path.IsAbs(strings.Clone(strings.Clone(name)))
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{"header.Get(key)", "values.Get(key)", "path.IsAbs(name)"} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected string lookup fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Contains(pass1, ".Clone(") || strings.Contains(pass1, `"strings"`) {
		t.Fatalf("all string clone layers and the orphaned import must disappear in pass 1:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("string lookup clone rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixIndexLookupCloneChainsReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import "strings"

func observe(values map[string]int, key, text string, index int) (int, byte) {
	value := values[strings.Clone(strings.Clone(key))]
	delete(values, strings.Clone(key))
	return value, strings.Clone(strings.Clone(text))[index]
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{"values[key]", "delete(values, key)", "return value, text[index]"} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected index/lookup fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Contains(pass1, ".Clone(") || strings.Contains(pass1, `"strings"`) {
		t.Fatalf("all clone layers and the orphaned import must disappear in pass 1:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("index/lookup clone rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixComparisonCloneChainsReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import (
	"bytes"
	"maps"
	"slices"
	"strings"
)

func compare(left, right string, data []byte, index map[string]int) (bool, bool, bool) {
	stringsEqual := strings.Clone(strings.Clone(left)) == strings.Clone(right)
	bytesNil := bytes.Clone(slices.Clone(bytes.Clone(data))) == nil
	mapsPresent := maps.Clone(maps.Clone(index)) != nil
	return stringsEqual, bytesNil, mapsPresent
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{"stringsEqual := left == right", "bytesNil := data == nil", "mapsPresent := index != nil"} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected comparison fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Contains(pass1, ".Clone(") || strings.Contains(pass1, `import`) {
		t.Fatalf("all clone layers and orphaned imports must disappear in pass 1:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("comparison clone rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixEphemeralSizeChainsReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import (
	"bytes"
	"slices"
	"strings"
)

func sizes(data []byte, text string) (int, int64) {
	n := bytes.NewReader(bytes.Clone(slices.Clone(data))).Len()
	size := bytes.NewReader(bytes.Clone([]byte(strings.Clone(strings.Clone(text))))).Size()
	return n, size
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{"n := len(data)", "size := int64(len(text))"} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected ephemeral size fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Contains(pass1, ".Clone(") || strings.Contains(pass1, "NewReader") || strings.Contains(pass1, "import") {
		t.Fatalf("constructor/clone scaffolding and orphaned imports must disappear in pass 1:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("ephemeral size rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixEphemeralBufferExtractionChainsReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import (
	"bytes"
	"slices"
	"strings"
)

func extract(data []byte, text string) (string, []byte, int, string) {
	encoded := bytes.NewBuffer(bytes.Clone(slices.Clone(data))).String()
	snapshot := bytes.NewBufferString(strings.Clone(text)).Bytes()
	length := len(bytes.NewBuffer(bytes.Clone(data)).Bytes())
	empty := bytes.NewBuffer(nil).String()
	return encoded, snapshot, length, empty
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{"encoded := string(data)", "snapshot := []byte(text)", "length := len(data)", "empty := string([]byte(nil))"} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected ephemeral buffer extraction fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Contains(pass1, ".Clone(") || strings.Contains(pass1, "NewBuffer") || strings.Contains(pass1, "import") {
		t.Fatalf("buffer/clone scaffolding and orphaned imports must disappear in pass 1:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("ephemeral buffer extraction rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixNopCloserTerminalChainsReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import "io"

func read(reader io.Reader, buffer []byte) (int, error) {
	return io.NopCloser(io.NopCloser(io.NopCloser(reader))).Read(buffer)
}

func close(reader io.Reader) error {
	return io.NopCloser(io.NopCloser(io.NopCloser(reader))).Close()
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{"return (reader).Read(buffer)", "return io.NopCloser(reader).Close()"} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected terminal NopCloser fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Count(pass1, "NopCloser") != 1 {
		t.Fatalf("Read wrappers and redundant Close wrappers must disappear in pass 1:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("terminal NopCloser rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixLimitReaderTerminalChainsReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import "io"

func read(reader io.Reader, buffer []byte, inner, middle, outer int64) (int, error) {
	return io.LimitReader(io.LimitReader(io.LimitReader(reader, inner), middle), outer).Read(buffer)
}
`
	pass1 := string(runFixMode(t, source))
	want := "return io.LimitReader(reader, min(inner, middle, outer)).Read(buffer)"
	if !strings.Contains(pass1, want) {
		t.Fatalf("expected terminal LimitReader fixed point %q in pass 1:\n%s", want, pass1)
	}
	if strings.Count(pass1, "LimitReader") != 1 {
		t.Fatalf("nested LimitReader layers must collapse to one in pass 1:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("terminal LimitReader rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixBufioConstructorChainsReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import (
	"bufio"
	"io"
)

func reader(source io.Reader) *bufio.Reader {
	return bufio.NewReader(bufio.NewReader(bufio.NewReader(source)))
}

func writer(destination io.Writer) *bufio.Writer {
	return bufio.NewWriterSize(bufio.NewWriterSize(bufio.NewWriterSize(destination, 8192), 4096), 1024)
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{
		"return bufio.NewReader(source)",
		"return bufio.NewWriterSize(destination, 8192)",
	} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected bufio constructor fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Count(pass1, "NewReader") != 1 || strings.Count(pass1, "NewWriterSize") != 1 {
		t.Fatalf("redundant bufio constructor layers must disappear in pass 1:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("bufio constructor rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixTerminalIOMultiTreesReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import "io"

func read(a, b, c, d io.Reader, buffer []byte) (int, error) {
	return io.MultiReader(io.MultiReader(a, b), io.MultiReader(c, d)).Read(buffer)
}

func write(a, b, c, d io.Writer, payload []byte) (int, error) {
	return io.MultiWriter(io.MultiWriter(a, io.MultiWriter(b, c)), d).Write(payload)
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{
		"return io.MultiReader(a, b, c, d).Read(buffer)",
		"return io.MultiWriter(a, b, c, d).Write(payload)",
	} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected terminal io multi-tree fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Count(pass1, "MultiReader") != 1 || strings.Count(pass1, "MultiWriter") != 1 {
		t.Fatalf("nested io multi-adapter constructors must disappear in pass 1:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("terminal io multi-tree rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixIOMultiTreesInConsumersReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import "io"

func read(a, b, c, d io.Reader) ([]byte, error) {
	return io.ReadAll(io.MultiReader(io.MultiReader(a, b), io.MultiReader(c, d)))
}

func write(a, b, c, d io.Writer, text string) (int, error) {
	return io.WriteString(io.MultiWriter(io.MultiWriter(a, io.MultiWriter(b, c)), d), text)
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{
		"return io.ReadAll(io.MultiReader(a, b, c, d))",
		"return io.WriteString(io.MultiWriter(a, b, c, d), text)",
	} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected io consumer multi-tree fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Count(pass1, "MultiReader") != 1 || strings.Count(pass1, "MultiWriter") != 1 {
		t.Fatalf("nested io multi-adapter constructors must disappear in one consumer pass:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("io consumer multi-tree rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixMultiReaderCopySourcesReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import "io"

func copyTo(destination io.Writer, a, b, c, d io.Reader) (int64, error) {
	return io.Copy(destination, io.MultiReader(io.MultiReader(a, b), io.MultiReader(c, d)))
}

func copyBuffer(destination io.Writer, a, b, c io.Reader, buffer []byte) (int64, error) {
	return io.CopyBuffer(destination, io.MultiReader(a, io.MultiReader(b, c)), buffer)
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{
		"return io.Copy(destination, io.MultiReader(a, b, c, d))",
		"return io.CopyBuffer(destination, io.MultiReader(a, b, c), buffer)",
	} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected MultiReader Copy source fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Count(pass1, "MultiReader") != 2 {
		t.Fatalf("nested MultiReader Copy source constructors must disappear in one pass:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("MultiReader Copy source rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixErrorsJoinTreesInIsReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import "errors"

func match(a, b, c, d, target error) bool {
	return errors.Is(errors.Join(errors.Join(a, b), errors.Join(c, d)), target)
}
`
	pass1 := string(runFixMode(t, source))
	const want = "return errors.Is(errors.Join(a, b, c, d), target)"
	if !strings.Contains(pass1, want) {
		t.Errorf("expected errors.Join tree fixed point %q in pass 1:\n%s", want, pass1)
	}
	if strings.Count(pass1, "errors.Join") != 1 {
		t.Fatalf("nested errors.Join constructors must disappear in one pass:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("errors.Join tree rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixNestedRepeatCountsReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import (
	"bytes"
	"slices"
	"strings"
)

func repeatBytes(data []byte) []byte {
	return bytes.Repeat(bytes.Repeat(data, 2), 3)
}

func repeatStrings(text string) string {
	return strings.Repeat(strings.Repeat(strings.Repeat(text, 2), 3), 4)
}

func repeatSlices(values []int) []int {
	return slices.Repeat[[]int](slices.Repeat[[]int](values, 5), 6)
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{
		"return bytes.Repeat(data, 6)",
		"return strings.Repeat(text, 24)",
		"return slices.Repeat[[]int](values, 30)",
	} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected Repeat fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	for _, call := range []string{"bytes.Repeat", "strings.Repeat", "slices.Repeat"} {
		if strings.Count(pass1, call) != 1 {
			t.Fatalf("nested %s calls must collapse in one pass:\n%s", call, pass1)
		}
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("nested Repeat rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixPathJoinPrefixSpineReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import "path"

func left(a, b, c, d string) string {
	return path.Join(path.Join(path.Join(a, b), c), d)
}

func right(a, b, c string) string {
	return path.Join(a, path.Join(b, c))
}
`
	pass1 := string(runFixMode(t, source))
	if want := "return path.Join(a, b, c, d)"; !strings.Contains(pass1, want) {
		t.Errorf("expected path.Join prefix fixed point %q in pass 1:\n%s", want, pass1)
	}
	if want := "return path.Join(a, path.Join(b, c))"; !strings.Contains(pass1, want) {
		t.Errorf("unsafe right-nested path.Join must remain %q:\n%s", want, pass1)
	}
	if strings.Count(pass1, "path.Join") != 3 {
		t.Fatalf("only the two safe left-spine path.Join layers should disappear:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("path.Join prefix-spine rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixSlicesConcatTreeReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import "slices"

func concat(a, b, c, d []int) []int {
	return slices.Concat(slices.Concat(a, b), slices.Concat(c, d))
}
`
	pass1 := string(runFixMode(t, source))
	if want := "return slices.Concat(a, b, c, d)"; !strings.Contains(pass1, want) {
		t.Errorf("expected slices.Concat tree fixed point %q in pass 1:\n%s", want, pass1)
	}
	if strings.Count(pass1, "slices.Concat") != 1 {
		t.Fatalf("nested slices.Concat calls must disappear in one pass:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("slices.Concat tree rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixCleanCanonicalProducerReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import (
	"path"
	"path/filepath"
)

func directory(name string) string {
	return path.Clean(path.Clean(path.Clean(path.Dir(name))))
}

func base(name string) string {
	return filepath.Clean(filepath.Base(name))
}

func knownJoin(root string) string {
	return path.Clean(path.Join(root, "fixed"))
}

func dynamicJoin(root string) string {
	return path.Clean(path.Join(root))
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{
		"return path.Dir(name)",
		"return filepath.Base(name)",
		`return path.Join(root, "fixed")`,
		"return path.Clean(path.Join(root))",
	} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected canonical-producer fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Count(pass1, "path.Clean") != 1 || strings.Count(pass1, "filepath.Clean") != 0 {
		t.Fatalf("only the unsafe dynamic-Join Clean should remain:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("canonical-producer rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixInverseSplitJoinReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import "strings"

func split(s string) string {
	return strings.Join(strings.Split(s, ","), ",")
}

func splitAfter(s string) string {
	return strings.Join(strings.SplitAfter(s, ","), "")
}

func limited(s string) string {
	return strings.Join(strings.SplitN(s, ",", 2), ",")
}
`
	pass1 := string(runFixMode(t, source))
	if strings.Count(pass1, "\treturn s\n") != 2 {
		t.Fatalf("inverse Split/Join compositions must disappear in one pass:\n%s", pass1)
	}
	if want := `return strings.Join(strings.SplitN(s, ",", 2), ",")`; !strings.Contains(pass1, want) {
		t.Errorf("limited SplitN must remain %q:\n%s", want, pass1)
	}
	if strings.Count(pass1, "strings.Join") != 1 || strings.Count(pass1, "strings.Split") != 1 {
		t.Fatalf("only the non-inverse limited composition should remain:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("inverse Split/Join rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixFilepathSlashNormalizerChainReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import "path/filepath"

func normalize(path string) string {
	return filepath.ToSlash(filepath.FromSlash(filepath.ToSlash(filepath.FromSlash(path))))
}

func single(path string) string {
	return filepath.FromSlash(path)
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{
		"return filepath.ToSlash(path)",
		"return filepath.FromSlash(path)",
	} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected filepath slash-chain fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Count(pass1, "filepath.ToSlash") != 1 || strings.Count(pass1, "filepath.FromSlash") != 1 {
		t.Fatalf("only the two outermost slash normalizers should remain:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("filepath slash-chain rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixFromSlashNativeProducerReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import "path/filepath"

func clean(name string) string {
	return filepath.FromSlash(filepath.FromSlash(filepath.Clean(name)))
}

func join(parts []string) string {
	return filepath.FromSlash(filepath.Join(parts...))
}

func mixed(name string) string {
	return filepath.FromSlash(filepath.ToSlash(filepath.FromSlash(filepath.Clean(name))))
}

func dynamic(name string) string {
	return filepath.FromSlash(name)
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{
		"return filepath.Clean(name)",
		"return filepath.Join(parts...)",
		"return filepath.FromSlash(name)",
	} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected native filepath-producer fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Count(pass1, "filepath.FromSlash") != 1 || strings.Count(pass1, "filepath.ToSlash") != 0 {
		t.Fatalf("only the dynamic FromSlash call should remain:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("native filepath-producer rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixToValidUTF8PostconditionChainReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import "strings"

func deep(payload string) string {
	return strings.ToValidUTF8(strings.ToValidUTF8(strings.ToValidUTF8(payload, "?"), "\xff"), "\xfe")
}

func partial(payload string) string {
	return strings.ToValidUTF8(strings.ToValidUTF8(strings.ToValidUTF8(payload, "\xff"), "?"), "\xfe")
}

func unsafe(payload string) string {
	return strings.ToValidUTF8(strings.ToValidUTF8(payload, "\xff"), "?")
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{
		`return strings.ToValidUTF8(payload, "?")`,
		`return strings.ToValidUTF8(strings.ToValidUTF8(payload, "\xff"), "?")`,
	} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected ToValidUTF8 postcondition fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if want := `return strings.ToValidUTF8(strings.ToValidUTF8(payload, "\xff"), "?")`; strings.Count(pass1, want) != 2 {
		t.Errorf("partial and unsafe chains should both retain %q exactly twice:\n%s", want, pass1)
	}
	if strings.Count(pass1, "strings.ToValidUTF8") != 5 {
		t.Fatalf("only the required sanitizer calls should remain in one pass:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("ToValidUTF8 postcondition rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixValidationOfSanitizedUTF8ReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import (
	"strings"
	"unicode/utf8"
)

func guaranteed(payload string) bool {
	return utf8.ValidString(strings.ToValidUTF8(strings.ToValidUTF8(payload, "\xff"), "?"))
}

func load() string { return "\xff" }

func effectful() bool {
	return utf8.ValidString(strings.ToValidUTF8(load(), "?"))
}
`
	pass1 := string(runFixMode(t, source))
	if want := "return true"; strings.Count(pass1, want) != 1 {
		t.Errorf("expected exactly one terminal UTF-8 validation fixed point %q in pass 1:\n%s", want, pass1)
	}
	if want := `return utf8.ValidString(strings.ToValidUTF8(load(), "?"))`; !strings.Contains(pass1, want) {
		t.Errorf("effectful sanitizer evaluation must remain %q:\n%s", want, pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("terminal UTF-8 validation rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixFieldsJoinCanonicalizationReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import (
	"bytes"
	"strings"
)

func text(payload string) string {
	return strings.Join(strings.Fields(strings.Join(strings.Fields(strings.Join(strings.Fields(payload), ",")), " - ")), "\t")
}

func data(payload []byte) []byte {
	return bytes.Join(bytes.Fields(bytes.Join(bytes.Fields(bytes.Join(bytes.Fields(payload), []byte(" "))), []byte(" "))), []byte(" "))
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{
		`return strings.Join(strings.Fields(payload), ",")`,
		`return bytes.Join(bytes.Fields(payload), []byte(" "))`,
	} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected Fields+Join fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Count(pass1, "strings.Join") != 1 || strings.Count(pass1, "bytes.Join") != 1 {
		t.Fatalf("only one canonicalization stage per function should remain:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("Fields+Join rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixByteEliminatingReplacementReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import "strings"

func deep(payload string) string {
	return strings.Replace(strings.ReplaceAll(strings.Replace(payload, "x", "_", -2), "x", "outer"), "x", "again", 4)
}

func partial(payload string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(payload, "x", "xx"), "x", ""), "x", "unused")
}

func noopBridge(payload string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(payload, "x", ""), "y", "y"), "x", "unused")
}

func load() string { return "unused" }

func effectful(payload string) string {
	return strings.ReplaceAll(strings.ReplaceAll(payload, "x", ""), "x", load())
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{
		`return strings.Replace(payload, "x", "_", -2)`,
		`return strings.ReplaceAll(strings.ReplaceAll(payload, "x", "xx"), "x", "")`,
		`return strings.ReplaceAll(payload, "x", "")`,
		`return strings.ReplaceAll(strings.ReplaceAll(payload, "x", ""), "x", load())`,
	} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected replacement fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("byte-eliminating replacement rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixGuardedBoundaryTrimReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import (
	"bytes"
	"strings"
)

func text(value, prefix string) string {
	if strings.HasPrefix(value, prefix) {
		value = strings.TrimPrefix(value, prefix)
	}
	return value
}

func data(value, suffix []byte) []byte {
	if bytes.HasSuffix(value, suffix) {
		return bytes.TrimSuffix(value, suffix)
	}
	return value
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{
		`if after, found := strings.CutPrefix(value, prefix); found`,
		`if after, found := bytes.CutSuffix(value, suffix); found`,
	} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected guarded boundary fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Contains(pass1, ".HasPrefix(") || strings.Contains(pass1, ".HasSuffix(") ||
		strings.Contains(pass1, ".TrimPrefix(") || strings.Contains(pass1, ".TrimSuffix(") {
		t.Fatalf("predicate+trim pairs must become one Cut call in pass 1:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("guarded boundary rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixAssignedSplitHeadsReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import "strings"

func heads(value string) (string, string) {
	all := strings.Split(value, ":")[0]
	limited := strings.SplitN(value, ":", 2)[0]
	return all, limited
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{
		`all, _, _ := strings.Cut(value, ":")`,
		`limited, _, _ := strings.Cut(value, ":")`,
	} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected assigned Split head fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Contains(pass1, ".Split(") || strings.Contains(pass1, ".SplitN(") {
		t.Fatalf("assigned Split heads must reach Cut directly in pass 1:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("assigned Split head rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixGuardedSplitPiecesReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import (
	"bytes"
	"strings"
)

func text(value string) (string, string) {
	var head string
	if strings.Contains(value, ":") {
		head = strings.SplitN(value, ":", 3)[0]
	}
	tail := value
	if strings.Contains(value, ":") {
		tail = strings.SplitN(value, ":", 2)[1]
	}
	return head, tail
}

func data(value []byte) []byte {
	if bytes.Contains(value, []byte(":")) {
		return bytes.SplitN(value, []byte{':'}, 2)[0]
	}
	return value
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{
		`if before, _, found := strings.Cut(value, ":"); found`,
		`if _, after, found := strings.Cut(value, ":"); found`,
		`if before, _, found := bytes.Cut(value, []byte(":")); found`,
		`return before[:len(before):len(before)]`,
	} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected guarded Split fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Contains(pass1, ".Contains(") || strings.Contains(pass1, ".SplitN(") {
		t.Fatalf("guarded Split consumers must become one Cut call in pass 1:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("guarded Split rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixContainsGuardedReplaceAllReachesFixedPointInOnePass(t *testing.T) {
	const source = `package p

import "strings"

func assign(value, old, replacement string) string {
	if strings.Contains(value, old) {
		value = strings.ReplaceAll(value, old, replacement)
	}
	return value
}

func early(value string) string {
	if strings.Contains(value, ":") {
		return strings.ReplaceAll(value, ":", "-")
	}
	return value
}

func branched(value string) string {
	if strings.Contains(value, ":") {
		return strings.ReplaceAll(value, ":", "-")
	} else {
		return value
	}
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{
		`value = strings.ReplaceAll(value, old, replacement)`,
		`return strings.ReplaceAll(value, ":", "-")`,
	} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected guarded ReplaceAll fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Contains(pass1, ".Contains(") || strings.Count(pass1, ".ReplaceAll(") != 3 {
		t.Fatalf("all three Contains guards must disappear and each ReplaceAll must remain in pass 1:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("guarded ReplaceAll rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixContainsGuardedIndexFamiliesComposeInOnePass(t *testing.T) {
	const source = `package p

import (
	"bytes"
	"slices"
	"strings"
)

func text(value string) int {
	if strings.Contains(value, ":") {
		return strings.Index(value, ":")
	}
	return -1
}

func data(value, needle []byte) int {
	if bytes.Contains(value, needle) {
		return bytes.Index(value, needle)
	}
	return -1
}

func any(value string) int {
	if strings.ContainsAny(value, ":") {
		return strings.IndexAny(value, ":")
	}
	return -1
}

func generic(value []byte) int {
	if slices.Contains(value, byte(':')) {
		return slices.Index(value, byte(':'))
	}
	return -1
}

var keepSlices = slices.Contains([]int{1}, 2)
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{
		`return strings.IndexByte(value, ":"[0])`,
		`return bytes.Index(value, needle)`,
		`return bytes.IndexByte(value, byte(':'))`,
	} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected guarded-index composition fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Count(pass1, ".Contains") != 1 || strings.Contains(pass1, "if ") {
		t.Fatalf("membership guards must disappear in the first fix pass:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("guarded Index family rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixContainsGuardedLastIndexFamiliesComposeInOnePass(t *testing.T) {
	const source = `package p

import (
	"bytes"
	"strings"
)

func text(value string) int {
	if strings.Contains(value, ":") {
		return strings.LastIndex(value, ":")
	}
	return -1
}

func data(value, needle []byte) int {
	if bytes.Contains(value, needle) {
		return bytes.LastIndex(value, needle)
	}
	return -1
}

func textAny(value string) int {
	if strings.ContainsAny(value, ":") {
		return strings.LastIndexAny(value, ":")
	}
	return -1
}

func dataAny(value []byte) int {
	if bytes.ContainsAny(value, ":") {
		return bytes.LastIndexAny(value, ":")
	}
	return -1
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{
		`return strings.LastIndexByte(value, ":"[0])`,
		`return bytes.LastIndex(value, needle)`,
		`return bytes.LastIndexByte(value, ":"[0])`,
	} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected guarded-LastIndex composition fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Contains(pass1, ".Contains") || strings.Contains(pass1, "if ") ||
		strings.Count(pass1, "strings.LastIndexByte(") != 2 ||
		strings.Count(pass1, "bytes.LastIndexByte(") != 1 || strings.Count(pass1, ".LastIndex(") != 1 {
		t.Fatalf("membership guards and generic one-byte backward searches must disappear in the first fix pass:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("guarded LastIndex family rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixUTF8ValidationGuardedSanitizerReachesFixedPointInOnePass(t *testing.T) {
	const source = `package p

import (
	"strings"
	"unicode/utf8"
)

func inPlace(value, replacement string) string {
	if !utf8.ValidString(value) {
		value = strings.ToValidUTF8(value, replacement)
	}
	return value
}

func returned(value string) string {
	if !utf8.ValidString(value) {
		return strings.ToValidUTF8(value, "?")
	}
	return value
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{
		`value = strings.ToValidUTF8(value, replacement)`,
		`return strings.ToValidUTF8(value, "?")`,
	} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected guarded UTF-8 sanitizer fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Contains(pass1, "ValidString") || strings.Contains(pass1, "if ") ||
		strings.Contains(pass1, "unicode/utf8") || strings.Count(pass1, "ToValidUTF8(") != 2 {
		t.Fatalf("validation guards and orphaned utf8 import must disappear in the first fix pass:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("guarded UTF-8 sanitizer rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixContainsGuardedCountFamiliesReachFixedPointInOnePass(t *testing.T) {
	const source = `package p

import (
	"bytes"
	"strings"
)

func text(value string) int {
	if strings.Contains(value, ":") {
		return strings.Count(value, ":")
	}
	return 0
}

func data(value, needle []byte) int {
	count := 0
	if bytes.Contains(value, needle) {
		count = bytes.Count(value, needle)
	}
	return count
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{
		`return strings.Count(value, ":")`,
		`count := bytes.Count(value, needle)`,
	} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected guarded-count fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Contains(pass1, ".Contains") || strings.Contains(pass1, "if ") || strings.Count(pass1, ".Count(") != 2 {
		t.Fatalf("membership guards and zero fallbacks must disappear in the first fix pass:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("guarded Count rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}

func TestFixContainsGuardedReplaceFamiliesComposeInOnePass(t *testing.T) {
	const source = `package p

import "strings"

func limited(value string) string {
	if strings.Contains(value, ":") {
		value = strings.Replace(value, ":", "-", 1)
	}
	return value
}

func all(value string) string {
	if strings.Contains(value, ":") {
		return strings.ReplaceAll(value, ":", "-")
	}
	return value
}

func zero(value string) string {
	if strings.Contains(value, ":") {
		return strings.Replace(value, ":", "-", 0)
	}
	return value
}
`
	pass1 := string(runFixMode(t, source))
	for _, want := range []string{
		`value = strings.Replace(value, ":", "-", 1)`,
		`return strings.ReplaceAll(value, ":", "-")`,
		"func zero(value string) string {\n\treturn value\n}",
	} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected guarded-Replace composition fixed point %q in pass 1:\n%s", want, pass1)
		}
	}
	if strings.Contains(pass1, ".Contains") || strings.Contains(pass1, "if ") || strings.Count(pass1, ".Replace(") != 1 {
		t.Fatalf("Contains guards and the zero-count Replace must disappear in the first fix pass:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Fatalf("guarded Replace rewrite is not idempotent:\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	assertFixedCompiles(t, []byte(pass2))
}
