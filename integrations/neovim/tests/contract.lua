local root = vim.fn.fnamemodify(assert(arg[1], "plugin root argument is required"), ":p")
vim.opt.runtimepath:prepend(root)

local failures = {}

local function check(condition, message)
  if not condition then
    table.insert(failures, message)
  end
end

local function wait_for(predicate, message)
  check(vim.wait(1000, predicate), message)
end

local directory = vim.fn.tempname() .. " path"
vim.fn.mkdir(directory, "p")
local path = directory .. "/file name.lua"
vim.fn.writefile({ "one", "two", "three", "four", "five", "six", "seven" }, path)
vim.cmd.edit(vim.fn.fnameescape(path))
vim.api.nvim_win_set_cursor(0, { 4, 0 })
local resolved_path = vim.fn.fnamemodify(vim.api.nvim_buf_get_name(0), ":p")
local resolved_directory = vim.fn.fnamemodify(resolved_path, ":h")

local notifications = {}
vim.notify = function(message, level)
  table.insert(notifications, { message = message, level = level })
end

local function valid_note_json(id, line, start_line, target, mutate)
  local anchor = {
    path = "file name.lua",
    side = "RIGHT",
    line = line,
    target = target,
  }
  if start_line then
    anchor.startSide = "RIGHT"
    anchor.startLine = start_line
  end
  local placement = vim.deepcopy(anchor)
  placement.quality = "exact"
  local note = {
    id = id,
    source = "git-notes",
    sourceId = id,
    summary = "Saved note",
    author = "reviewer",
    origin = "user",
    authority = "owner",
    state = "open",
    createdAt = "2026-09-06T12:00:00Z",
    anchor = anchor,
    placement = placement,
  }
  if mutate then
    mutate(note)
  end
  return vim.json.encode(note) .. "\n"
end

local captured
vim.system = function(argv, options, callback)
  captured = { argv = argv, options = options }
  callback({ code = 0, stdout = valid_note_json("git-notes:one", 7, 5, "commits"), stderr = "" })
  return {}
end

local notes = require("changes.notes")
local callback_result
local message = "keep $(printf quoted) 'value'\nwith context"
notes.add({
  message = message,
  start_line = 7,
  line = 5,
  provider = "git-notes",
  commit = "HEAD",
}, function(err, note)
  callback_result = { err = err, note = note }
end)
wait_for(function()
  return callback_result ~= nil
end, "add callback did not run")
check(callback_result and callback_result.err == nil, "successful add returned an error")
check(callback_result and callback_result.note.id == "git-notes:one", "successful add lost the decoded note")
check(captured.options.stdin == message, "note message did not remain exact stdin data")
check(captured.options.cwd == resolved_directory, "process cwd is not the buffer parent")
check(captured.options.text == true, "process did not request text output")

local expected = {
  "changes", "note", "add", "--file", resolved_path, "--line", "7", "--start-line", "5",
  "--side", "right", "--origin", "user", "--provider", "git-notes", "--commit", "HEAD",
  "--expected-file-sha256", vim.fn.sha256(table.concat({ "one", "two", "three", "four", "five", "six", "seven", "" }, "\n")),
  "--message-file", "-", "--json",
}
check(vim.deep_equal(captured.argv, expected), "argv did not preserve path, range, or options")

for _, fixture in ipairs({
  { name = "staged", options = { staged = true }, target = "index" },
  { name = "committed range", options = { from = "HEAD^", to = "HEAD" }, target = "commits" },
  { name = "working from revision", options = { from = "HEAD" }, target = "working" },
  { name = "normalized side", options = { side = " right " }, target = "working" },
}) do
  local accepted
  vim.system = function(_, _, callback)
    callback({ code = 0, stdout = valid_note_json(fixture.name, 4, nil, fixture.target), stderr = "" })
    return {}
  end
  local options = vim.tbl_extend("force", { message = fixture.name, line = 4 }, fixture.options)
  notes.add(options, function(err, note)
    accepted = { err = err, note = note }
  end)
  wait_for(function()
    return accepted ~= nil
  end, fixture.name .. " callback did not run")
  check(accepted and accepted.err == nil, fixture.name .. " response target was rejected")
