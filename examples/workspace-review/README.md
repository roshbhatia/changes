# Review a multi-repository feature

Use a shared workspace root when one change spans an API, worker, and client.

```bash
export SYSINIT_WORKSPACE="$HOME/src/payments-rewrite"
changes --recursive --since origin/main
```

Changes keeps repository names in the tree. Each configured analysis provider
receives one repository comparison and returns data keyed by repository path.
