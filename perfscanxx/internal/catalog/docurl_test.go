package catalog

import "testing"

// TestDocURL pins the upstream clang-tidy doc-URL construction shared by
// -explain and the SARIF helpUri: it is namespaced by check FAMILY
// (checks/<family>/<name>.html) built from the TidyName, and query-based custom
// checks (which have no upstream page) return ok=false.
func TestDocURL(t *testing.T) {
	cases := []struct {
		id      string
		wantURL string
		wantOK  bool
	}{
		{"PX1001", "https://clang.llvm.org/extra/clang-tidy/checks/performance/for-range-copy.html", true},
		{"PX3003", "https://clang.llvm.org/extra/clang-tidy/checks/performance/avoid-endl.html", true},
		{"PX3009", "https://clang.llvm.org/extra/clang-tidy/checks/readability/redundant-string-cstr.html", true},
		{"PX2101", "", false}, // custom-reserve-before-loop: query-based, no upstream page
	}
	for _, tc := range cases {
		e, ok := ByID(tc.id)
		if !ok {
			t.Fatalf("%s not in catalog", tc.id)
		}
		url, gotOK := DocURL(e)
		if gotOK != tc.wantOK || url != tc.wantURL {
			t.Errorf("DocURL(%s) = %q,%v; want %q,%v", tc.id, url, gotOK, tc.wantURL, tc.wantOK)
		}
	}

	// Defensive branch: a non-custom entry whose TidyName has no "-" (no
	// family/name split) yields no URL rather than a malformed one. No real
	// catalog entry is shaped this way, so it is exercised synthetically.
	if url, ok := DocURL(Entry{TidyName: "nohyphen"}); ok || url != "" {
		t.Errorf("DocURL(hyphen-less TidyName) = %q,%v; want \"\",false", url, ok)
	}
}
