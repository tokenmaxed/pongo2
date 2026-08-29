package pongo2

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

var errMeterStopped = errors.New("meter stopped execution")

type recordingMeter struct {
	iterationErr error
	resolvedErr  error
	chargeErr    error
	enterErr     error

	chargeLimit int
	enterLimit  int

	iterationCalls int
	resolvedCalls  int
	chargeCalls    int
	enterCalls     int
	leaveCalls     int

	charged      int
	peakCharged  int
	totalCharge  int
	totalRelease int
	macroDepth   int
	peakMacros   int
}

func (meter *recordingMeter) Iteration() error {
	meter.iterationCalls++
	return meter.iterationErr
}

func (meter *recordingMeter) Resolved(*Value) error {
	meter.resolvedCalls++
	return meter.resolvedErr
}

func (meter *recordingMeter) Charge(n int) error {
	meter.chargeCalls++
	if meter.chargeErr != nil && (meter.chargeLimit == 0 || n > meter.chargeLimit-meter.charged) {
		return meter.chargeErr
	}
	meter.charged += n
	meter.totalCharge += n
	if meter.charged > meter.peakCharged {
		meter.peakCharged = meter.charged
	}
	return nil
}

func (meter *recordingMeter) Release(n int) {
	meter.charged -= n
	meter.totalRelease += n
}

func (meter *recordingMeter) EnterMacro() error {
	meter.enterCalls++
	if meter.enterErr != nil && (meter.enterLimit == 0 || meter.macroDepth >= meter.enterLimit) {
		return meter.enterErr
	}
	meter.macroDepth++
	if meter.macroDepth > meter.peakMacros {
		meter.peakMacros = meter.macroDepth
	}
	return nil
}

func (meter *recordingMeter) LeaveMacro() {
	meter.leaveCalls++
	meter.macroDepth--
}

func executeWithMeter(t *testing.T, tpl *Template, context Context, meter Meter) (string, error) {
	t.Helper()
	var output bytes.Buffer
	err := tpl.ExecuteWriterUnbufferedWithOptions(context, &output, ExecutionOptions{Meter: meter})
	return output.String(), err
}

func templateFromSource(t *testing.T, source string) *Template {
	t.Helper()
	set := NewSet(t.Name(), &DummyLoader{})
	tpl, err := set.FromString(source)
	if err != nil {
		t.Fatalf("FromString: %v", err)
	}
	return tpl
}

func templateFromFiles(t *testing.T, files fstest.MapFS, name string) *Template {
	t.Helper()
	set := NewSet(t.Name(), NewFSLoader(files))
	tpl, err := set.FromFile(name)
	if err != nil {
		t.Fatalf("FromFile(%q): %v", name, err)
	}
	return tpl
}

