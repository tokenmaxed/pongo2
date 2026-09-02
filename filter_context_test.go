package pongo2_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tokenmaxed/pongo2/v7"
)

var errContextFilter = errors.New("context filter stopped execution")

func TestContextFilterRunsInExpressionsAndFilterTags(t *testing.T) {
	t.Parallel()
	set := pongo2.NewSet("context-filter", &pongo2.DummyLoader{})
	var calls int
	err := set.RegisterFilterCtx("observe", func(ctx *pongo2.ExecutionContext, in, _ *pongo2.Value) (*pongo2.Value, error) {
		calls++
		prefix, ok := ctx.Public["prefix"].(string)
		if !ok {
			return nil, fmt.Errorf("execution context has no prefix")
		}
		return pongo2.AsValue(prefix + in.String()), nil
	})
	if err != nil {
		t.Fatalf("RegisterFilterCtx: %v", err)
	}
	tpl, err := set.FromString(`{% filter observe %}tag{% endfilter %}|{{ value|observe }}`)
	if err != nil {
		t.Fatalf("FromString: %v", err)
	}
	got, err := tpl.Execute(pongo2.Context{"prefix": "seen:", "value": "expr"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got != "seen:tag|seen:expr" || calls != 2 {
		t.Fatalf("Execute/calls = %q/%d, want %q/2", got, calls, "seen:tag|seen:expr")
	}
}

func TestContextFilterRegistrationAndReplacement(t *testing.T) {
	t.Parallel()
	set := pongo2.NewSet("context-filter-api", &pongo2.DummyLoader{})
	ctxFilter := func(_ *pongo2.ExecutionContext, in, _ *pongo2.Value) (*pongo2.Value, error) {
		return pongo2.AsValue("ctx:" + in.String()), nil
	}
	if err := set.RegisterFilterCtx("ctx", ctxFilter); err != nil {
		t.Fatalf("RegisterFilterCtx: %v", err)
	}
	if !set.FilterExists("ctx") {
		t.Fatal("FilterExists(ctx) = false")
	}
	if _, err := set.ApplyFilter("ctx", pongo2.AsValue("x"), nil); err == nil ||
		!strings.Contains(err.Error(), "requires an execution context") {
		t.Fatalf("ApplyFilter context-only error = %v", err)
	}
	got, err := set.ApplyFilterCtx(nil, "ctx", pongo2.AsValue("x"), nil)
	if err != nil || got.String() != "ctx:x" {
		t.Fatalf("ApplyFilterCtx = %v, %v; want ctx:x", got, err)
	}

	if err := set.ReplaceFilterCtx("upper", ctxFilter); err != nil {
		t.Fatalf("ReplaceFilterCtx: %v", err)
	}
	got, err = set.ApplyFilterCtx(nil, "upper", pongo2.AsValue("x"), nil)
	if err != nil || got.String() != "ctx:x" {
		t.Fatalf("replaced ApplyFilterCtx = %v, %v; want ctx:x", got, err)
	}
	if err := set.ReplaceFilter("upper", func(in, _ *pongo2.Value) (*pongo2.Value, error) {
		return pongo2.AsValue("plain:" + in.String()), nil
	}); err != nil {
		t.Fatalf("ReplaceFilter after context filter: %v", err)
	}
	got, err = set.ApplyFilter("upper", pongo2.AsValue("x"), nil)
	if err != nil || got.String() != "plain:x" {
		t.Fatalf("restored ApplyFilter = %v, %v; want plain:x", got, err)
	}
}

func TestContextFilterErrorsPreserveIdentity(t *testing.T) {
	t.Parallel()
	for name, source := range map[string]string{
		"expression": `{{ value|stop }}`,
		"filter tag": `{% filter stop %}body{% endfilter %}`,
	} {
		t.Run(name, func(t *testing.T) {
			set := pongo2.NewSet(t.Name(), &pongo2.DummyLoader{})
			if err := set.RegisterFilterCtx("stop", func(*pongo2.ExecutionContext, *pongo2.Value, *pongo2.Value) (*pongo2.Value, error) {
				return nil, errContextFilter
			}); err != nil {
				t.Fatalf("RegisterFilterCtx: %v", err)
			}
			tpl, err := set.FromString(source)
			if err != nil {
				t.Fatalf("FromString: %v", err)
			}
			_, err = tpl.Execute(pongo2.Context{"value": "body"})
			if !errors.Is(err, errContextFilter) {
				t.Fatalf("Execute error = %v, want errors.Is context filter error", err)
			}
		})
	}
}

func TestContextFilterRunsInAutoescapedTags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		source  string
		context pongo2.Context
		want    string
		calls   int
	}{
		{
			name:    "cycle string",
			source:  `{% cycle value %}`,
			context: pongo2.Context{"value": "one"},
			want:    "ctx:one",
			calls:   1,
		},
		{
			name:    "referenced cycle string",
			source:  `{% cycle value fallback as item %}{% cycle item %}`,
			context: pongo2.Context{"value": "one", "fallback": "two"},
			want:    "ctx:onectx:two",
			calls:   2,
		},
		{
			name:    "cycle number",
			source:  `{% cycle value %}`,
			context: pongo2.Context{"value": 7},
			want:    "7",
			calls:   0,
		},
		{
			name:    "firstof string",
			source:  `{% firstof missing value %}`,
			context: pongo2.Context{"value": "one"},
			want:    "ctx:one",
			calls:   1,
		},
		{
			name:    "firstof number",
			source:  `{% firstof missing value %}`,
			context: pongo2.Context{"value": 7},
			want:    "ctx:7",
			calls:   1,
		},
		{
			name:    "explicit safe bypass",
			source:  `{% firstof missing value|safe %}`,
			context: pongo2.Context{"value": "<b>safe</b>"},
			want:    "<b>safe</b>",
			calls:   0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			set := pongo2.NewSet(t.Name(), &pongo2.DummyLoader{})
			set.SetAutoescape(true)
			calls := 0
			err := set.ReplaceFilterCtx("escape", func(ctx *pongo2.ExecutionContext, in, _ *pongo2.Value) (*pongo2.Value, error) {
				calls++
				prefix, ok := ctx.Public["prefix"].(string)
				if !ok {
					return nil, fmt.Errorf("execution context has no prefix")
				}
				return pongo2.AsSafeValue(prefix + in.String()), nil
			})
			if err != nil {
				t.Fatalf("ReplaceFilterCtx: %v", err)
			}
			tpl, err := set.FromString(test.source)
			if err != nil {
				t.Fatalf("FromString: %v", err)
			}
			context := test.context
			context["prefix"] = "ctx:"
			got, err := tpl.Execute(context)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if got != test.want || calls != test.calls {
				t.Fatalf("Execute/calls = %q/%d, want %q/%d", got, calls, test.want, test.calls)
			}
		})
	}
}
