local M = {}

local defaults = {
  command = "changes",
  side = "right",
  origin = "user",
}

local max_file_bytes = 16 * 1024 * 1024

local config = vim.deepcopy(defaults)

local origins = { agent = true, external = true, user = true }
local authorities = { advisory = true, external = true, owner = true }
local states = { open = true, resolved = true }
local sides = { LEFT = true, RIGHT = true }
local targets = { commits = true, index = true, working = true }
local qualities = { exact = true, context = true, file = true, outdated = true, orphan = true }

local function merged(options)
  return vim.tbl_extend("force", {}, config, options or {})
end

local function command_argv(command)
  if type(command) == "string" and command ~= "" then
    return { command }
  end
  if type(command) == "table" and #command > 0 then
    local argv = {}
    for index, value in ipairs(command) do
      if type(value) ~= "string" or value == "" then
        return nil, "command entries must be non-empty strings"
      end
      argv[index] = value
    end
    return argv
  end
  return nil, "command must be a non-empty string or list"
end

local function valid_optional_strings(value, fields)
  for _, field in ipairs(fields) do
    if value[field] ~= nil and type(value[field]) ~= "string" then
      return false
    end
  end
  return true
end

local function valid_rfc3339(value)
  if value == nil then
    return true
  end
  if type(value) ~= "string" then
    return false
  end

  local timestamp = value
  if timestamp:sub(-1) == "Z" then
    timestamp = timestamp:sub(1, -2)
  else
    local offset_hour, offset_minute
    timestamp, _, offset_hour, offset_minute = value:match("^(.*)([+-])(%d%d):(%d%d)$")
    if timestamp == nil or tonumber(offset_hour) > 23 or tonumber(offset_minute) > 59 then
      return false
    end
  end

  local year, month, day, hour, minute, seconds = timestamp:match(
    "^(%d%d%d%d)%-(%d%d)%-(%d%d)T(%d%d):(%d%d):(.+)$"
  )
  if year == nil then
    return false
  end
  local second = seconds:match("^(%d%d)$")
  if second == nil then
    second = seconds:match("^(%d%d)%.[0-9]+$")
  end
  if second == nil then
    return false
  end

  year, month, day = tonumber(year), tonumber(month), tonumber(day)
  hour, minute, second = tonumber(hour), tonumber(minute), tonumber(second)
  if month < 1 or month > 12 or hour > 23 or minute > 59 or second > 59 then
    return false
  end
  local days = ({ 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31 })[month]
  if month == 2 and (year % 400 == 0 or (year % 4 == 0 and year % 100 ~= 0)) then
    days = 29
  end
  return day >= 1 and day <= days
end

local function valid_utf8(value)
  local index = 1
  while index <= #value do
    local first = value:byte(index)
    local length = 1
    local second_min, second_max = 0x80, 0xBF
    if first <= 0x7F then
      length = 1
    elseif first >= 0xC2 and first <= 0xDF then
      length = 2
    elseif first >= 0xE0 and first <= 0xEF then
      length = 3
      if first == 0xE0 then
        second_min = 0xA0
      elseif first == 0xED then
        second_max = 0x9F
      end
    elseif first >= 0xF0 and first <= 0xF4 then
      length = 4
      if first == 0xF0 then
        second_min = 0x90
      elseif first == 0xF4 then
        second_max = 0x8F
      end
    else
      return false
    end
    if index + length - 1 > #value then
      return false
    end
    if length > 1 then
      local second = value:byte(index + 1)
      if second < second_min or second > second_max then
        return false
      end
      for offset = 2, length - 1 do
        local continuation = value:byte(index + offset)
        if continuation < 0x80 or continuation > 0xBF then
          return false
        end
      end
    end
    index = index + length
  end
  return true
end

local function valid_path(path)
  if type(path) ~= "string" or vim.trim(path) == "" or path:sub(1, 1) == "/" then
    return false
  end
  if path == "." or path == ".." or path:sub(1, 3) == "../" or path:sub(-1) == "/" then
    return false
  end
  if path:find("//", 1, true) then
    return false
  end
  for part in path:gmatch("[^/]+") do
    if part == "." or part == ".." then
      return false
    end
  end
  return true
end