end

local empty_side_capture
local empty_side_result
notes.setup({ side = "" })
vim.system = function(argv, _, callback)
  empty_side_capture = argv
  callback({ code = 0, stdout = valid_note_json("empty-side", 4, nil, "working"), stderr = "" })
  return {}
end
notes.add({ message = "empty configured side", line = 4 }, function(err, note)
  empty_side_result = { err = err, note = note }
end)
wait_for(function()
  return empty_side_result ~= nil
end, "empty-side callback did not run")
local empty_side_value
for index, value in ipairs(empty_side_capture or {}) do
  if value == "--side" then
    empty_side_value = empty_side_capture[index + 1]
  end
end
check(empty_side_value == "right", "empty configured side was not normalized before execution")
check(
  empty_side_result and empty_side_result.err == nil and empty_side_result.note.id == "empty-side",
  "empty configured side wrote a note then rejected the matching response"
)
notes.setup()

local repository = vim.fn.tempname() .. " repository"
local nested_directory = repository .. "/nested"
vim.fn.mkdir(repository .. "/.git", "p")
vim.fn.mkdir(nested_directory, "p")
local nested_path = nested_directory .. "/nested.lua"
vim.fn.writefile({ "nested" }, nested_path)
vim.cmd.edit(vim.fn.fnameescape(nested_path))
local rooted
vim.system = function(_, _, callback)
  callback({
    code = 0,
    stdout = valid_note_json("rooted", 1, nil, "working", function(note)
      note.anchor.path = "nested/nested.lua"
      note.placement.path = "nested/nested.lua"
    end),
    stderr = "",
  })
  return {}
end
notes.add({ message = "rooted", line = 1 }, function(err, note)
  rooted = { err = err, note = note }
end)
wait_for(function()
  return rooted ~= nil
end, "repository-root response callback did not run")
check(rooted and rooted.err == nil, "repository-relative response path did not resolve from the captured root")
vim.cmd.edit(vim.fn.fnameescape(path))

local process_count = 0
vim.system = function()
  process_count = process_count + 1
end
vim.ui.input = function(options, callback)
  check(options.scope == "line", "prompt scope is not line")
  callback(nil)
end
notes.prompt()
vim.ui.input = function(_, callback)
  callback("   ")
end
notes.prompt()
check(process_count == 0, "cancelled or empty prompt started a process")

vim.api.nvim_buf_set_lines(0, 0, 0, false, { "unsaved" })
local dirty_error
notes.add({ message = "must not run" }, function(err)
  dirty_error = err
end)
wait_for(function()
  return dirty_error ~= nil
end, "dirty-buffer callback did not run")
check(dirty_error == "save the buffer before adding a diff note", "dirty buffer did not fail closed")
check(process_count == 0, "dirty buffer started a process")
vim.cmd.undo()

vim.api.nvim_buf_set_lines(0, 0, 1, false, { "viewed only" })
vim.api.nvim_set_option_value("modified", false, { buf = 0 })
local detached_error
notes.add({ message = "must not run" }, function(err)
  detached_error = err
end)
wait_for(function()
  return detached_error ~= nil
end, "detached-buffer callback did not run")
check(
  detached_error == "target buffer content does not match the target file",
  "buffer content detached from disk did not fail closed"
)
check(process_count == 0, "buffer detached from disk started a process")
vim.cmd("edit!")

local function write_bytes(target, data)
  local handle = assert(vim.uv.fs_open(target, "w", 420))
  if #data > 0 then
    assert(vim.uv.fs_write(handle, data, 0))
  end
  assert(vim.uv.fs_close(handle))
end

