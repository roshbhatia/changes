# Read GitHub pull request review notes

[![GitHub pull request notes demo](demo.gif)](demo.tape)

Install the read-only GitHub provider and authenticate `gh` for the repository
host:

```bash
nix profile install github:roshbhatia/changes#changes \
  github:roshbhatia/changes#provider-github-pr
gh auth status
changes provider validate github-pr
```

Run Changes on a branch with an open pull request:

```bash
pr_number=$(gh pr view --json number --jq .number)
pr_base_ref=$(gh pr view --json baseRefName --jq .baseRefName)
pr_base_tip=$(gh pr view --json baseRefOid --jq .baseRefOid)
pr_head=$(gh pr view --json headRefOid --jq .headRefOid)
pr_url=$(gh pr view --json url --jq .url)
pr_repo_url=${pr_url%/pull/*}.git
if test "$(git rev-parse --is-shallow-repository)" = true; then
  git fetch --unshallow --no-tags "$pr_repo_url" "$pr_base_ref" "refs/pull/$pr_number/head"
else
  git fetch --no-tags "$pr_repo_url" "$pr_base_ref" "refs/pull/$pr_number/head"
fi
pr_base=$(git merge-base "$pr_base_tip" "$pr_head")
changes "$pr_base" "$pr_head"
changes note list --from "$pr_base" --to "$pr_head" --provider github-pr --json
```

The fetch makes both pull request tips available without changing a local
branch. The pull request object IDs avoid assumptions about remote names. The
merge base still matches GitHub after the base branch advances. A shallow clone
downloads its remaining history so the local merge-base calculation is valid.

The provider reads review threads, replies, resolved state, outdated state,
original commits, current lines, authors, timestamps, and URLs. It does not
comment, reply, resolve a thread, or send a notification.

GitHub can return more than 100 replies in one thread without a usable nested
cursor. The provider fails closed in that case instead of returning an
incomplete thread.

Replay the [VHS tape](demo.tape) with a deterministic GitHub fixture:

```bash
nix develop -c ./hack/example-demos.sh github-pr-notes
```
