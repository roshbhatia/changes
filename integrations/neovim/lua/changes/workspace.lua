local M = {}

local defaults = {
  command = "changes",
  layout = "unified",
  view = "working",
}

local config = vim.deepcopy(defaults)
local namespace = vim.api.nvim_create_namespace("changes-workspace-notes")

local function canonical(path)
  return vim.uv.fs_realpath(path) or vim.fs.normalize(path)
end

local function command_argv(command)
  if type(command) == "string" and command ~= "" then
    return { command }
  end
  if type(command) == "table" and #command > 0 then
    local result = {}
    for index, value in ipairs(command) do
      if type(value) ~= "string" or value == "" then
        return nil, "command entries must be non-empty strings"
      end
      result[index] = value
    end
    return result
  end
  return nil, "command must be a non-empty string or list"
end

local function repository_root(path)
  local root = vim.fs.root(path, ".git")
  if root == nil then
    return nil, "buffer is not inside a Git repository"
  end
  return canonical(root)
end

local function validate(snapshot, root)
  if type(snapshot) ~= "table" or snapshot.version ~= "changes.workspace/v1" then
    return nil, "changes returned an unsupported workspace version"
  end
  if type(snapshot.repository) ~= "table" or canonical(snapshot.repository.root or "") ~= root then
    return nil, "changes returned a workspace for another repository"
  end
  if type(snapshot.files) ~= "table" or type(snapshot.notes) ~= "table" or type(snapshot.rendered) ~= "string" then
    return nil, "changes returned an incomplete workspace"
  end
  return snapshot
end

function M.setup(options)
  config = vim.tbl_extend("force", {}, defaults, options or {})
end

function M.read(options, callback)
  options = vim.tbl_extend("force", {}, config, options or {})
  callback = callback or function() end
  local buffer = options.buf or 0
  local path = vim.api.nvim_buf_get_name(buffer)
  local root, root_error = repository_root(path ~= "" and path or vim.fn.getcwd())
  if root == nil then
    vim.schedule(function()
      callback(root_error)
    end)
    return
  end
  local argv, command_error = command_argv(options.command)
  if argv == nil then
    vim.schedule(function()
      callback(command_error)
    end)
    return
  end
  vim.list_extend(argv, { "workspace", "--view", options.view, "--layout", options.layout })
  if options.commit ~= nil and options.commit ~= "" then
    vim.list_extend(argv, { "--commit", options.commit })
  end
  if options.refresh then
    table.insert(argv, "--refresh")
  end
  if options.config ~= nil and options.config ~= "" then
    vim.list_extend(argv, { "--config", options.config })
  end
  vim.system(argv, { cwd = root, text = true }, function(result)
    vim.schedule(function()
      if result.code ~= 0 then
        callback(vim.trim(result.stderr or "") ~= "" and vim.trim(result.stderr) or "changes workspace failed")
        return
      end
      local ok, decoded = pcall(vim.json.decode, result.stdout)
      if not ok then
        callback("changes returned invalid workspace JSON")
        return
      end
      local snapshot, validation_error = validate(decoded, root)
      callback(validation_error, snapshot)
    end)
  end)
end

function M.decorate(buffer, snapshot)
  buffer = buffer or 0
  local path = canonical(vim.api.nvim_buf_get_name(buffer))
  local root = canonical(snapshot.repository.root)
  if path:sub(1, #root + 1) ~= root .. "/" then
    return nil, "buffer is outside the workspace repository"
  end
  local relative = path:sub(#root + 2)
  local notes = {}
  for _, note in ipairs(snapshot.notes) do
    notes[note.id] = note
  end
  vim.api.nvim_buf_clear_namespace(buffer, namespace, 0, -1)
  for _, file in ipairs(snapshot.files) do
    if file.path == relative then
      for _, hunk in ipairs(file.hunks or {}) do
        for _, line in ipairs(hunk.lines or {}) do
          local number = line.newLine
          if number ~= nil and number > 0 and number <= vim.api.nvim_buf_line_count(buffer) and type(line.noteIds) == "table" then
            local labels = {}
            for _, id in ipairs(line.noteIds) do
              local note = notes[id]
              if note ~= nil then
                table.insert(labels, note.author .. " via " .. note.source)
              end
            end
            if #labels > 0 then
              vim.api.nvim_buf_set_extmark(buffer, namespace, number - 1, 0, {
                virt_text = { { " ◆ " .. table.concat(labels, ", "), "DiagnosticInfo" } },
                virt_text_pos = "eol",
              })
            end
          end
        end
      end
    end
  end
  return true
end

function M.open(options)
  M.read(options, function(err, snapshot)
    if err ~= nil then
      vim.notify(err, vim.log.levels.ERROR)
      return
    end
    vim.cmd.new()
    local buffer = vim.api.nvim_get_current_buf()
    vim.api.nvim_buf_set_name(buffer, "changes://workspace")
    vim.bo[buffer].buftype = "nofile"
    vim.bo[buffer].bufhidden = "wipe"
    vim.bo[buffer].swapfile = false
    vim.api.nvim_buf_set_lines(buffer, 0, -1, false, vim.split(snapshot.rendered, "\n", { plain = true }))
    vim.bo[buffer].modifiable = false
  end)
end

return M