for _, fixture in ipairs({
  { name = "empty", data = "" },
  { name = "one-newline", data = "\n" },
  { name = "no-eol", data = "one" },
  { name = "crlf", data = "one\r\ntwo\r\n" },
  { name = "utf8", data = string.char(0xC3, 0xA9) .. "\n" },
}) do
  local fixture_path = directory .. "/" .. fixture.name .. ".txt"
  write_bytes(fixture_path, fixture.data)
  vim.cmd.edit(vim.fn.fnameescape(fixture_path))
  local accepted
  vim.system = function(_, _, callback)
    callback({
      code = 0,
      stdout = valid_note_json("bytes", 1, nil, "working", function(note)
        note.anchor.path = fixture.name .. ".txt"
        note.placement.path = fixture.name .. ".txt"
      end),
      stderr = "",
    })
    return {}
  end
  notes.add({ message = "byte fixture", line = 1 }, function(err, note)
    accepted = { err = err, note = note }
  end)
  wait_for(function()
    return accepted ~= nil
  end, fixture.name .. " callback did not run")
  check(accepted and accepted.err == nil, fixture.name .. " file did not pass byte validation")
end
vim.cmd.edit(vim.fn.fnameescape(path))
vim.system = function()
  process_count = process_count + 1
end
process_count = 0

local large_path = directory .. "/large.lua"
local large_handle = assert(vim.uv.fs_open(large_path, "w", 420))
assert(vim.uv.fs_ftruncate(large_handle, 16 * 1024 * 1024 + 1))
assert(vim.uv.fs_close(large_handle))
local large_buffer = vim.api.nvim_create_buf(false, true)
vim.api.nvim_buf_set_name(large_buffer, large_path)
local large_error
notes.add({ message = "must not run", buf = large_buffer, line = 1 }, function(err)
  large_error = err
end)
wait_for(function()
  return large_error ~= nil
end, "large-file callback did not run")
check(
  large_error == "cannot read the target file: target file exceeds the 16 MiB annotation limit",
  "large target file did not fail closed"
)
check(process_count == 0, "large target file started a process")
vim.api.nvim_buf_delete(large_buffer, { force = true })

local binary_path = directory .. "/binary.txt"
write_bytes(binary_path, "binary\n")
vim.cmd.edit(vim.fn.fnameescape(binary_path))
vim.api.nvim_set_option_value("binary", true, { buf = 0 })
local binary_error
notes.add({ message = "must not run", line = 1 }, function(err)
  binary_error = err
end)
wait_for(function()
  return binary_error ~= nil
end, "binary-buffer callback did not run")
check(binary_error == "target buffer must not use binary mode", "binary buffer did not fail closed")
check(process_count == 0, "binary buffer started a process")

local invalid_utf8_path = directory .. "/invalid-utf8.txt"
write_bytes(invalid_utf8_path, string.char(0xFF) .. "\n")
vim.cmd("edit ++bin " .. vim.fn.fnameescape(invalid_utf8_path))
vim.api.nvim_set_option_value("binary", false, { buf = 0 })
local invalid_utf8_error
notes.add({ message = "must not run", line = 1 }, function(err)
  invalid_utf8_error = err
end)
wait_for(function()
  return invalid_utf8_error ~= nil
end, "invalid-UTF-8 callback did not run")
check(invalid_utf8_error == "target buffer must contain valid UTF-8", "invalid UTF-8 did not fail closed")
check(process_count == 0, "invalid UTF-8 started a process")
vim.cmd.edit(vim.fn.fnameescape(path))

local delayed_input
local delayed_capture
vim.system = function(argv, options, callback)
  delayed_capture = { argv = argv, options = options }
  callback({ code = 0, stdout = valid_note_json("delayed", 4, nil, "working"), stderr = "" })
  return {}
end
vim.ui.input = function(_, callback)
  delayed_input = callback
