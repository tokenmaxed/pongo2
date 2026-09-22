package pongo2

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

var countSeeds = []string{
	"", "plain text", "α\nβ\xff", "{# a #}{# b #}X", "{# unclosed", "{# newline\n#}",
	`{% verbatim %}{{ raw }}{# literal #}{% endverbatim %}{# gone #}`,
	`{% verbatim %}{% endverbatim %}{% verbatim %}b{% endverbatim %}`,
	`{% verbatim %}unclosed`, `{{- "x\n\t\r\\\"\'" -}}`, `{{ "\q" }}`,
	"{{ \"line\nbreak\" }}", `{{ 'unterminated`, `{{ 123abc _abc123 123 123.4 }}`,
	`{{ == >= <= && || != <> ( ) + - * / ^ , . ! | : = % [ ] }}`,
	"{{\n}}", "{{ a\f{% x %}", "{{", "{%-", "{{{% endcomment %}}}",
	"{{ \"\xff\xc0\x80α\xf0\x9f\" }}", "\x00\x7f\x80\xc0\xe0\xf0\xff",
	`{% comment %}{# {% endcomment %} #}{{{% endcomment %}}}`,
}

func TestLexerDecoderMatchesUTF8(t *testing.T) {
	var source strings.Builder
	for value := range 256 {
		source.WriteByte(byte(value))
	}
	source.WriteString("α世界😀\xff\xc0\x80\xe0\x80\x80\xf4\x90\x80\x80\xf0\x9f")
	input := source.String()
	// Include every byte offset, including inside multibyte runes, and every
	// truncated suffix. The shared cursor must retain the original decoder's
	// treatment of invalid UTF-8 as well as ordinary ASCII.
	for end := 1; end <= len(input); end++ {
		for pos := 0; pos < end; pos++ {
			l := lexer{input: input[:end], pos: pos}
			got, width := l.decodeRune()
			want, wantWidth := utf8.DecodeRuneInString(input[pos:end])
			if got != want || width != wantWidth {
				t.Fatalf("decode [%d:%d] = (%U,%d), want (%U,%d)", pos, end, got, width, want, wantWidth)
			}
		}
	}
}

func sameLexError(t *testing.T, got, want error) {
	t.Helper()
	if got == nil || want == nil {
		if got != nil || want != nil {
			t.Fatalf("count error = %v, Lex error = %v", got, want)
		}
		return
	}
	g, gok := got.(*Error)
	w, wok := want.(*Error)
	if !gok || !wok || g.Filename != w.Filename || g.Line != w.Line ||
		g.Column != w.Column || g.Sender != w.Sender || g.OrigError.Error() != w.OrigError.Error() ||
		g.Token != w.Token || got.Error() != want.Error() {
		t.Fatalf("count error = %#v (%v), Lex error = %#v (%v)", got, got, want, want)
	}
}

func countParity(t *testing.T, name, source string) {
	t.Helper()
	tokens, lexErr := Lex(name, source)
	count, countErr := CountTokens(name, source, nil)
	sameLexError(t, countErr, lexErr)
	if lexErr == nil && count != len(tokens) {
		t.Fatalf("count = %d, Lex = %d tokens", count, len(tokens))
	}
	var bytes, counted int
	bounded, boundedErr := CountTokens(name, source, func(b, n int) error {
		if b < 0 || n < 0 || n > 1 || b > len("{% endverbatim %}") {
			t.Fatalf("invalid callback deltas (%d, %d)", b, n)
		}
		bytes += b
		counted += n
		return nil
	})
	sameLexError(t, boundedErr, lexErr)
	if bounded != count || counted != count || bytes > len(source) ||
		(lexErr == nil && bytes != len(source)) {
		t.Fatalf("count=%d bounded=%d deltas=(%d bytes,%d tokens), source=%d", count, bounded, bytes, counted, len(source))
	}
}

