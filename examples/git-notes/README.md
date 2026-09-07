# Store committed notes in Git

[![Git notes demo](demo.gif)](demo.tape)

Install the commit-only provider beside Changes:

```bash
nix profile install github:roshbhatia/changes#changes \
  github:roshbhatia/changes#provider-git-notes
```

Write against an exact committed comparison. Select the provider when another
writable provider is installed:

```bash
changes note add \
  --commit HEAD \
  --provider git-notes \
  --file internal/router.go \
  --line 63 \
  --origin agent \
  --author review-agent \
  --message "Keep the authorization check before dispatch"
```

The provider attaches canonical line-delimited records to the head commit under
`refs/notes/changes`. Working-tree and index reads return no Git notes. Their
writes fail with guidance to select a commit.

Normal Git fetches and pushes do not synchronize the custom ref. Share it only
with explicit Git commands:

```bash
git fetch origin refs/notes/changes:refs/notes/changes-incoming
git notes --ref=refs/notes/changes merge \
  -s cat_sort_uniq refs/notes/changes-incoming
git push origin refs/notes/changes
```

The provider never runs these commands and never changes `notes.rewriteRef`,
fetch refspecs, push refspecs, or display configuration. Git does not move notes
to rewritten commits unless you configure that policy yourself.

Inspect the local document without Changes:

```bash
git notes --ref=refs/notes/changes show HEAD
```

Replay the [VHS tape](demo.tape) from the repository root:

```bash
nix develop -c ./hack/example-demos.sh git-notes
```
