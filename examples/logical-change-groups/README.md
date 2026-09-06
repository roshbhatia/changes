# Group a diff by logical change flow

[![Logical change groups demo](demo.gif)](demo.tape)

Git can order files with an orderfile, but that still treats each path as the
unit of review. A `changes.groups` provider can instead place related hunks
under ordered, nested change groups:

```yaml
version: provider/v1
name: demo-groups
description: Group the demo by request flow
command: [./provider]
actions:
  changes.groups:
    description: Return the logical change tree
```

The provider receives the comparison patch. It returns group `id`, `title`,
`parentId`, `order`, and `anchors`. A file anchor claims the whole file. A line
or range anchor claims each matching hunk. The first ordered group wins when
anchors overlap. Changes keeps unmatched hunks under `other changes`.

This shape follows two useful precedents. [Git range-diff](https://git-scm.com/docs/git-range-diff)
preserves patch-series order, while [SARIF code flows](https://docs.oasis-open.org/sarif/sarif/v2.1.0/cos01/sarif-v2.1.0-cos01.html)
model an ordered path through code locations. The group tree combines those
ideas without changing the physical Git diff. Git's
[`diff.orderFile`](https://git-scm.com/docs/git-diff#Documentation/git-diff.txt--Oltorderfilegt)
remains a useful physical fallback.

Use `providers.group` or `--group-provider` when more than one grouping
provider is installed. Changes otherwise uses the highest-priority provider.
Grouping providers run during review, must be read-only, and can use the normal
provider cache. Logical grouping requires the `builtin` display engine because
a filter owns its output and does not expose stable hunks. Use `--no-groups` to
show the physical path tree.

The local [demo provider](provider) is a complete executable example. Replay
the [VHS tape](demo.tape) from the repository root:

```bash
nix develop -c ./hack/example-demos.sh logical-change-groups
```
