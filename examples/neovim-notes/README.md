# Add diff notes from Neovim

[![Neovim notes demo](demo.gif)](demo.tape)

Install Changes and one writable provider:

```bash
nix profile install github:roshbhatia/changes#changes \
  github:roshbhatia/changes#provider-local-notes
```

Add the separate adapter output to a Nix-managed Neovim configuration:

```nix
programs.neovim.plugins = [ inputs.changes.packages.${pkgs.system}.neovim-plugin ];
```

The plugin exposes `:ChangesNote`, `require("changes.notes").prompt(opts)`,
`require("changes.notes").add(opts, callback)`, and normal and visual
`<Plug>(changes-note)` mappings. It does not choose a user key:

```lua
vim.keymap.set({ "n", "x" }, "<leader>cn", "<Plug>(changes-note)")
```

`prompt` delegates presentation to `vim.ui.input`, so a configured input UI can
show a floating popup. Cancellation and empty input write nothing. The adapter
passes an argument list to `vim.system` and sends the note body through standard
input. It does not quote text through a shell. Save the target buffer first.
The adapter rejects unsaved content and disk changes made while the prompt is
open because Changes anchors against Git and disk. It requires an ordinary
UTF-8 file buffer without a byte-order mark and proves that its bounded content
equals the regular disk file. The adapter also passes the saved file digest to
Changes. Changes refuses the note when the selected working, index, or commit
side contains different content, or when Git would transform working content.
The byte check preserves empty, newline-terminated, missing-final-newline, and
CRLF files.

Configure committed notes when you want them in `refs/notes/changes`:

```bash
nix profile install github:roshbhatia/changes#provider-git-notes
```

```lua
require("changes.notes").setup({
  provider = "git-notes",
  commit = "HEAD",
  origin = "user",
})
```

For automation, bypass the popup and observe the result:

```lua
require("changes.notes").add({
  message = "Keep this guard next to the state transition",
  line = 48,
}, function(err, note)
  if err then
    vim.notify(err, vim.log.levels.ERROR)
    return
  end
  print(note.id)
end)
```

Replay the [VHS tape](demo.tape) from the repository root:

```bash
nix develop -c ./hack/example-demos.sh neovim-notes
```