func TestCountTokensDifferential(t *testing.T) {
	for i, source := range countSeeds {
		t.Run(fmt.Sprint(i), func(t *testing.T) { countParity(t, "fixture", source) })
	}
	if err := filepath.WalkDir("template_tests", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || strings.HasSuffix(name, ".out") {
			return nil
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		t.Run(name, func(t *testing.T) { countParity(t, name, string(data)) })
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// This hash captures every token field and lexical error for the existing
// checked-in template corpus, from v7.0.0-baseml.5 at
// 5a02867cd320f2009b4e304f3f4effb3b9c78c1e. It guards ordinary Lex semantics
// independently of the CountTokens/Lex comparison, which shares the lexer.
// Deliberate corpus or lexer changes require regenerating and reviewing it.
func TestLexPinnedCorpusFingerprint(t *testing.T) {
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	if err := filepath.WalkDir("template_tests", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || strings.HasSuffix(name, ".out") {
			return nil
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		tokens, lexErr := Lex(filepath.ToSlash(name), string(data))
		errorText := ""
		if lexErr != nil {
			errorText = lexErr.Error()
		}
		return encoder.Encode(struct {
			Name   string
			Tokens []*Token
			Error  string
		}{filepath.ToSlash(name), tokens, errorText})
	}); err != nil {
		t.Fatal(err)
	}
	const want = "ab0f3dfd8f77acef9bffed8f35de058b79dfacecf87a005f8a1255c44b5bbe1f"
	if got := fmt.Sprintf("%x", hash.Sum(nil)); got != want {
		t.Fatalf("Lex corpus fingerprint = %s, want %s", got, want)
	}
}

func TestCountTokensStopsAtEveryReservation(t *testing.T) {
	stop := errors.New("reservation stopped")
	for i, source := range countSeeds {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			calls := 0
			_, _ = CountTokens("stops", source, func(int, int) error { calls++; return nil })
			for at := 1; at <= calls; at++ {
				seen, accepted := 0, 0
				count, err := CountTokens("stops", source, func(_, tokens int) error {
					seen++
					if seen >= at {
						return stop
					}
					accepted += tokens
					return nil
				})
				if err != stop || seen != at || count != accepted {
					t.Fatalf("stop %d: count=%d accepted=%d calls=%d error=%v", at, count, accepted, seen, err)
				}
			}
		})
	}
}

func TestCountTokensExactBudget(t *testing.T) {
	stop := errors.New("work limit")
	for i, source := range countSeeds {
		tokens, err := Lex("budget", source)
		if err != nil {
			continue
		}
		cost := len(source) + 512*len(tokens)
		for _, limit := range []int{cost, cost - 1} {
			if limit < 0 {
				continue
			}
			t.Run(fmt.Sprintf("%d/%d", i, limit), func(t *testing.T) {
				remaining := limit
				count, err := CountTokens("budget", source, func(b, n int) error {
					charge := b + 512*n
					if charge > remaining {
						return stop
					}
					remaining -= charge
					return nil
				})
				if limit == cost {
					if err != nil || count != len(tokens) || remaining != 0 {
						t.Fatalf("exact: count=%d remaining=%d error=%v", count, remaining, err)
					}
				} else if err != stop {
					t.Fatalf("one short error=%v, want original limit", err)
				}
			})
		}
	}
}

func longCountSource(shape string, size int) string {
	switch shape {
	case "comment":
		return "{#" + strings.Repeat("x", size) + "#}"
	case "verbatim":
		return "{% verbatim %}" + strings.Repeat("x", size) + "{% endverbatim %}"
	case "identifier":
		return "{{ " + strings.Repeat("x", size) + " }}"
	case "escaped string":
		return `{{ "` + strings.Repeat(`\n`, size/2) + `" }}`
	case "dense":
		return strings.Repeat(`{{ a }}`, size/7)
	default:
		return strings.Repeat("x", size)
	}
}

func TestCountTokensEarlyStopAndCancellation(t *testing.T) {
	stop := errors.New("work stopped")
	for _, shape := range []string{"text", "comment", "verbatim", "identifier", "escaped string", "dense"} {
		t.Run(shape, func(t *testing.T) {
			source := longCountSource(shape, 1<<20)
			for _, cancel := range []bool{false, true} {
				ctx, cancelContext := context.WithCancel(context.Background())
				seen, calls := 0, 0
				_, err := CountTokens(shape, source, func(b, _ int) error {
					calls++
					if seen+b > 256 {
						if cancel {
							cancelContext()
							return ctx.Err()
						}
						return stop
					}
					seen += b
					return ctx.Err()
				})
				cancelContext()
				want := stop
				if cancel {
					want = context.Canceled
				}
				if err != want || seen > 256 || calls > 520 {
					t.Fatalf("cancel=%t error=%v scanned=%d calls=%d", cancel, err, seen, calls)
				}
			}
		})
	}
}

func TestCountTokensAllocationBound(t *testing.T) {
	stop := errors.New("work stopped")
	for _, shape := range []string{"text", "comment", "verbatim", "identifier", "escaped string", "dense"} {
		for _, size := range []int{1024, 1 << 20} {
			t.Run(fmt.Sprintf("%s/%d", shape, size), func(t *testing.T) {
				source := longCountSource(shape, size)
				for _, bounded := range []bool{false, true} {
					count := func() {
						var reserve func(int, int) error
						if bounded {
							remaining := 256
							reserve = func(b, n int) error {
								charge := b + 512*n
								if charge > remaining {
									return stop
								}
								remaining -= charge
								return nil
							}
						}
						_, err := CountTokens(shape, source, reserve)
						if (!bounded && err != nil) || (bounded && err != stop) {
							t.Fatalf("bounded=%t error=%v", bounded, err)
						}
					}
					// The caller's closure itself may escape, but no allocation
					// count or byte size may depend on scanned/rejected input.
					allocs := testing.AllocsPerRun(3, count)
					if allocs > 4 {
						t.Fatalf("bounded=%t allocations=%g, want constant <=4", bounded, allocs)
					}
					runtime.GC()
					var before, after runtime.MemStats
					runtime.ReadMemStats(&before)
					for range 3 {
						count()
					}
					runtime.ReadMemStats(&after)
					bytes := (after.TotalAlloc - before.TotalAlloc) / 3
					t.Logf("bounded=%t allocations=%g cumulative bytes/scan=%d", bounded, allocs, bytes)
					if bytes > 4096 {
						t.Fatalf("bounded=%t bytes/scan=%d, want bounded setup only", bounded, bytes)
					}
				}
			})
		}
	}
}

func TestCountTokensConcurrent(t *testing.T) {
	var workers sync.WaitGroup
	for range 16 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i, source := range countSeeds {
				countParity(t, fmt.Sprint(i), source)
			}
		}()
	}
	workers.Wait()
}

