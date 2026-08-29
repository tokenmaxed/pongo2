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
| B0 | Rename module, update x/text, add CI | Fork baseline |
| F1 | Restart lexer after comments/verbatim | To be filed |
| F2 | Export token spans and provenance | Fork hook |
| F3 | Bound macros; expose names | Bound to be filed; export is a hook |
| F4 | Meter work, live buffers, and macros | Fork hook |
| F5 | Resolve filters; mark values | Ban fix to file; hooks are policy |
| F6 | Add resolved-value/context hooks | Fork hook |
| F7 | Fix `urlize` escaping and mail passes | To be filed |

Replace each "To be filed" entry with its upstream pull request link when it
is opened.