local function valid_range(value)
  if not sides[value.side] then
    return false
  end
  if value.startSide ~= nil and type(value.startSide) ~= "string" then
    return false
  end
  for _, field in ipairs({ "startLine", "line" }) do
    local line = value[field]
    if line ~= nil and (type(line) ~= "number" or line < 0 or line % 1 ~= 0) then
      return false
    end
  end
  local start_side = value.startSide or ""
  local start_line = value.startLine or 0
  local line = value.line or 0
  if line == 0 and start_line ~= 0 then
    return false
  end
  if start_line == 0 and start_side ~= "" then
    return false
  end
  if start_line > 0 and not sides[start_side] then
    return false
  end
  return line == 0 or start_side ~= value.side or start_line <= line
end

local function valid_anchor(anchor)
  return type(anchor) == "table"
    and valid_path(anchor.path)
    and targets[anchor.target] == true
    and valid_optional_strings(anchor, { "base", "head", "fingerprint", "context" })
    and valid_range(anchor)
end

local function valid_placement(placement, anchor)
  if type(placement) ~= "table" or not targets[placement.target] or not qualities[placement.quality] then
    return false
  end
  if not valid_optional_strings(placement, { "path", "side", "startSide", "base", "head", "fingerprint" }) then
    return false
  end
  for _, field in ipairs({ "startLine", "line" }) do
    local line = placement[field]
    if line ~= nil and (type(line) ~= "number" or line < 0 or line % 1 ~= 0) then
      return false
    end
  end
  if placement.quality == "orphan" then
    return true
  end
  if not valid_path(placement.path) or placement.path ~= anchor.path or not valid_range(placement) then
    return false
  end
  return placement.quality ~= "file" or (placement.line or 0) == 0
end

local function same_optional(left, right)
  return (left or "") == (right or "")
end

local function requested_target(options)
  if options.commit ~= nil and options.commit ~= "" or options.to ~= nil and options.to ~= "" then
    return "commits"
  end
  if options.staged then
    return "index"
  end
  return "working"
end

local function requested_side(value)
  if type(value) == "string" and vim.trim(value) == "" then
    return defaults.side
  end
  return value
end

local function valid_note(note, expected)
  if type(note) ~= "table" then
    return false
  end
  for _, field in ipairs({ "id", "source", "sourceId", "summary", "author" }) do
    if type(note[field]) ~= "string" or vim.trim(note[field]) == "" then
      return false
    end
  end
  if not origins[note.origin] or not authorities[note.authority] or not states[note.state] then
    return false
  end
  if not valid_optional_strings(note, {
    "threadId", "replyTo", "rationale", "session", "createdAt", "updatedAt", "url",
  }) then
    return false
  end
  if not valid_rfc3339(note.createdAt) or not valid_rfc3339(note.updatedAt) then
    return false
  end
  if not valid_anchor(note.anchor) or not valid_placement(note.placement, note.anchor) then
    return false
  end
  local anchor, placement = note.anchor, note.placement
  if not (placement.quality == "exact"
    and placement.path == anchor.path
    and placement.side == anchor.side
    and same_optional(placement.startSide, anchor.startSide)
    and (placement.startLine or 0) == (anchor.startLine or 0)
    and (placement.line or 0) == (anchor.line or 0)
    and placement.target == anchor.target
    and same_optional(placement.base, anchor.base)
    and same_optional(placement.head, anchor.head)
    and same_optional(placement.fingerprint, anchor.fingerprint)) then
    return false
  end

  local anchor_path = vim.fs.normalize(expected.root .. "/" .. anchor.path)
  local expected_start = expected.start_line or 0
  local expected_start_side = expected_start > 0 and expected.side or ""
  return anchor_path == vim.fs.normalize(expected.path)
    and anchor.side == expected.side
    and same_optional(anchor.startSide, expected_start_side)
    and (anchor.startLine or 0) == expected_start
    and (anchor.line or 0) == expected.line
    and anchor.target == expected.target
end

local function complete(callback, err, note)
  vim.schedule(function()
    if err then
      vim.notify("changes note: " .. err, vim.log.levels.ERROR)
    else
      vim.notify("changes note: " .. note.id, vim.log.levels.INFO)
    end
    if callback then
      callback(err, note)
    end
  end)
end

local function add_option(argv, name, value)
  if value ~= nil and value ~= "" then
    table.insert(argv, name)
    table.insert(argv, tostring(value))
  end
end

local function resolve_range(options, buffer)
  local finish = options.line
  if finish == nil then
    if buffer ~= vim.api.nvim_get_current_buf() then
      return nil, nil, "line is required for a non-current buffer"
    end
    finish = vim.api.nvim_win_get_cursor(0)[1]
  end
  local start = options.start_line
  if start and start > finish then
    start, finish = finish, start
  end
  if start == finish then
    start = nil
  end
  if finish < 1 or (start and start < 1) then
    return nil, nil, "lines must be positive"
  end
  return start, finish