func TestMeterErrorsAbortEveryMeteredConstruct(t *testing.T) {
	t.Run("for iteration", func(t *testing.T) {
		tpl := templateFromSource(t, `{% for item in items %}{{ item }}{% endfor %}`)
		meter := &recordingMeter{iterationErr: errMeterStopped}
		output, err := executeWithMeter(t, tpl, Context{"items": []int{1, 2}}, meter)
		if !errors.Is(err, errMeterStopped) {
			t.Fatalf("Execute error = %v, want meter error", err)
		}
		if output != "" || meter.iterationCalls != 1 {
			t.Fatalf("output = %q, iteration calls = %d; want empty output and one call", output, meter.iterationCalls)
		}
	})

	t.Run("macro entry", func(t *testing.T) {
		tpl := templateFromSource(t, `{% macro value() %}unreachable{% endmacro %}{{ value() }}`)
		meter := &recordingMeter{enterErr: errMeterStopped}
		output, err := executeWithMeter(t, tpl, nil, meter)
		if !errors.Is(err, errMeterStopped) {
			t.Fatalf("Execute error = %v, want meter error", err)
		}
		if output != "" || meter.enterCalls != 1 || meter.leaveCalls != 0 || meter.chargeCalls != 0 {
			t.Fatalf("output = %q, enter/leave/charge = %d/%d/%d", output,
				meter.enterCalls, meter.leaveCalls, meter.chargeCalls)
		}
	})

	t.Run("imported macro entry", func(t *testing.T) {
		tpl := templateFromFiles(t, fstest.MapFS{
			"main.tpl":   {Data: []byte(`{% import "macros.tpl" value %}{{ value() }}`)},
			"macros.tpl": {Data: []byte(`{% macro value() export %}unreachable{% endmacro %}`)},
		}, "main.tpl")
		meter := &recordingMeter{enterErr: errMeterStopped}
		output, err := executeWithMeter(t, tpl, nil, meter)
		if !errors.Is(err, errMeterStopped) {
			t.Fatalf("Execute error = %v, want meter error", err)
		}
		if output != "" || meter.enterCalls != 1 || meter.leaveCalls != 0 || meter.chargeCalls != 0 {
			t.Fatalf("output = %q, enter/leave/charge = %d/%d/%d", output,
				meter.enterCalls, meter.leaveCalls, meter.chargeCalls)
		}
	})

	buffered := []struct {
		name string
		tpl  func(*testing.T) *Template
	}{
		{
			name: "macro buffer",
			tpl: func(t *testing.T) *Template {
				return templateFromSource(t, `{% macro value() %}a{{ suffix }}{% endmacro %}{{ value() }}`)
			},
		},
		{
			name: "filter buffer",
			tpl: func(t *testing.T) *Template {
				return templateFromSource(t, `{% filter upper %}a{{ suffix }}{% endfilter %}`)
			},
		},
		{
			name: "spaceless buffer",
			tpl: func(t *testing.T) *Template {
				return templateFromSource(t, `{% spaceless %}a{{ suffix }}{% endspaceless %}`)
			},
		},
		{
			name: "block.Super buffer",
			tpl: func(t *testing.T) *Template {
				return templateFromFiles(t, fstest.MapFS{
					"base.tpl":  {Data: []byte(`{% block body %}a{{ suffix }}{% endblock %}`)},
					"child.tpl": {Data: []byte(`{% extends "base.tpl" %}{% block body %}{{ block.Super }}{% endblock %}`)},
				}, "child.tpl")
			},
		},
		{
			name: "ifchanged content buffer",
			tpl: func(t *testing.T) *Template {
				return templateFromSource(t, `{% ifchanged %}a{{ suffix }}{% endifchanged %}`)
			},
		},
	}
	for _, test := range buffered {
		t.Run(test.name, func(t *testing.T) {
			meter := &recordingMeter{
				chargeErr:   errMeterStopped,
				chargeLimit: 1,
			}
			output, err := executeWithMeter(t, test.tpl(t), Context{"suffix": "b"}, meter)
			if !errors.Is(err, errMeterStopped) {
				t.Fatalf("Execute error = %v, want meter error", err)
			}
			if output != "" {
				t.Fatalf("partial output = %q, want none before the construct returns", output)
			}
			if meter.chargeCalls != 2 {
				t.Fatalf("Charge calls = %d, want 2", meter.chargeCalls)
			}
			if meter.totalCharge == 0 || meter.totalRelease != meter.totalCharge || meter.charged != 0 {
				t.Fatalf("charge/release/current = %d/%d/%d, want a fully released partial buffer",
					meter.totalCharge, meter.totalRelease, meter.charged)
			}
		})
	}
}

func TestMeterIfchangedRetainedContentAccounting(t *testing.T) {
	tpl := templateFromSource(t,
		`{% for value in values %}{% ifchanged %}{{ value }}{% else %}!{% endifchanged %}{% endfor %}`)
	meter := &recordingMeter{}
	output, err := executeWithMeter(t, tpl, Context{"values": []string{"aa", "aa", "bb"}}, meter)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if output != "aa!bb" {
		t.Fatalf("output = %q, want aa!bb", output)
	}
	if meter.peakCharged != 4 {
		t.Fatalf("peak charge = %d, want old and candidate contents (4 bytes)", meter.peakCharged)
	}
	if meter.totalCharge != 6 || meter.totalRelease != 6 || meter.charged != 0 {
		t.Fatalf("charge/release/current = %d/%d/%d, want 6/6/0",
			meter.totalCharge, meter.totalRelease, meter.charged)
	}
}

func TestMeterRecursiveMacroPeakIsLinearInOutput(t *testing.T) {
	const blobBytes = 1024
	set := NewSet(t.Name(), &DummyLoader{})
	// A Meter, when installed, owns macro admission. This deliberately sets the
	// TemplateSet fallback below every tested recursion depth.
	set.MacroDepthLimit = 1
	tpl, err := set.FromString(
		`{% macro repeat(n, blob) %}{{ blob }}{% if n > 1 %}{{ repeat(n-1, blob) }}{% endif %}{% endmacro %}` +
			`{{ repeat(depth, blob) }}`)
	if err != nil {
		t.Fatalf("FromString: %v", err)
	}
	blob := strings.Repeat("x", blobBytes)
	for _, depth := range []int{1, 10, 100, 400} {
		t.Run(strconv.Itoa(depth), func(t *testing.T) {
			meter := &recordingMeter{}
			output, err := executeWithMeter(t, tpl, Context{"depth": depth, "blob": blob}, meter)
			if err != nil {
				t.Fatalf("Execute depth %d: %v", depth, err)
			}
			wantBytes := depth * blobBytes
			if len(output) != wantBytes {
				t.Fatalf("output bytes = %d, want %d", len(output), wantBytes)
			}
			if meter.peakCharged != wantBytes {
				t.Fatalf("peak charge = %d, want one output's %d bytes", meter.peakCharged, wantBytes)
			}
			if meter.charged != 0 || meter.totalRelease != meter.totalCharge {
				t.Fatalf("charge/release/current = %d/%d/%d after return",
					meter.totalCharge, meter.totalRelease, meter.charged)
			}
			if meter.enterCalls != depth || meter.leaveCalls != depth || meter.peakMacros != depth || meter.macroDepth != 0 {
				t.Fatalf("macro enter/leave/peak/current = %d/%d/%d/%d, want %d/%d/%d/0",
					meter.enterCalls, meter.leaveCalls, meter.peakMacros, meter.macroDepth,
					depth, depth, depth)
			}
		})
	}
}

