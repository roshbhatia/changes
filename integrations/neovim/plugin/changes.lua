if vim.g.loaded_changes_notes then
  return
end
vim.g.loaded_changes_notes = true

vim.api.nvim_create_user_command("ChangesNote", function(arguments)
  local options = {}
  if arguments.range > 0 then
    options.start_line = arguments.line1
    options.line = arguments.line2
  end
  require("changes.notes").prompt(options)
end, { desc = "Add a Changes diff note", range = true })

vim.api.nvim_create_user_command("ChangesWorkspace", function(arguments)
  require("changes.workspace").open({ refresh = arguments.bang })
end, { bang = true, desc = "Open the Changes workspace" })

vim.api.nvim_create_user_command("ChangesWorkspaceDecorate", function(arguments)
  require("changes.workspace").read({ refresh = arguments.bang }, function(err, snapshot)
    if err ~= nil then
      vim.notify(err, vim.log.levels.ERROR)
      return
    end
    require("changes.workspace").decorate(0, snapshot)
  end)
end, { bang = true, desc = "Decorate the current buffer with Changes notes" })

vim.keymap.set("n", "<Plug>(changes-note)", function()
  require("changes.notes").prompt()
end, { desc = "Add a Changes diff note" })

vim.keymap.set("x", "<Plug>(changes-note)", function()
  require("changes.notes").prompt({
    start_line = vim.fn.line("v"),
    line = vim.fn.line("."),
  })
end, { desc = "Add a Changes diff note" })
