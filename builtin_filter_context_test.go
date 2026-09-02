package pongo2_test

import (
	"testing"

	"github.com/tokenmaxed/pongo2/v7"
)

func TestApplyBuiltinFilterWithContext(t *testing.T) {
	t.Parallel()
	got, err := pongo2.ApplyFilterCtx(nil, "upper", pongo2.AsValue("value"), nil)
	if err != nil || got.String() != "VALUE" {
		t.Fatalf("ApplyFilterCtx = %v, %v; want VALUE", got, err)
	}
	if _, err := pongo2.ApplyFilterCtx(nil, "missing", pongo2.AsValue("value"), nil); err == nil {
		t.Fatal("ApplyFilterCtx accepted an unknown built-in filter")
	}
}
