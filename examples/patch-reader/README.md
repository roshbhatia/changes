# Read a selected patch with diffnav

[![Patch reader demo](demo.gif)](demo.tape)

Set the optional reader in `~/.config/changes/config.yaml`:

```yaml
interactive:
  reader: [diffnav]
```

Run `changes interactive --view commit` in a repository. Focus the Files tree
with `Ctrl-h`. Select a file or directory and press `Enter` to preview it.
Press `o` to read that scope in diffnav. Press `q` to return to Changes.
The reader receives a Git patch on stdin and uses the repository directory.

The recording reviews Changes commit `6147beb`, which adds release archive
validation. It opens the `hack/` directory, shows diffnav help, and returns
to the Changes preview. No remote service or model supplies the displayed data.

Replay the [VHS tape](demo.tape):

```sh
nix develop -c ./hack/example-demos.sh patch-reader
```

The tape uses Liga SFMono Nerd Font for diffnav icons. Set `FontFamily` to an installed Nerd Font before replaying.

The development shell provides diffnav. Other installations must provide the
reader executable separately. A missing executable appears as an error in Changes.
