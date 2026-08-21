package ps5082

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
	"unicode/utf8"
)

func compareDeep(a, b string) int {
	return strings.Compare(strings.Clone(strings.Clone(a)), strings.Clone(strings.Clone(b))) // want "strings.Compare scalar observation consumes 4 throwaway strings.Clone layer[(]s[)] across 2 argument[(]s[)]"
}

func containsBoth(value, substring string) bool {
	return strings.Contains(strings.Clone(value), strings.Clone(substring)) // want "strings.Contains scalar observation consumes 2 throwaway strings.Clone layer"
}

func validUTF8(value string) bool {
	return utf8.ValidString(strings.Clone(value)) // want "unicode/utf8.ValidString scalar observation consumes 1 throwaway strings.Clone layer"
}

func stringHash(seed maphash.Seed, value string) uint64 {
	return maphash.String(seed, strings.Clone(value)) // want "hash/maphash.String scalar observation consumes 1 throwaway strings.Clone layer"
}

func pathMatch(pattern, name string) (bool, error) {
	return path.Match(strings.Clone(pattern), strings.Clone(name)) // want "path.Match scalar observation consumes 2 throwaway strings.Clone layer"
}

func filepathMatch(pattern, name string) (bool, error) {
	return filepath.Match(strings.Clone(pattern), strings.Clone(name)) // want "path/filepath.Match scalar observation consumes 2 throwaway strings.Clone layer"
}

func pathIsAbs(name string) bool {
	return path.IsAbs(strings.Clone(strings.Clone(name))) // want "path.IsAbs scalar observation consumes 2 throwaway strings.Clone layer"
}

func filepathIsAbs(name string) bool {
	return filepath.IsAbs(strings.Clone(name)) // want "path/filepath.IsAbs scalar observation consumes 1 throwaway strings.Clone layer"
}

func filepathIsLocal(name string) bool {
	return filepath.IsLocal(strings.Clone(name)) // want "path/filepath.IsLocal scalar observation consumes 1 throwaway strings.Clone layer"
}

func canBackquote(value string) bool {
	return strconv.CanBackquote(strings.Clone(value)) // want "strconv.CanBackquote scalar observation consumes 1 throwaway strings.Clone layer"
}

func parseHTTPVersion(value string) (int, int, bool) {
	return http.ParseHTTPVersion(strings.Clone(value)) // want "net/http.ParseHTTPVersion scalar observation consumes 1 throwaway strings.Clone layer"
}

func typeByExtension(extension string) string {
	return mime.TypeByExtension(strings.Clone(extension)) // want "mime.TypeByExtension scalar observation consumes 1 throwaway strings.Clone layer"
}

func getenv(key string) string {
	return os.Getenv(strings.Clone(key)) // want "os.Getenv scalar observation consumes 1 throwaway strings.Clone layer"
}

func lookupEnv(key string) (string, bool) {
	return os.LookupEnv(strings.Clone(key)) // want "os.LookupEnv scalar observation consumes 1 throwaway strings.Clone layer"
}

func headerGet(header http.Header, key string) string {
	return header.Get(strings.Clone(key)) // want "net/http.Get scalar observation consumes 1 throwaway strings.Clone layer"
}

func mimeHeaderGet(header textproto.MIMEHeader, key string) string {
	return header.Get(strings.Clone(key)) // want "net/textproto.Get scalar observation consumes 1 throwaway strings.Clone layer"
}

func valuesGet(values url.Values, key string) string {
	return values.Get(strings.Clone(key)) // want "net/url.Get scalar observation consumes 1 throwaway strings.Clone layer"
}

func valuesHas(values url.Values, key string) bool {
	return values.Has(strings.Clone(key)) // want "net/url.Has scalar observation consumes 1 throwaway strings.Clone layer"
}

// Exact receiver matching must not confuse this network operation with
// http.Header.Get.
func clientGet(client *http.Client, address string) (*http.Response, error) {
	return client.Get(strings.Clone(address))
}

func commentPreserved(value string) bool {
	return utf8.ValidString(strings.Clone( /* snapshot rationale */ value)) // want "unicode/utf8.ValidString scalar observation consumes 1 throwaway strings.Clone layer"
}

// PS5106 owns this larger chain and removes Compare plus both Clone calls in
// one fixed-point rewrite. PS5082 must not emit an overlapping inner fix.
func compareToZeroFixedPoint(a, b string) bool {
	return strings.Compare(strings.Clone(a), strings.Clone(b)) == 0
}

// String-returning operations may expose the clone's retention behavior.
func trimReturnsString(value string) string {
	return strings.TrimSpace(strings.Clone(value))
}

// Readers retain their input.
func readerRetains(value string) *strings.Reader {
	return strings.NewReader(strings.Clone(value))
}

// Callback observers stay outside the allowlist.
func callbackCanObserveState(value string) bool {
	return strings.ContainsFunc(strings.Clone(value), func(rune) bool { return false })
}

func standaloneClone(value string) string {
	return strings.Clone(value)
}

type cloner struct{}

func (cloner) Clone(value string) string { return value }

func userMethod(c cloner, value string) bool {
	return utf8.ValidString(c.Clone(value))
}
