# Review a multi-repository feature

[![Workspace review demo](demo.gif)](demo.tape)

Use a shared workspace root when one change spans an API, worker, and client.

```bash
changes --recursive \
  --root "$HOME/src/payments-rewrite" \
  --since origin/main
```

Changes keeps repository names in the tree. Each configured analysis provider
receives one repository comparison and returns data keyed by repository path.

Replay the [VHS tape](demo.tape) from the repository root:

```bash
nix develop -c ./hack/example-demos.sh workspace-review
```
