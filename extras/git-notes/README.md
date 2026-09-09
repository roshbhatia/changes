# git-notes

Store token parser review notes in Git.

## Install

```sh
brew install roshbhatia/tap/changes-provider-git-notes
nix profile add 'github:roshbhatia/changes#provider-git-notes'
```

Install the core utility separately, or select its all-provider bundle. Runtime tools still need their own credentials.

## Demo

![Store token parser review notes in Git](demo.gif)

[Tape source](demo.tape) · [Task script](demo.sh)

Run `nix develop -c bash extras/git-notes/demo.sh` to run the task without recording.
Run `nix develop -c python3 hack/extra-demos.py git-notes` to record it.