func TestMeterMacroErrorReleasesSuccessfulEntries(t *testing.T) {
	tpl := templateFromSource(t,
		`{% macro recurse(n) %}{% if n > 0 %}{{ recurse(n-1) }}{% endif %}{% endmacro %}{{ recurse(20) }}`)
	meter := &recordingMeter{enterErr: errMeterStopped, enterLimit: 3}
	_, err := executeWithMeter(t, tpl, nil, meter)
	if !errors.Is(err, errMeterStopped) {
		t.Fatalf("Execute error = %v, want meter error", err)
	}
	if meter.enterCalls != 4 || meter.leaveCalls != 3 || meter.macroDepth != 0 {
		t.Fatalf("macro enter/leave/current = %d/%d/%d, want 4/3/0",
			meter.enterCalls, meter.leaveCalls, meter.macroDepth)
	}
}

func TestNilMeterIsByteIdentical(t *testing.T) {
	tpl := templateFromFiles(t, fstest.MapFS{
		"base.tpl": {Data: []byte(`base:{% block body %}<strong>{{ base }}</strong>{% endblock %}:end`)},
		"child.tpl": {Data: []byte(
			`{% extends "base.tpl" %}` +
				`{% block body %}` +
				`{% macro cell(value) %}<i>{{ value }}</i>{% endmacro %}` +
				`{% filter upper %}{% spaceless %}` +
				`<div>{{ block.Super }}{% for value in values %}` +
				`{% ifchanged %}{{ cell(value) }}{% else %}!{% endifchanged %}{% endfor %}</div>` +
				`{% endspaceless %}{% endfilter %}` +
				`{% endblock %}`)},
	}, "child.tpl")
	context := Context{"base": "b&", "values": []string{"a", "a", "<c>"}}

	var ordinary bytes.Buffer
	if err := tpl.ExecuteWriterUnbuffered(context, &ordinary); err != nil {
		t.Fatalf("ExecuteWriterUnbuffered: %v", err)
	}
	const expected = `base:<DIV><STRONG>B&AMP;</STRONG><I>A</I>!<I>&LT;C&GT;</I></DIV>:end`
	if ordinary.String() != expected {
		t.Fatalf("ordinary output = %q, want upstream bytes %q", ordinary.String(), expected)
	}
	var withOptions bytes.Buffer
	if err := tpl.ExecuteWriterUnbufferedWithOptions(context, &withOptions, ExecutionOptions{}); err != nil {
		t.Fatalf("ExecuteWriterUnbufferedWithOptions: %v", err)
	}
	if !bytes.Equal(withOptions.Bytes(), ordinary.Bytes()) {
		t.Fatalf("nil-meter output = %q, want byte-identical %q", withOptions.Bytes(), ordinary.Bytes())
	}
}

func TestNewChildExecutionContextCopiesMeter(t *testing.T) {
	tpl := templateFromSource(t, "")
	meter := &recordingMeter{}
	parent := newExecutionContextWithOptions(tpl, Context{}, ExecutionOptions{Meter: meter})
	child := NewChildExecutionContext(parent)
	if child.Meter != meter {
		t.Fatalf("child Meter = %#v, want parent's %#v", child.Meter, meter)
	}
}

func TestMeterResolvedErrorsPreserveIdentity(t *testing.T) {
	tests := map[string]string{
		"variable":   `{{ value }}`,
		"filter tag": `{% filter upper %}body{% endfilter %}`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			tpl := templateFromSource(t, source)
			meter := &recordingMeter{resolvedErr: errMeterStopped}
			_, err := executeWithMeter(t, tpl, Context{"value": "body"}, meter)
			if !errors.Is(err, errMeterStopped) {
				t.Fatalf("Execute error = %v, want errors.Is meter error", err)
			}
			if meter.resolvedCalls != 1 {
				t.Fatalf("Resolved calls = %d, want 1", meter.resolvedCalls)
			}
		})
	}
}

func TestMeterResolvedCoversLoopSubjectAndItems(t *testing.T) {
	tpl := templateFromSource(t, `{% for item in items %}{{ item }}{% endfor %}`)
	meter := &recordingMeter{}
	output, err := executeWithMeter(t, tpl, Context{"items": []string{"a", "b"}}, meter)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if output != "ab" || meter.iterationCalls != 2 || meter.resolvedCalls != 3 {
		t.Fatalf("output/iterations/resolved = %q/%d/%d, want ab/2/3",
			output, meter.iterationCalls, meter.resolvedCalls)
	}
}