func FuzzCountTokens(f *testing.F) {
	for _, source := range countSeeds {
		f.Add(source, uint16(128))
	}
	f.Fuzz(func(t *testing.T, source string, budget uint16) {
		countParity(t, "fuzz", source)
		remaining := int(budget)
		stop := errors.New("fuzz limit")
		stopped := false
		_, err := CountTokens("fuzz", source, func(b, n int) error {
			if stopped {
				t.Fatal("callback after failure")
			}
			charge := b + 512*n
			if charge > remaining {
				stopped = true
				return stop
			}
			remaining -= charge
			return nil
		})
		if stopped && err != stop {
			t.Fatalf("lost callback error: %v", err)
		}
	})
}

func BenchmarkCountTokens(b *testing.B) {
	for _, shape := range []string{"text", "comment", "verbatim", "identifier", "escaped string", "dense"} {
		source := longCountSource(shape, 4096)
		b.Run(shape, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			for b.Loop() {
				if _, err := CountTokens("benchmark", source, func(int, int) error { return nil }); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkLexerAdmissionShapes(b *testing.B) {
	for _, shape := range []string{"text", "comment", "verbatim", "identifier", "escaped string", "dense"} {
		source := longCountSource(shape, 4096)
		b.Run(shape, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			for b.Loop() {
				if _, err := Lex("benchmark", source); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
