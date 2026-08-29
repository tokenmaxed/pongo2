package pongo2_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tokenmaxed/pongo2/v7"
)

func TestMacroDepthLimitCoversDefinitionsAndImports(t *testing.T) {
	t.Parallel()
	tests := map[string]fstest.MapFS{
		"definition": {
			"main.tpl": {Data: []byte(
				`{% macro rec(n) %}{% if n > 0 %}{{ rec(n-1) }}{% endif %}{% endmacro %}{{ rec(20) }}`)},
		},
		"import": {
			"main.tpl": {Data: []byte(`{% import "macros.tpl" rec %}{{ rec(20) }}`)},
			"macros.tpl": {Data: []byte(
				`{% macro rec(n) export %}{% if n > 0 %}{{ rec(n-1) }}{% endif %}{% endmacro %}`)},
		},
	}
	for name, files := range tests {
		t.Run(name, func(t *testing.T) {
			set := pongo2.NewSet(name, pongo2.NewFSLoader(files))
			set.MacroDepthLimit = 4
			tpl, err := set.FromFile("main.tpl")
			if err != nil {
				t.Fatalf("FromFile: %v", err)
			}
			_, err = tpl.Execute(nil)
			if err == nil || !strings.Contains(err.Error(),
				"maximum recursive macro call depth reached (max is 4)") {
				t.Fatalf("Execute error = %v, want macro depth limit", err)
			}
		})
	}
}

func TestExportedMacroNamesAreSortedAndIndependent(t *testing.T) {
	t.Parallel()
	set := pongo2.NewSet("exports", &pongo2.DummyLoader{})
	tpl, err := set.FromString(
		`{% macro zebra() export %}z{% endmacro %}` +
			`{% macro alpha() export %}a{% endmacro %}`)
	if err != nil {
		t.Fatalf("FromString: %v", err)
	}
	names := tpl.ExportedMacroNames()
	if got := strings.Join(names, ","); got != "alpha,zebra" {
		t.Fatalf("ExportedMacroNames = %q", got)
	}
	names[0] = "changed"
	if got := strings.Join(tpl.ExportedMacroNames(), ","); got != "alpha,zebra" {
		t.Fatalf("second ExportedMacroNames = %q after caller mutation", got)
	}
}
