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
