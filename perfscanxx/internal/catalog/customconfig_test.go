package catalog

import "strings"

import "testing"

import "gopkg.in/yaml.v3"

// TestYamlQuote pins the double-quoted YAML escaping used for a custom check's
// Message. The escape order matters: backslashes must be doubled BEFORE quotes
// are escaped, or the backslash inserted for a quote would itself be doubled.
// Apostrophes are legal unescaped inside a double-quoted scalar and must be left
// alone. Untested before: no catalog Message currently contains a special char,
// so a naive rewrite of yamlQuote would silently ship a config-breaking bug the
// day a message gains a `"` or `\`.
func TestYamlQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{`plain`, `"plain"`},
		{`it's fine`, `"it's fine"`},   // apostrophe untouched
		{`a "quote"`, `"a \"quote\""`}, // quotes escaped
		{`a\back`, `"a\\back"`},        // backslash doubled
		{`\"`, `"\\\""`},               // backslash THEN quote: \\ + \"
	}
	for _, c := range cases {
		if got := yamlQuote(c.in); got != c.want {
			t.Errorf("yamlQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestClangTidyConfigIsValidYAML pins that ClangTidyConfig emits a config that a
// real YAML parser accepts and whose fields survive a round-trip — including a
// Message carrying the characters that most easily break hand-built YAML (a
// double quote, a backslash, an apostrophe, a colon) and a multi-line Query (the
// `Query: |` block scalar). TestClangTidyConfigCustom only substring-checks a
// plain-message entry, so neither yamlQuote nor the block-scalar structure was
// ever validated as parseable.
func TestClangTidyConfigIsValidYAML(t *testing.T) {
	msg := `a "quoted" term, a back\slash, an apostrophe's tail, and a colon: here`
	e := Entry{
		ID: "PXTEST", TidyName: "custom-quote-test", Custom: true,
		Query:   "match varDecl(\n  hasName(\"x\"))",
		Bind:    "v",
		Message: msg,
	}
	cfg := ClangTidyConfig([]Entry{e})

	var parsed struct {
		Checks       string `yaml:"Checks"`
		CustomChecks []struct {
			Name       string `yaml:"Name"`
			Query      string `yaml:"Query"`
			Diagnostic []struct {
				BindName string `yaml:"BindName"`
				Message  string `yaml:"Message"`
				Level    string `yaml:"Level"`
			} `yaml:"Diagnostic"`
		} `yaml:"CustomChecks"`
	}
	if err := yaml.Unmarshal([]byte(cfg), &parsed); err != nil {
		t.Fatalf("ClangTidyConfig produced unparseable YAML: %v\n%s", err, cfg)
	}
	if len(parsed.CustomChecks) != 1 {
		t.Fatalf("want 1 CustomChecks entry, got %d:\n%s", len(parsed.CustomChecks), cfg)
	}
	cc := parsed.CustomChecks[0]
	if cc.Name != "quote-test" {
		t.Errorf("Name = %q, want quote-test", cc.Name)
	}
	if len(cc.Diagnostic) != 1 || cc.Diagnostic[0].Message != msg {
		t.Errorf("Message did not round-trip: got %q, want %q", cc.Diagnostic[0].Message, msg)
	}
	if cc.Diagnostic[0].BindName != "v" || cc.Diagnostic[0].Level != "Warning" {
		t.Errorf("BindName/Level wrong: %+v", cc.Diagnostic[0])
	}
	// The block scalar preserves the multi-line query (modulo a trailing newline).
	if strings.TrimRight(cc.Query, "\n") != e.Query {
		t.Errorf("Query did not round-trip: got %q, want %q", cc.Query, e.Query)
	}
}

func TestClangTidyConfigCustom(t *testing.T) {
	sel := Select("PX2101", LevelAggressive) // the custom reserve-before-loop
	if len(sel) != 1 || !sel[0].Custom {
		t.Fatalf("expected 1 custom entry, got %v", sel)
	}
	if !AnyCustom(sel) {
		t.Fatal("AnyCustom = false for a custom selection")
	}
	cfg := ClangTidyConfig(sel)
	for _, want := range []string{"custom-reserve-before-loop", "CustomChecks:", "Name: reserve-before-loop", "Query: |", "match cxxMemberCallExpr", "BindName: grow", "Level: Warning"} {
		if !strings.Contains(cfg, want) {
			t.Errorf("config missing %q:\n%s", want, cfg)
		}
	}
}

func TestClangTidyConfigNoCustom(t *testing.T) {
	sel := Select("PX1001", LevelIdiomatic)
	if AnyCustom(sel) {
		t.Fatal("AnyCustom = true for a builtin-only selection")
	}
	// A builtin-only selection must emit the Checks line and NOTHING else — no
	// CustomChecks section (the early-return path in ClangTidyConfig). Emitting
	// an empty CustomChecks would make clang-tidy reject the config.
	cfg := ClangTidyConfig(sel)
	if !strings.HasPrefix(cfg, "Checks: '") {
		t.Errorf("config should start with the Checks line, got:\n%s", cfg)
	}
	if strings.Contains(cfg, "CustomChecks:") {
		t.Errorf("builtin-only selection must not emit a CustomChecks section:\n%s", cfg)
	}
}