end

local function file_digest(path)
  local path_stat, stat_error = vim.uv.fs_lstat(path)
  if not path_stat then
    return nil, nil, stat_error or "cannot inspect target file"
  end
  if path_stat.type ~= "file" then
    return nil, nil, "target path is not a regular file"
  end
  local handle, open_error = vim.uv.fs_open(path, "r", 438)
  if not handle then
    return nil, nil, open_error or "cannot open target file"
  end
  local stat, stat_error = vim.uv.fs_fstat(handle)
  if not stat then
    vim.uv.fs_close(handle)
    return nil, nil, stat_error or "cannot inspect target file"
  end
  if stat.type ~= "file" or stat.dev ~= path_stat.dev or stat.ino ~= path_stat.ino then
    vim.uv.fs_close(handle)
    return nil, nil, "target path changed while opening it"
  end
  if stat.size > max_file_bytes then
    vim.uv.fs_close(handle)
    return nil, nil, "target file exceeds the 16 MiB annotation limit"
  end
  local data, read_error = vim.uv.fs_read(handle, stat.size, 0)
  vim.uv.fs_close(handle)
  if data == nil then
    return nil, nil, read_error or "cannot read target file"
  end
  return vim.fn.sha256(data), stat.size
end

local function buffer_digest(buffer, disk_size)
  if vim.api.nvim_get_option_value("buftype", { buf = buffer }) ~= "" then
    return nil, "target buffer must be an ordinary file buffer"
  end
  if vim.api.nvim_get_option_value("binary", { buf = buffer }) then
    return nil, "target buffer must not use binary mode"
  end
  local encoding = vim.api.nvim_get_option_value("fileencoding", { buf = buffer })
  if encoding ~= "" and encoding:lower() ~= "utf-8" and encoding:lower() ~= "utf8" then
    return nil, "target buffer must use UTF-8 file encoding"
  end
  if vim.api.nvim_get_option_value("bomb", { buf = buffer }) then
    return nil, "target buffer must not use a byte-order mark"
  end
  local fileformat = vim.api.nvim_get_option_value("fileformat", { buf = buffer })
  local separator = ({ unix = "\n", dos = "\r\n", mac = "\r" })[fileformat]
  if separator == nil then
    return nil, "target buffer uses an unsupported file format"
  end
  local lines = vim.api.nvim_buf_get_lines(buffer, 0, -1, true)
  local size = math.max(#lines - 1, 0) * #separator
  for _, line in ipairs(lines) do
    size = size + #line
    if size > max_file_bytes then
      return nil, "target buffer exceeds the 16 MiB annotation limit"
    end
  end
  local data = table.concat(lines, separator)
  local empty_disk_buffer = disk_size == 0 and #lines == 1 and lines[1] == ""
  if not empty_disk_buffer and vim.api.nvim_get_option_value("endofline", { buf = buffer }) then
    data = data .. separator
  end
  if #data > max_file_bytes then
    return nil, "target buffer exceeds the 16 MiB annotation limit"
  end
  if not valid_utf8(data) then
    return nil, "target buffer must contain valid UTF-8"
  end
  return vim.fn.sha256(data)
end

local function capture_target(options)
  local buffer = options.buf or vim.api.nvim_get_current_buf()
  if buffer == 0 then
    buffer = vim.api.nvim_get_current_buf()
  end
  if not vim.api.nvim_buf_is_valid(buffer) then
    return nil, "the target buffer is no longer valid"
  end
  if vim.api.nvim_get_option_value("modified", { buf = buffer }) then
    return nil, "save the buffer before adding a diff note"
  end
  local name = vim.api.nvim_buf_get_name(buffer)
  if name == "" then
    return nil, "the current buffer has no file path"
  end
  local path = vim.fn.fnamemodify(name, ":p")
  local root = vim.fs.root(path, ".git")
  if root == nil or root == "" then
    root = vim.fn.fnamemodify(path, ":h")
  else
    root = vim.fn.fnamemodify(root, ":p")
  end
  local digest, file_size, digest_error = file_digest(path)
  if digest_error then
    return nil, "cannot read the target file: " .. digest_error
  end
  local viewed_digest, viewed_error = buffer_digest(buffer, file_size)
  if viewed_error then
    return nil, viewed_error
  end
  if viewed_digest ~= digest then
    return nil, "target buffer content does not match the target file"
  end
  local start_line, line, range_error = resolve_range(options, buffer)
  if range_error then
    return nil, range_error
  end
  return {
    buf = buffer,
    path = path,
    root = root,
    directory = vim.fn.fnamemodify(path, ":h"),
    start_line = start_line,
    line = line,
    changedtick = vim.api.nvim_buf_get_changedtick(buffer),
    digest = digest,
    file_size = file_size,
    viewed_digest = viewed_digest,
  }
end

local function validate_target(target)
  if not vim.api.nvim_buf_is_valid(target.buf) then
    return "the target buffer is no longer valid"
  end
  local path = vim.fn.fnamemodify(vim.api.nvim_buf_get_name(target.buf), ":p")
  if path ~= target.path then
    return "the target buffer path changed while entering the note"
  end
  if vim.api.nvim_get_option_value("modified", { buf = target.buf }) then
    return "save the buffer before adding a diff note"
  end
  if vim.api.nvim_buf_get_changedtick(target.buf) ~= target.changedtick then
    return "the target buffer changed while entering the note"
  end
  local digest, file_size, digest_error = file_digest(target.path)
  if digest_error then
    return "cannot read the target file: " .. digest_error
  end
  if digest ~= target.digest or file_size ~= target.file_size then
    return "the target file changed while entering the note"
  end
  local viewed_digest, viewed_error = buffer_digest(target.buf, file_size)
  if viewed_error then
    return viewed_error
  end
  if viewed_digest ~= target.viewed_digest then
    return "the target buffer changed while entering the note"
  end
  if viewed_digest ~= digest then
    return "target buffer content does not match the target file"
  end
end

function M.setup(options)
  if options ~= nil and type(options) ~= "table" then
    error("changes.notes.setup expects a table")
  end
  local next_config = vim.tbl_extend("force", {}, defaults, options or {})
  local _, err = command_argv(next_config.command)
  if err then
    error(err)
  end
  config = next_config
end

function M.add(options, callback)
  options = merged(options)
  local side = requested_side(options.side)
  local message = options.message
  if type(message) ~= "string" or message:match("^%s*$") then
    complete(callback, "message must contain text")
    return
  end

  local target = options._target
  local target_error
  if target == nil then
    target, target_error = capture_target(options)
  end
  if target_error then
    complete(callback, target_error)
    return
  end
  target_error = validate_target(target)
  if target_error then
    complete(callback, target_error)
    return
  end
  local argv, command_error = command_argv(options.command)
  if command_error then
    complete(callback, command_error)
    return
  end

  vim.list_extend(argv, { "note", "add", "--file", target.path, "--line", tostring(target.line) })
  add_option(argv, "--start-line", target.start_line)
  add_option(argv, "--side", side)
  add_option(argv, "--author", options.author)
  add_option(argv, "--origin", options.origin)
  add_option(argv, "--session", options.session)
  add_option(argv, "--provider", options.provider)
  add_option(argv, "--commit", options.commit)
  add_option(argv, "--from", options.from)
  add_option(argv, "--to", options.to)
  add_option(argv, "--expected-file-sha256", target.digest)
  if options.staged then
    table.insert(argv, "--staged")
  end
  vim.list_extend(argv, { "--message-file", "-", "--json" })

  local expected = {
    path = target.path,
    root = target.root,
    side = type(side) == "string" and vim.trim(side):upper() or "",
    start_line = target.start_line,
    line = target.line,
    target = requested_target(options),
  }

  local ok, process_error = pcall(vim.system, argv, {
    cwd = target.directory,
    text = true,
    stdin = message,
  }, function(result)
    local signal = result.signal or 0
    if result.code ~= 0 or signal ~= 0 then
      local detail = vim.trim(result.stderr or "")
      if detail == "" then
        if signal ~= 0 then
          detail = "changes terminated by signal " .. tostring(signal)
        else
          detail = "changes exited with status " .. tostring(result.code)
        end
      end
      complete(callback, detail)
      return
    end
    local decoded, note = pcall(vim.json.decode, result.stdout or "")
    if not decoded or not valid_note(note, expected) then
      complete(callback, "changes returned invalid JSON")
      return
    end
    complete(callback, nil, note)
  end)
  if not ok then
    complete(callback, tostring(process_error))
  end
end

function M.prompt(options, callback)
  options = merged(options)
  local target, target_error = capture_target(options)
  if target_error then
    complete(callback, target_error)
    return
  end
  options._target = target
  vim.ui.input({ prompt = "Diff note: ", scope = "line" }, function(message)
    if message == nil or message:match("^%s*$") then
      return
    end
    options.message = message
    M.add(options, callback)
  end)
end

return M