end
vim.api.nvim_win_set_cursor(0, { 4, 0 })
notes.prompt()
local second_directory = vim.fn.tempname() .. " other"
vim.fn.mkdir(second_directory, "p")
local second_path = second_directory .. "/other.lua"
vim.fn.writefile({ "other" }, second_path)
vim.cmd.edit(vim.fn.fnameescape(second_path))
vim.cmd.cd(vim.fn.fnameescape(second_directory))
delayed_input("delayed note")
check(delayed_capture ~= nil, "delayed prompt did not start a process")
check(delayed_capture and delayed_capture.options.cwd == resolved_directory, "delayed prompt changed repository")
check(delayed_capture and delayed_capture.argv[5] == resolved_path, "delayed prompt changed buffer path")
check(delayed_capture and delayed_capture.argv[7] == "4", "delayed prompt changed cursor line")
vim.cmd.edit(vim.fn.fnameescape(path))
vim.cmd.cd(vim.fn.fnameescape(directory))

local disk_input
local disk_error
process_count = 0
vim.system = function()
  process_count = process_count + 1
end
vim.ui.input = function(_, callback)
  disk_input = callback
end
notes.prompt({}, function(err)
  disk_error = err
end)
vim.fn.writefile({ "external", "two", "three", "four", "five", "six", "seven" }, path)
disk_input("must not run")
wait_for(function()
  return disk_error ~= nil
end, "external-file callback did not run")
check(disk_error == "the target file changed while entering the note", "external file change did not fail closed")
check(process_count == 0, "external file change started a process")
vim.fn.writefile({ "one", "two", "three", "four", "five", "six", "seven" }, path)
vim.cmd("edit!")

local failure
vim.system = function(_, _, callback)
  callback({ code = 9, stdout = "", stderr = "provider failed\n" })
  return {}
