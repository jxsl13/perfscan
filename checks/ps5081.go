package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5081 removes bytes/slices/maps clone layers immediately consumed by a
// typed standard-library observer that cannot expose or mutate the clone.
var PS5081 = register(&lint.Check{
	ID:       "PS5081",
	Category: "alloc",
	Slug:     "clone-fed-readonly-stdlib-observer",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a read-only standard-library observer is fed throwaway clones",
		Text: `Cloning a slice or map immediately before a scalar/read-only
standard-library observation allocates and copies the whole container without
protecting any value that the observer can expose:

  bytes.Equal(bytes.Clone(a), slices.Clone(b)) -> bytes.Equal(a, b)
  utf8.Valid(bytes.Clone(data))                -> utf8.Valid(data)
  maps.Equal(maps.Clone(a), maps.Clone(b))     -> maps.Equal(a, b)
  sha256.Sum256(bytes.Clone(data))             -> sha256.Sum256(data)

This check matches the outer observer and then unwraps arbitrarily deep,
heterogeneous bytes.Clone, slices.Clone, and maps.Clone chains from every safe
argument in one fix. All callees are resolved through go/types; aliases and
explicit generic instantiations work, while shadowed helpers, methods named
Clone, dot imports, ellipsis, and type-changing wrappers do not.

The observer allowlist contains only APIs whose result cannot retain a view of
the cloned container:

  - bytes scalar predicates/search/count/compare functions;
  - slices Compare, Equal, BinarySearch, Contains, Index, IsSorted, Max, Min;
  - maps.Equal;
  - unicode/utf8 validation, rune counting, and decode observations;
  - encoding/binary Varint, Uvarint, and Size observations;
  - encoding/json validation;
  - net/http content sniffing;
  - hex/base32/base64 EncodeToString;
  - crypto fixed-size Sum functions and constant-time equality;
  - DSA, ECDSA, Ed25519, RSA, and X.509 signature verification; and
  - adler32, crc32, crc64, and maphash one-shot checksums.

Slice/map-returning APIs are excluded because removing the clone would change
aliasing. Callback-taking observers are also excluded: a callback can close
over and mutate the original container, making the clone a real snapshot.
Readers, iterators, sort/mutation functions, Cut/Split/Trim families, and
append-style encoders stay outside the rule for the same lifetime, aliasing, or
capacity reasons.

Argument evaluation is part of the snapshot contract too. A Clone in an early
argument is removed only when every later argument contains no call or channel
receive that could mutate the same backing container before the observer
starts. Typed conversions, len/cap, and recursively checked stdlib Clone calls
are known non-mutating and remain eligible. For example,
bytes.Equal(bytes.Clone(a), mutate(a)) stays untouched, while a Clone in the
last argument is safe. This left-to-right gate prevents the rewrite from
moving a snapshot past a mutation hidden in another argument.

The rewrite is BIT-IDENTICAL for race-free Go programs. Each base expression
is still evaluated once in its original argument position; only allocation and
copy scaffolding disappears. Nilness, element values, panics defined by the
observer, comparison order, decoding behavior, and checksum bytes are
unchanged. Comments keep the finding advisory. The shared multi-package fix
engine safely removes any now-unused clone imports—including adjacent specs in
one import block—and refuses cgo, commented imports, overlapping semantic
uses, or unsafe local-declaration fallout.`,
		Before: `same := bytes.Equal(
	bytes.Clone(left),
	slices.Clone(right),
)`,
		After: `same := bytes.Equal(left, right)`,
		MeasuredWin: `On Apple M2 Pro, bytes.Equal over four heterogeneous clone
layers on two equal 65,527-byte inputs measured 17115 ns/op, 262145 B/op,
4 allocs/op -> 1212 ns/op, 0 B/op, 0 allocs/op (median of five runs): 14.12x
faster and 92.9% less time, eliminating all four full-slice copies.
DetectContentType(bytes.Clone(data)) on one 65,527-byte input measured
8,203 ns/op, 65,536 B/op, 1 alloc/op versus direct observation at 963.8 ns/op,
0 B/op, 0 allocs/op: 8.51x faster and 88.3% less time.
json.Valid(bytes.Clone(data)) on valid 65,514-byte JSON measured 160,973 ns/op,
65,572 B/op, 1 alloc/op versus 132,838 ns/op, 18 B/op, 0 allocs/op: 1.21x
faster and 17.5% less time, eliminating the full-input copy.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5081",
		Doc:  "throwaway Clone call immediately feeds a read-only stdlib observer",
		Run:  runPS5081,
	},
})

type ps5081Observer struct {
	kind    typedCallKind
	indices []int
}

var ps5081Observers = map[string]map[string]ps5081Observer{
	"bytes": {
		"Compare":       {kind: typedPackageFunc, indices: []int{0, 1}},
		"Contains":      {kind: typedPackageFunc, indices: []int{0, 1}},
		"ContainsAny":   {kind: typedPackageFunc, indices: []int{0}},
		"ContainsRune":  {kind: typedPackageFunc, indices: []int{0}},
		"Count":         {kind: typedPackageFunc, indices: []int{0, 1}},
		"Equal":         {kind: typedPackageFunc, indices: []int{0, 1}},
		"EqualFold":     {kind: typedPackageFunc, indices: []int{0, 1}},
		"HasPrefix":     {kind: typedPackageFunc, indices: []int{0, 1}},
		"HasSuffix":     {kind: typedPackageFunc, indices: []int{0, 1}},
		"Index":         {kind: typedPackageFunc, indices: []int{0, 1}},
		"IndexAny":      {kind: typedPackageFunc, indices: []int{0}},
		"IndexByte":     {kind: typedPackageFunc, indices: []int{0}},
		"IndexRune":     {kind: typedPackageFunc, indices: []int{0}},
		"LastIndex":     {kind: typedPackageFunc, indices: []int{0, 1}},
		"LastIndexAny":  {kind: typedPackageFunc, indices: []int{0}},
		"LastIndexByte": {kind: typedPackageFunc, indices: []int{0}},
	},
	"slices": {
		"BinarySearch": {kind: typedPackageFunc, indices: []int{0}},
		"Compare":      {kind: typedPackageFunc, indices: []int{0, 1}},
		"Contains":     {kind: typedPackageFunc, indices: []int{0}},
		"Equal":        {kind: typedPackageFunc, indices: []int{0, 1}},
		"Index":        {kind: typedPackageFunc, indices: []int{0}},
		"IsSorted":     {kind: typedPackageFunc, indices: []int{0}},
		"Max":          {kind: typedPackageFunc, indices: []int{0}},
		"Min":          {kind: typedPackageFunc, indices: []int{0}},
	},
	"maps": {
		"Equal": {kind: typedPackageFunc, indices: []int{0, 1}},
	},
	"unicode/utf8": {
		"DecodeLastRune": {kind: typedPackageFunc, indices: []int{0}},
		"DecodeRune":     {kind: typedPackageFunc, indices: []int{0}},
		"FullRune":       {kind: typedPackageFunc, indices: []int{0}},
		"RuneCount":      {kind: typedPackageFunc, indices: []int{0}},
		"Valid":          {kind: typedPackageFunc, indices: []int{0}},
	},
	"encoding/hex": {
		"EncodeToString": {kind: typedPackageFunc, indices: []int{0}},
	},
	"encoding/base32": {
		"EncodeToString": {kind: typedMethod, indices: []int{0}},
	},
	"encoding/base64": {
		"EncodeToString": {kind: typedMethod, indices: []int{0}},
	},
	"encoding/binary": {
		"Size":    {kind: typedPackageFunc, indices: []int{0}},
		"Uvarint": {kind: typedPackageFunc, indices: []int{0}},
		"Varint":  {kind: typedPackageFunc, indices: []int{0}},
	},
	"encoding/json": {
		"Valid": {kind: typedPackageFunc, indices: []int{0}},
	},
	"net/http": {
		"DetectContentType": {kind: typedPackageFunc, indices: []int{0}},
	},
	"crypto/md5": {
		"Sum": {kind: typedPackageFunc, indices: []int{0}},
	},
	"crypto/sha1": {
		"Sum": {kind: typedPackageFunc, indices: []int{0}},
	},
	"crypto/sha256": {
		"Sum224": {kind: typedPackageFunc, indices: []int{0}},
		"Sum256": {kind: typedPackageFunc, indices: []int{0}},
	},
	"crypto/sha512": {
		"Sum384":     {kind: typedPackageFunc, indices: []int{0}},
		"Sum512":     {kind: typedPackageFunc, indices: []int{0}},
		"Sum512_224": {kind: typedPackageFunc, indices: []int{0}},
		"Sum512_256": {kind: typedPackageFunc, indices: []int{0}},
	},
	"crypto/sha3": {
		"Sum224": {kind: typedPackageFunc, indices: []int{0}},
		"Sum256": {kind: typedPackageFunc, indices: []int{0}},
		"Sum384": {kind: typedPackageFunc, indices: []int{0}},
		"Sum512": {kind: typedPackageFunc, indices: []int{0}},
	},
	"crypto/hmac": {
		"Equal": {kind: typedPackageFunc, indices: []int{0, 1}},
	},
	"crypto/subtle": {
		"ConstantTimeCompare": {kind: typedPackageFunc, indices: []int{0, 1}},
	},
	"crypto/ed25519": {
		"Verify":            {kind: typedPackageFunc, indices: []int{0, 1, 2}},
		"VerifyWithOptions": {kind: typedPackageFunc, indices: []int{0, 1, 2}},
	},
	"crypto/dsa": {
		"Verify": {kind: typedPackageFunc, indices: []int{1}},
	},
	"crypto/ecdsa": {
		"Verify":     {kind: typedPackageFunc, indices: []int{1}},
		"VerifyASN1": {kind: typedPackageFunc, indices: []int{1, 2}},
	},
	"crypto/rsa": {
		"VerifyPKCS1v15": {kind: typedPackageFunc, indices: []int{2, 3}},
		"VerifyPSS":      {kind: typedPackageFunc, indices: []int{2, 3}},
	},
	"crypto/x509": {
		"CheckSignature": {kind: typedMethod, indices: []int{1, 2}},
	},
	"hash/adler32": {
		"Checksum": {kind: typedPackageFunc, indices: []int{0}},
	},
	"hash/crc32": {
		"Checksum":     {kind: typedPackageFunc, indices: []int{0}},
		"ChecksumIEEE": {kind: typedPackageFunc, indices: []int{0}},
	},
	"hash/crc64": {
		"Checksum": {kind: typedPackageFunc, indices: []int{0}},
	},
	"hash/maphash": {
		"Bytes": {kind: typedPackageFunc, indices: []int{1}},
	},
}

func runPS5081(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			observerCall, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			fn, sig, ok := typedCallee(pass, observerCall.Fun)
			if !ok || fn.Pkg() == nil || observerCall.Ellipsis.IsValid() {
				return true
			}
			observer, ok := ps5081Observers[fn.Pkg().Path()][fn.Name()]
			if !ok || (observer.kind == typedPackageFunc) != (sig.Recv() == nil) {
				return true
			}
			var matches []typedUnaryCallChain
			totalLayers := 0
			for _, index := range observer.indices {
				if index < 0 || index >= len(observerCall.Args) {
					continue
				}
				if !cloneRemovalLaterArgumentsStable(pass, observerCall, index) {
					continue
				}
				if matched, ok := ps5081CloneChain(pass, observerCall.Args[index]); ok {
					matches = append(matches, matched)
					totalLayers += len(matched.calls)
				}
			}
			if totalLayers == 0 {
				return true
			}
			spans := make([]tokenSpan, 0, totalLayers*2)
			var paths []string
			for _, matched := range matches {
				spans = append(spans, matched.spans...)
				paths = append(paths, matched.paths...)
			}
			diagnostic := analysis.Diagnostic{
				Pos:     observerCall.Pos(),
				End:     observerCall.End(),
				Message: fmt.Sprintf("%s.%s read-only observation consumes %d throwaway clone layer(s) across %d argument(s); observe the original containers directly", fn.Pkg().Path(), fn.Name(), totalLayers, len(matches)),
			}
			if fix, ok := fixDeletedCallScaffoldingPaths(pass, file, paths, "remove clones before read-only observation", spans...); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5081CloneChain(pass *analysis.Pass, expr ast.Expr) (typedUnaryCallChain, bool) {
	return matchTypedUnaryPackageCallChain(pass, expr, isTypedContainerStdlibClone)
}
