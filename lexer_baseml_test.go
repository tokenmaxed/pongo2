package pongo2_test

import (
	"strings"
	"testing"

	"github.com/tokenmaxed/pongo2/v7"
)

func TestLexerRestartsAfterCommentAndVerbatim(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		source string
		want   string
	}{
		"adjacent comments": {
			source: "{# a #}{# b #}X",
			want:   "X",
		},
		"verbatim after comment": {
			source: "{# a #}{% verbatim %}{{ raw }}{% endverbatim %}",
			want:   "{{ raw }}",
		},
		"adjacent verbatim regions": {
			source: "{% verbatim %}a{% endverbatim %}{% verbatim %}b{% endverbatim %}",
			want:   "ab",
		},
		"comment after verbatim": {
			source: "{% verbatim %}{# literal #}{% endverbatim %}{# drop #}X",
			want:   "{# literal #}X",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			tpl, err := pongo2.FromString(test.source)
			if err != nil {
				t.Fatalf("FromString(%q): %v", test.source, err)
			}
			got, err := tpl.Execute(nil)
			if err != nil {
				t.Fatalf("Execute(%q): %v", test.source, err)
			}
			if got != test.want {
				t.Fatalf("Execute(%q) = %q, want %q", test.source, got, test.want)
			}
		})
	}
}

func TestLexReportsSourceSpansAndVerbatimHTML(t *testing.T) {
	t.Parallel()
	const source = "α{# drop #}{% verbatim %}{{ raw }}{% endverbatim %}B{{- \"x\\n\" -}}C"
	tokens, err := pongo2.Lex("spans.tpl", source)
	if err != nil {
		t.Fatalf("Lex: %v", err)
	}
	for index, token := range tokens {
		if token.Pos < 0 || token.Pos > token.End || token.End > len(source) {
			t.Fatalf("token %d has invalid span [%d:%d] in %d bytes",
				index, token.Pos, token.End, len(source))
		}
		if index > 0 && token.Pos < tokens[index-1].End {
			t.Fatalf("token %d overlaps its predecessor: [%d:%d] after [%d:%d]",
				index, token.Pos, token.End,
				tokens[index-1].Pos, tokens[index-1].End)
		}
		if token.Verbatim && token.Typ != pongo2.TokenHTML {
			t.Fatalf("non-HTML token %d is marked verbatim: %#v", index, token)
		}
	}

	assertHTMLSpan := func(value string, verbatim bool) {
		t.Helper()
		start := strings.Index(source, value)
		if start < 0 {
			t.Fatalf("source does not contain %q", value)
		}
		for _, token := range tokens {
			if token.Typ == pongo2.TokenHTML && token.Val == value {
				if token.Pos != start || token.End != start+len(value) ||
					token.Verbatim != verbatim {
					t.Fatalf("HTML %q = span [%d:%d], verbatim %t; want [%d:%d], %t",
						value, token.Pos, token.End, token.Verbatim,
						start, start+len(value), verbatim)
				}
				return
			}
		}
		t.Fatalf("no HTML token for %q", value)
	}
	assertHTMLSpan("α", false)
	assertHTMLSpan("{{ raw }}", true)
	assertHTMLSpan("B", false)
	assertHTMLSpan("C", false)

	open := strings.Index(source, "{{-")
	close := strings.Index(source, "-}}")
	quoted := strings.Index(source, `"x\n"`)
	var sawOpen, sawString, sawClose bool
	for _, token := range tokens {
		switch {
		case token.Typ == pongo2.TokenSymbol && token.Pos == open:
			sawOpen = token.Val == "{{" && token.TrimWhitespaces &&
				token.End == open+len("{{-")
		case token.Typ == pongo2.TokenString:
			sawString = token.Val == "x\n" && token.Pos == quoted+1 &&
				token.End == quoted+len(`"x\n"`)-1
		case token.Typ == pongo2.TokenSymbol && token.Pos == close:
			sawClose = token.Val == "}}" && token.TrimWhitespaces &&
				token.End == close+len("-}}")
		}
	}
	if !sawOpen || !sawString || !sawClose {
		t.Fatalf("normalized token spans missing: open=%t string=%t close=%t\n%#v",
			sawOpen, sawString, sawClose, tokens)
	}
}

// This differential oracle pins the Django comment contract: a short
// comment produces nothing and every other fragment produces its own output.
func TestTemplateCommentSemanticsMatchDjangoOracle(t *testing.T) {
	t.Parallel()
	fragments := []struct{ source, output string }{
		{"A", "A"},
		{"{# c #}", ""},
		{"{#x#}", ""},
		{" ", " "},
		{`{{ "v" }}`, "v"},
		{"{% if true %}T{% endif %}", "T"},
		{"{% verbatim %}{{ raw }}{% endverbatim %}", "{{ raw }}"},
	}

	sequence := make([]int, 0, 3)
	cases := 0
	var walk func(int)
	walk = func(depth int) {
		if depth != 0 {
			var source, want strings.Builder
			for _, index := range sequence {
				source.WriteString(fragments[index].source)
				want.WriteString(fragments[index].output)
			}
			cases++
			tpl, err := pongo2.FromString(source.String())
			if err != nil {
				t.Fatalf("FromString(%q): %v", source.String(), err)
			}
			got, err := tpl.Execute(nil)
			if err != nil {
				t.Fatalf("Execute(%q): %v", source.String(), err)
			}
			if got != want.String() {
				t.Fatalf("%q rendered %q, Django renders %q",
					source.String(), got, want.String())
			}
		}
		if depth == cap(sequence) {
			return
		}
		for index := range fragments {
			sequence = append(sequence, index)
			walk(depth + 1)
			sequence = sequence[:len(sequence)-1]
		}
	}
	walk(0)
	wantCases := len(fragments) + len(fragments)*len(fragments) +
		len(fragments)*len(fragments)*len(fragments)
	if cases != wantCases {
		t.Fatalf("generated %d sources, want %d", cases, wantCases)
	}
}
