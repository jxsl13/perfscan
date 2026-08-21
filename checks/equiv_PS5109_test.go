package checks

import (
	"math/rand"
	"path"
	"reflect"
	"testing"
)

func TestEquiv_PS5109PrefixJoinExhaustive(t *testing.T) {
	parts := []string{
		"", ".", "..", "...", "/", "//", "///", "a", "b", "a/", "/a",
		"a/b", "a//b", "a/.", "a/..", "../a", "../../a", "/../a",
		"a/../../b", "./a", `a\b`, "\x00", "//host/share", "http:", "http://host",
	}
	for _, a := range parts {
		for _, b := range parts {
			for _, c := range parts {
				before := path.Join(path.Join(a, b), c)
				after := path.Join(a, b, c)
				if before != after {
					t.Fatalf("prefix flattening differs: a=%q b=%q c=%q before=%q after=%q", a, b, c, before, after)
				}
			}
		}
	}
}

func TestEquiv_PS5109DeepPrefixJoinRandom(t *testing.T) {
	random := rand.New(rand.NewSource(5109))
	alphabet := []byte("./abc\\:\x00")
	word := func() string {
		data := make([]byte, random.Intn(24))
		for index := range data {
			data[index] = alphabet[random.Intn(len(alphabet))]
		}
		return string(data)
	}
	for range 20_000 {
		a, b, c, d := word(), word(), word(), word()
		before := path.Join(path.Join(path.Join(a, b), c), d)
		after := path.Join(a, b, c, d)
		if before != after {
			t.Fatalf("deep prefix flattening differs: a=%q b=%q c=%q d=%q before=%q after=%q", a, b, c, d, before, after)
		}
	}
}

func TestEquiv_PS5109EvaluationOrder(t *testing.T) {
	run := func(flat bool) []string {
		var order []string
		part := func(name, value string) string {
			order = append(order, name)
			return value
		}
		if flat {
			_ = path.Join(part("a", "a"), part("b", ".."), part("c", "c"), part("d", "d"))
		} else {
			_ = path.Join(path.Join(path.Join(part("a", "a"), part("b", "..")), part("c", "c")), part("d", "d"))
		}
		return order
	}
	before, after := run(false), run(true)
	want := []string{"a", "b", "c", "d"}
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(after, want) {
		t.Fatalf("evaluation order differs: before=%v after=%v want=%v", before, after, want)
	}
}

func TestEquiv_PS5109RightNestedJoinIsDeliberatelyExcluded(t *testing.T) {
	nested := path.Join(".", path.Join("", "/../a"))
	flat := path.Join(".", "", "/../a")
	if nested == flat || nested != "a" || flat != "../a" {
		t.Fatalf("right-nesting exclusion witness lost: nested=%q flat=%q", nested, flat)
	}
}
