package checks

import (
	"bytes"
	"reflect"
	"testing"
)

// TestPS3101ReadOnlyFuncList pins the bit-identity contract of PS3101's
// []byte-sharing whitelist: every entry in bytesReadOnlyFuncs must be a
// vetted stdlib bytes function whose results are ONLY bool/int values —
// nothing that could alias-retain the shared hoisted buffer. Adding a name
// here without adding its vetted reference fails; adding a reference whose
// signature returns a slice, struct, or pointer (bytes.Split, bytes.Cut,
// bytes.NewReader, bytes.Replace, ...) also fails.
func TestPS3101ReadOnlyFuncList(t *testing.T) {
	vetted := map[string]any{
		"Compare":       bytes.Compare,
		"Contains":      bytes.Contains,
		"ContainsAny":   bytes.ContainsAny,
		"ContainsFunc":  bytes.ContainsFunc,
		"ContainsRune":  bytes.ContainsRune,
		"Count":         bytes.Count,
		"Equal":         bytes.Equal,
		"EqualFold":     bytes.EqualFold,
		"HasPrefix":     bytes.HasPrefix,
		"HasSuffix":     bytes.HasSuffix,
		"Index":         bytes.Index,
		"IndexAny":      bytes.IndexAny,
		"IndexByte":     bytes.IndexByte,
		"IndexFunc":     bytes.IndexFunc,
		"IndexRune":     bytes.IndexRune,
		"LastIndex":     bytes.LastIndex,
		"LastIndexAny":  bytes.LastIndexAny,
		"LastIndexByte": bytes.LastIndexByte,
		"LastIndexFunc": bytes.LastIndexFunc,
	}
	for name := range bytesReadOnlyFuncs {
		fn, ok := vetted[name]
		if !ok {
			t.Errorf("bytesReadOnlyFuncs contains %q, which is not in the vetted table: PS3101 would share one hoisted []byte across iterations with bytes.%s — vet that it neither mutates nor aliases its argument, then extend the table", name, name)
			continue
		}
		rt := reflect.TypeOf(fn)
		for i := 0; i < rt.NumOut(); i++ {
			if k := rt.Out(i).Kind(); k != reflect.Bool && k != reflect.Int {
				t.Errorf("bytes.%s result %d has kind %v: only bool/int results can guarantee the shared buffer never escapes", name, i, k)
			}
		}
	}
	for name := range vetted {
		if !bytesReadOnlyFuncs[name] {
			t.Errorf("vetted table contains %q but bytesReadOnlyFuncs does not: keep the two in sync", name)
		}
	}
}
