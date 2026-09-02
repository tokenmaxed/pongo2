package pongo2_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/tokenmaxed/pongo2/v7"
)

func TestFilterTagValueHooks(t *testing.T) {
	set := pongo2.NewSet("hooks", &pongo2.DummyLoader{})
	var marks []string
	set.MarkValue = func(value *pongo2.Value) *pongo2.Value {
		marks = append(marks, value.String())
		return value
	}
	var params []string
	set.FilterParamValue = func(param *pongo2.Value, literal bool) *pongo2.Value {
		params = append(params, fmt.Sprintf("%t:%s", literal, param.String()))
		return param
	}

	tpl, err := set.FromString(`{% filter default:"literal"|add:data %}x{% endfilter %}`)
	if err != nil {
		t.Fatalf("FromString: %v", err)
	}
	got, err := tpl.Execute(pongo2.Context{"data": "D"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got != "xD" {
		t.Fatalf("Execute = %q, want xD", got)
	}
	if want := []string{"x", "x", "xD"}; !slices.Equal(marks, want) {
		t.Fatalf("MarkValue calls = %q, want %q", marks, want)
	}
	if want := []string{"true:literal", "false:D"}; !slices.Equal(params, want) {
		t.Fatalf("FilterParamValue calls = %q, want %q", params, want)
	}
}

func TestFilterTagArrayParameterIsNotLiteral(t *testing.T) {
	set := pongo2.NewSet("array-provenance", &pongo2.DummyLoader{})
	if err := set.RegisterFilter("keep", func(in, _ *pongo2.Value) (*pongo2.Value, error) {
		return in, nil
	}); err != nil {
		t.Fatalf("RegisterFilter: %v", err)
	}
	var literals []bool
	set.FilterParamValue = func(param *pongo2.Value, literal bool) *pongo2.Value {
		literals = append(literals, literal)
		return param
	}

	tpl, err := set.FromString(`{% filter keep:[data]|keep:["constant"] %}x{% endfilter %}`)
	if err != nil {
		t.Fatalf("FromString: %v", err)
	}
	if _, err := tpl.Execute(pongo2.Context{"data": "D"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if want := []bool{false, false}; !slices.Equal(literals, want) {
		t.Fatalf("literal flags = %v, want %v", literals, want)
	}
}

func TestMacroResultUsesMarkValue(t *testing.T) {
	set := pongo2.NewSet("macro-hook", &pongo2.DummyLoader{})
	var marks []string
	set.MarkValue = func(value *pongo2.Value) *pongo2.Value {
		marks = append(marks, value.String())
		return value
	}
	tpl, err := set.FromString(`{% macro m() %}<b>{{ value }}</b>{% endmacro %}{{ m() }}`)
	if err != nil {
		t.Fatalf("FromString: %v", err)
	}
	got, err := tpl.Execute(pongo2.Context{"value": "<&"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := "<b>&lt;&amp;</b>"
	if got != want {
		t.Fatalf("Execute = %q, want %q", got, want)
	}
	if !slices.Equal(marks, []string{want}) {
		t.Fatalf("MarkValue calls = %q, want [%q]", marks, want)
	}
}
