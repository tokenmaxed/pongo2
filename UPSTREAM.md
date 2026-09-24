# Upstream relationship

This repository is the maintained pongo2 fork used by
[`github.com/tokenmaxed/baseml`](https://github.com/tokenmaxed/baseml).

The fork started from
[`flosch/pongo2@c0f82578df571c69ce46d4af5ba2f358d98c98e6`](https://github.com/flosch/pongo2/commit/c0f82578df571c69ce46d4af5ba2f358d98c98e6).
The module path is deliberately `github.com/tokenmaxed/pongo2/v7`, so a
baseml consumer receives this code without relying on a `replace` directive.

## Rebase procedure

1. Fetch `https://github.com/flosch/pongo2.git` as the `upstream` remote.
2. Create a fresh branch from the upstream commit selected for the next
   baseml release.
3. Reapply the downstream commits in the order listed below, resolving each
   conflict in its own commit.
4. Run the upstream tests, race tests, formatting check, `go vet`,
   `staticcheck`, and `govulncheck`.
5. Update the pinned upstream commit in this document and compare the complete
   fork diff before tagging a new `v7.0.0-baseml.N` prerelease.

Never merge upstream history into a released downstream series: rebasing the
small downstream commit stack keeps every divergence reviewable.

## Downstream changes

| ID | Change | Upstream status |
| --- | --- | --- |
| B0 | Rename module, add CI, update `x/text` | Fork baseline; v0.41.0 fixes reachable GO-2026-5970 in `slugify` |
| F1 | Restart lexer after comments/verbatim | [flosch/pongo2#381](https://github.com/flosch/pongo2/pull/381) |
| F2 | Export token spans and provenance | Fork hook |
| F3 | Bound macros; expose and validate exported names | Bound: [flosch/pongo2#382](https://github.com/flosch/pongo2/pull/382); exports are hooks |
| F4 | Meter work, live buffers, and macros | Fork hook |
| F5 | Resolve filters; mark values | Ban fix: [flosch/pongo2#384](https://github.com/flosch/pongo2/pull/384); hooks are policy |
| F6 | Add resolved-value/context hooks | Fork hook |
| F7 | Fix `urlize` escaping and mail passes | [flosch/pongo2#383](https://github.com/flosch/pongo2/pull/383) |
| F8 | Meter string operands before rune indexing | Fork hook |
| F9 | Use context-aware tag autoescape and parser-resolved named cycles | Fork integration |
| F10 | Preserve sequence-value safety and make `join` context-aware | Downstream bug fix |
| F11 | Preserve value-level safety across macro argument binding | Downstream bug fix |
| F12 | Count tokens with bounded source/token reservations in the same lexer | Fork hook |

## Count-only lexical admission

`CountTokens(name, source, reserve)` is an additive preflight hook for callers
that need to reserve later lexical/parser allocations. It shares the lexer
state machine used by `Lex` and template construction. It emits no tokens,
token slice, decoded strings or AST. Lexer states use method expressions so
state transitions also avoid per-token closure allocation. It has constant
auxiliary storage; only a lexical error needs its bounded error objects.

The optional callback receives incremental source-byte and token deltas before
admission. Peeking does not charge a source byte twice. Source scanning calls
it while advancing through long text, comments, verbatim regions, identifiers
and strings, even where no token has yet been emitted. It also polls once at
entry, including empty input. Rune decoding and fixed delimiter recognition
can inspect bounded lookahead before reservation. A callback's first error is
returned unchanged, so callers can enforce their own work and context policy.
Already consumed work is not refunded. Lexical errors reached within admitted
work keep the same error type, message and position as `Lex`.

The hook owns no filesystem, source snapshot, cache, compilation budget or
retention policy. It does not reserve or perform a later real parse. A nil
callback provides an unbounded count; `Lex` and parser defaults are unchanged.
The maximum token count is bounded by the source byte length: each counted
token consumes at least one source byte, including the delimiters of an empty
quoted string. Count arithmetic therefore fits the source's `int` length.

This modification is based on fork `v7.0.0-baseml.5`, commit
`5a02867cd320f2009b4e304f3f4effb3b9c78c1e`, with the original MIT license
preserved in `LICENSE`. Its lexer already includes downstream F1/F2 decisions
on restart, trim markers, source spans and verbatim provenance; none is changed.
Before upgrading the lexer, run `TestCountTokensDifferential`, the
`FuzzCountTokens` target, resource/cancellation tests and the native suite.
`TestLexPinnedCorpusFingerprint` records every original public token field and
lexical error across the checked-in template corpus at that exact revision.
Review any corpus/fingerprint change alongside the intentional lexical delta;
do not simply update the fingerprint to make a failing parity check pass.
The corpus and checks ship in the module; they require no private audit files
or network oracle.

The shared cursor decodes ASCII inline in `next` and passes every other byte
to `utf8.DecodeRuneInString`, preserving its invalid-byte behavior. Both count
and token modes use the same cursor and states. `v7.0.0-baseml.6` held the
ASCII test in a separate `decodeRune` method that exceeds the compiler's
inlining budget (cost 114, budget 80 under Go 1.27.1), so `next` called it for
every rune, ASCII included. That helper did not offset the optional-reservation
checks: under Go 1.27.1, whose inlinable `utf8.DecodeRuneInString` already
gave `v7.0.0-baseml.5` a call-free ASCII path, it made ordinary `Lex` of long
ASCII runs slower than `.5`. `v7.0.0-baseml.7` removes it; keep the ASCII test
in `next`, where it needs no call. Measured under Go 1.27.1 with interleaved
paired rounds and identical-copy controls, `.7` takes 0.77-0.91 of `.6`'s
ordinary `Lex` time on 4 KiB text, comment, verbatim, identifier and
escaped-string inputs and 0.94-0.95 on dense tags. Against `.5` it is within
5% on comments, identifiers and escaped strings, 5-12% slower on plain text
and 8-13% faster on dense tags; on verbatim input it measured 1.03-1.19 of
`.5`, whose identical builds differ there by up to 15% with code placement.
Under the Go 1.25.0 floor, `.7` is faster than both.
`TestLexerDecoderMatchesUTF8` drives `next` over byte values, offsets and
truncated inputs against the original decoder. Ordinary `Lex` benchmarks under
the fork's Go floor and the consumer's Go version accompany changes to this
hot path, separately from count-mode allocation evidence.