end
notes.add({ message = "failure" }, function(err, note)
  failure = { err = err, note = note }
end)
wait_for(function()
  return failure ~= nil
end, "failure callback did not run")
check(failure and failure.err == "provider failed" and failure.note == nil, "process failure was not observable")
check(notifications[#notifications].level == vim.log.levels.ERROR, "process failure did not notify at error level")

local signaled
vim.system = function(_, _, callback)
  callback({
    code = 0,
    signal = 15,
    stdout = valid_note_json("signaled", 4, nil, "working"),
    stderr = "",
  })
  return {}
end
notes.add({ message = "signaled" }, function(err, note)
  signaled = { err = err, note = note }
end)
wait_for(function()
  return signaled ~= nil
end, "signaled-process callback did not run")
check(
  signaled and signaled.err == "changes terminated by signal 15" and signaled.note == nil,
  "signaled process was accepted"
)
check(notifications[#notifications].level == vim.log.levels.ERROR, "signaled process did not notify at error level")

local invalid
vim.system = function(_, _, callback)
  callback({ code = 0, stdout = "not json", stderr = "" })
  return {}
end
notes.add({ message = "invalid json" }, function(err)
  invalid = err
end)
wait_for(function()
  return invalid ~= nil
end, "invalid JSON callback did not run")
check(invalid == "changes returned invalid JSON", "invalid JSON error was not stable")

local incomplete
local notification_count = #notifications
vim.system = function(_, _, callback)
  callback({ code = 0, stdout = '{"id":"incomplete"}\n', stderr = "" })
  return {}
end
notes.add({ message = "incomplete object" }, function(err, note)
  incomplete = { err = err, note = note }
end)
wait_for(function()
  return incomplete ~= nil
end, "incomplete-object callback did not run")
check(
  incomplete and incomplete.err == "changes returned invalid JSON" and incomplete.note == nil,
  "incomplete note object was accepted"
)
check(#notifications == notification_count + 1, "incomplete note object did not notify")
check(
  notifications[#notifications].message == "changes note: changes returned invalid JSON"
    and notifications[#notifications].level == vim.log.levels.ERROR,
  "incomplete note object did not notify at error level"
)

local timestamped
vim.system = function(_, _, callback)
  callback({
    code = 0,
    stdout = valid_note_json("timestamped", 4, nil, "working", function(note)
      note.createdAt = "2026-09-06T12:00:00.123-07:00"
      note.updatedAt = "2026-09-06T19:00:00Z"
    end),
    stderr = "",
  })
  return {}
end
notes.add({ message = "valid timestamps" }, function(err, note)
  timestamped = { err = err, note = note }
end)
wait_for(function()
  return timestamped ~= nil
end, "timestamped response callback did not run")
check(timestamped and timestamped.err == nil and timestamped.note.id == "timestamped", "valid timestamps were rejected")

for _, fixture in ipairs({
  {
    name = "createdAt",
    mutate = function(note)
      note.createdAt = "2026-02-30T12:00:00Z"
    end,
  },
  {
    name = "updatedAt",
    mutate = function(note)
      note.updatedAt = "not-rfc3339"
    end,
  },
  {
    name = "quality",
    mutate = function(note)
      note.placement.quality = "context"
    end,
  },
  {
    name = "path",
    mutate = function(note)
      note.placement.path = "other.lua"
    end,
  },
  {
    name = "side",
    mutate = function(note)
      note.placement.side = "LEFT"
    end,
  },
  {
    name = "start side",
    mutate = function(note)
      note.placement.startSide = "LEFT"
    end,
  },
  {
    name = "start line",
    mutate = function(note)
      note.placement.startLine = 6
    end,
  },
  {
    name = "end line",
    mutate = function(note)
      note.placement.line = 6
    end,
  },
  {
    name = "target",
    mutate = function(note)
      note.placement.target = "index"
    end,
  },
  {
    name = "base",
    mutate = function(note)
      note.placement.base = "other-base"
    end,
  },
  {
    name = "head",
    mutate = function(note)
      note.placement.head = "other-head"
    end,
  },
  {
    name = "fingerprint",
    mutate = function(note)
      note.placement.fingerprint = "other-fingerprint"
    end,
  },
  {
    name = "requested path",
    mutate = function(note)
      note.anchor.path = "other.lua"
      note.placement.path = "other.lua"
    end,
  },
  {
    name = "requested side",
    mutate = function(note)
      note.anchor.side = "LEFT"
      note.placement.side = "LEFT"
      note.anchor.startSide = "LEFT"
      note.placement.startSide = "LEFT"
    end,
  },
  {
    name = "requested start line",
    mutate = function(note)
      note.anchor.startLine = 6
      note.placement.startLine = 6
    end,
  },
  {
    name = "requested end line",
    mutate = function(note)
      note.anchor.line = 6
      note.placement.line = 6
    end,
  },
  {
    name = "requested target",
    mutate = function(note)
      note.anchor.target = "commits"
      note.placement.target = "commits"
    end,
  },
}) do
  local rejected
  vim.system = function(_, _, callback)
    callback({
      code = 0,
      stdout = valid_note_json("invalid-create", 7, 5, "working", fixture.mutate),
      stderr = "",
    })
    return {}
  end
  notes.add({ message = fixture.name, start_line = 5, line = 7 }, function(err, note)
    rejected = { err = err, note = note }
  end)
  wait_for(function()
    return rejected ~= nil
  end, fixture.name .. " response callback did not run")
  check(
    rejected and rejected.err == "changes returned invalid JSON" and rejected.note == nil,
    fixture.name .. " response was accepted"
  )
end

vim.cmd.runtime("plugin/changes.lua")
check(vim.fn.exists(":ChangesNote") == 2, ":ChangesNote is not registered")
check(vim.fn.maparg("<Plug>(changes-note)", "n") ~= "", "normal Plug mapping is missing")
check(vim.fn.maparg("<Plug>(changes-note)", "x") ~= "", "visual Plug mapping is missing")
check(vim.fn.maparg("<leader>cn", "n") == "", "plugin installed a user keybinding")

if #failures > 0 then
  error(table.concat(failures, "\n"))
end

print("changes.nvim contract passed")
