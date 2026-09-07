complete -c changes -e
complete -c changes -f
function __changes_completion_values_0
  begin
    printf '%s\n' 'auto' 'always' 'never'
  end | string match -rv '\t'; or true
end
function __changes_completion_values_1
  begin
    printf '%s\n' 'builtin' 'filter'
  end | string match -rv '\t'; or true
end
function __changes_completion_values_2
  begin
    command 'changes' '__values' 'group-providers' (commandline -cp) 2>/dev/null; or true
  end | string match -rv '\t'; or true
end
function __changes_completion_values_3
  begin
    printf '%s\n' 'unified' 'side-by-side'
  end | string match -rv '\t'; or true
end
function __changes_completion_values_4
  begin
    printf '%s\n' 'completion' 'difftool' 'render' 'generate' 'note' 'provider'
    command 'changes' '__values' 'repository' 2>/dev/null; or true
  end | string match -rv '\t'; or true
end
function __changes_completion_values_5
  begin
    command 'changes' '__values' 'paths' 2>/dev/null; or true
  end | string match -rv '\t'; or true
end
function __changes_completion_values_6
  begin
    printf '%s\n' 'auto' 'always' 'never'
  end | string match -rv '\t'; or true
end
function __changes_completion_values_7
  begin
    printf '%s\n' 'builtin' 'difftool'
  end | string match -rv '\t'; or true
end
function __changes_completion_values_8
  begin
    printf '%s\n' 'unified' 'side-by-side'
  end | string match -rv '\t'; or true
end
function __changes_completion_values_9
  begin
    printf '%s\n' 'auto' 'always' 'never'
  end | string match -rv '\t'; or true
end
function __changes_completion_values_10
  begin
    printf '%s\n' 'builtin' 'filter'
  end | string match -rv '\t'; or true
end
function __changes_completion_values_11
  begin
    printf '%s\n' 'unified' 'side-by-side'
  end | string match -rv '\t'; or true
end
function __changes_completion_values_12
  begin
    command 'changes' '__values' 'repository' 2>/dev/null; or true
  end | string match -rv '\t'; or true
end
function __changes_completion_values_13
  begin
    command 'changes' '__values' 'paths' 2>/dev/null; or true
  end | string match -rv '\t'; or true
end
function __changes_completion_values_14
  begin
    command 'changes' '__values' 'repository' 2>/dev/null; or true
  end | string match -rv '\t'; or true
end
function __changes_completion_values_15
  begin
    printf '%s\n' 'agent' 'user'
  end | string match -rv '\t'; or true
end
function __changes_completion_values_16
  begin
    command 'changes' '__values' 'note-writers' (commandline -cp) 2>/dev/null; or true
  end | string match -rv '\t'; or true
end
function __changes_completion_values_17
  begin
    printf '%s\n' 'left' 'right'
  end | string match -rv '\t'; or true
end
function __changes_completion_values_18
  begin
    command 'changes' '__values' 'repository' 2>/dev/null; or true
  end | string match -rv '\t'; or true
end
function __changes_completion_values_19
  begin
    command 'changes' '__values' 'repository' 2>/dev/null; or true
  end | string match -rv '\t'; or true
end
function __changes_completion_values_20
  begin
    command 'changes' '__values' 'repository' 2>/dev/null; or true
  end | string match -rv '\t'; or true
end
function __changes_completion_values_21
  begin
    command 'changes' '__values' 'note-generators' (commandline -cp) 2>/dev/null; or true
  end | string match -rv '\t'; or true
end
function __changes_completion_values_22
  begin
    command 'changes' '__values' 'note-writers' (commandline -cp) 2>/dev/null; or true
  end | string match -rv '\t'; or true
end
function __changes_completion_values_23
  begin
    command 'changes' '__values' 'repository' 2>/dev/null; or true
  end | string match -rv '\t'; or true
end
function __changes_completion_values_24
  begin
    command 'changes' '__values' 'repository' 2>/dev/null; or true
  end | string match -rv '\t'; or true
end
function __changes_completion_values_25
  begin
    command 'changes' '__values' 'repository' 2>/dev/null; or true
  end | string match -rv '\t'; or true
end
function __changes_completion_values_26
  begin
    command 'changes' '__values' 'note-readers' (commandline -cp) 2>/dev/null; or true
  end | string match -rv '\t'; or true
end
function __changes_completion_values_27
  begin
    command 'changes' '__values' 'repository' 2>/dev/null; or true
  end | string match -rv '\t'; or true
end
function __changes_completion_values_28
  begin
    command 'changes' '__values' 'providers' (commandline -cp) 2>/dev/null; or true
  end | string match -rv '\t'; or true
end
function __changes_completion_values_29
  begin
    command 'changes' '__values' 'providers' (commandline -cp) 2>/dev/null; or true
  end | string match -rv '\t'; or true
end

function __changes_completion_context
  set -l context ''
  set -l words (commandline -opc)
  set -l consume_value 0
  set -l options_done 0
  for word in $words[2..-1]
    if test $consume_value -eq 1
      set consume_value 0
      continue
    end
    if test $options_done -eq 1
      continue
    end
    if test "$word" = '--'
      set options_done 1
      continue
    end
    switch "$context:$word"
      case ':--budget'
        set consume_value 1
        continue
      case ':--budget=*'
        continue
      case ':--color'
        set consume_value 1
        continue
      case ':--color=*'
        continue
      case ':--config'
        set consume_value 1
        continue
      case ':--config=*'
        continue
      case ':--engine'
        set consume_value 1
        continue
      case ':--engine=*'
        continue
      case ':--filter'
        set consume_value 1
        continue
      case ':--filter=*'
        continue
      case ':--group-provider'
        set consume_value 1
        continue
      case ':--group-provider=*'
        continue
      case ':--interval'
        set consume_value 1
        continue
      case ':--interval=*'
        continue
      case ':--layout'
        set consume_value 1
        continue
      case ':--layout=*'
        continue
      case ':--root'
        set consume_value 1
        continue
      case ':--root=*'
        continue
      case ':--since'
        set consume_value 1
        continue
      case ':--since=*'
        continue
      case ':--width'
        set consume_value 1
        continue
      case ':--width=*'
        continue
      case 'difftool:--color'
        set consume_value 1
        continue
      case 'difftool:--color=*'
        continue
      case 'difftool:--config'
        set consume_value 1
        continue
      case 'difftool:--config=*'
        continue
      case 'difftool:--engine'
        set consume_value 1
        continue
      case 'difftool:--engine=*'
        continue
      case 'difftool:--difftool'
        set consume_value 1
        continue
      case 'difftool:--difftool=*'
        continue
      case 'difftool:--layout'
        set consume_value 1
        continue
      case 'difftool:--layout=*'
        continue
      case 'difftool:--width'
        set consume_value 1
        continue
      case 'difftool:--width=*'
        continue
      case 'render:--color'
        set consume_value 1
        continue
      case 'render:--color=*'
        continue
      case 'render:--config'
        set consume_value 1
        continue
      case 'render:--config=*'
        continue
      case 'render:--engine'
        set consume_value 1
        continue
      case 'render:--engine=*'
        continue
      case 'render:--filter'
        set consume_value 1
        continue
      case 'render:--filter=*'
        continue
      case 'render:--layout'
        set consume_value 1
        continue
      case 'render:--layout=*'
        continue
      case 'render:--width'
        set consume_value 1
        continue
      case 'render:--width=*'
        continue
      case 'note add:--author'
        set consume_value 1
        continue
      case 'note add:--author=*'
        continue
      case 'note add:--commit'
        set consume_value 1
        continue
      case 'note add:--commit=*'
        continue
      case 'note add:--config'
        set consume_value 1
        continue
      case 'note add:--config=*'
        continue
      case 'note add:--expected-file-sha256'
        set consume_value 1
        continue
      case 'note add:--expected-file-sha256=*'
        continue
      case 'note add:--file'
        set consume_value 1
        continue
      case 'note add:--file=*'
        continue
      case 'note add:--from'
        set consume_value 1
        continue
      case 'note add:--from=*'
        continue
      case 'note add:--line'
        set consume_value 1
        continue
      case 'note add:--line=*'
        continue
      case 'note add:--message'
        set consume_value 1
        continue
      case 'note add:--message=*'
        continue
      case 'note add:--message-file'
        set consume_value 1
        continue
      case 'note add:--message-file=*'
        continue
      case 'note add:--origin'
        set consume_value 1
        continue
      case 'note add:--origin=*'
        continue
      case 'note add:--provider'
        set consume_value 1
        continue
      case 'note add:--provider=*'
        continue
      case 'note add:--session'
        set consume_value 1
        continue
      case 'note add:--session=*'
        continue
      case 'note add:--side'
        set consume_value 1
        continue
      case 'note add:--side=*'
        continue
      case 'note add:--start-line'
        set consume_value 1
        continue
      case 'note add:--start-line=*'
        continue
      case 'note add:--to'
        set consume_value 1
        continue
      case 'note add:--to=*'
        continue
      case 'note generate:--commit'
        set consume_value 1
        continue
      case 'note generate:--commit=*'
        continue
      case 'note generate:--config'
        set consume_value 1
        continue
      case 'note generate:--config=*'
        continue
      case 'note generate:--from'
        set consume_value 1
        continue
      case 'note generate:--from=*'
        continue
      case 'note generate:--provider'
        set consume_value 1
        continue
      case 'note generate:--provider=*'
        continue
      case 'note generate:--session'
        set consume_value 1
        continue
      case 'note generate:--session=*'
        continue
      case 'note generate:--store'
        set consume_value 1
        continue
      case 'note generate:--store=*'
        continue
      case 'note generate:--to'
        set consume_value 1
        continue
      case 'note generate:--to=*'
        continue
      case 'note list:--commit'
        set consume_value 1
        continue
      case 'note list:--commit=*'
        continue
      case 'note list:--config'
        set consume_value 1
        continue
      case 'note list:--config=*'
        continue
      case 'note list:--from'
        set consume_value 1
        continue
      case 'note list:--from=*'
        continue
      case 'note list:--provider'
        set consume_value 1
        continue
      case 'note list:--provider=*'
        continue
      case 'note list:--to'
        set consume_value 1
        continue
      case 'note list:--to=*'
        continue
      case 'provider list:--config'
        set consume_value 1
        continue
      case 'provider list:--config=*'
        continue
      case 'provider validate:--config'
        set consume_value 1
        continue
      case 'provider validate:--config=*'
        continue
    end
    switch "$context:$word"
      case ':completion'
        set context 'completion'
      case ':difftool'
        set context 'difftool'
      case ':render'
        set context 'render'
      case ':generate'
        set context 'generate'
      case ':note'
        set context 'note'
      case 'note:add'
        set context 'note add'
      case 'note:generate'
        set context 'note generate'
      case 'note:list'
        set context 'note list'
      case ':provider'
        set context 'provider'
      case 'provider:list'
        set context 'provider list'
      case 'provider:validate'
        set context 'provider validate'
    end
  end
  echo $context
end
complete -c changes -n 'test (__changes_completion_context) = ""' -l budget -r -d 'Analysis time budget'
complete -c changes -n 'test (__changes_completion_context) = ""' -f -l color -r -a '(__changes_completion_values_0)' -d 'Color output'
complete -c changes -n 'test (__changes_completion_context) = ""' -l config -r -d 'YAML configuration file'
complete -c changes -n 'test (__changes_completion_context) = ""' -f -l engine -r -a '(__changes_completion_values_1)' -d 'Patch display engine'
complete -c changes -n 'test (__changes_completion_context) = ""' -l filter -r -d 'Standard-input patch filter'
complete -c changes -n 'test (__changes_completion_context) = ""' -f -l group-provider -r -a '(__changes_completion_values_2)' -d 'Logical change-group provider'
complete -c changes -n 'test (__changes_completion_context) = ""' -l interval -r -d 'Watch interval'
complete -c changes -n 'test (__changes_completion_context) = ""' -f -l layout -r -a '(__changes_completion_values_3)' -d 'Diff layout'
complete -c changes -n 'test (__changes_completion_context) = ""' -l no-calls -d 'Skip call analysis'
complete -c changes -n 'test (__changes_completion_context) = ""' -l no-groups -d 'Skip logical change grouping'
complete -c changes -n 'test (__changes_completion_context) = ""' -l no-notes -d 'Skip diff notes'
complete -c changes -n 'test (__changes_completion_context) = ""' -l no-symbols -d 'Skip symbol analysis'
complete -c changes -n 'test (__changes_completion_context) = ""' -l recursive -s r -d 'Read all workspace repositories'
complete -c changes -n 'test (__changes_completion_context) = ""' -l root -r -d 'Workspace scan root'
complete -c changes -n 'test (__changes_completion_context) = ""' -l since -r -d 'Left revision or time'
complete -c changes -n 'test (__changes_completion_context) = ""' -l staged -d 'Compare the index'
complete -c changes -n 'test (__changes_completion_context) = ""' -l stat -s s -d 'Show change summary'
complete -c changes -n 'test (__changes_completion_context) = ""' -l version -d 'Print the Changes version'
complete -c changes -n 'test (__changes_completion_context) = ""' -l watch -s w -d 'Watch for changes'
complete -c changes -n 'test (__changes_completion_context) = ""' -l width -r -d 'Render width'
complete -c changes -f -n 'test (__changes_completion_context) = ""' -a completion -d 'Generate shell completions'
complete -c changes -f -n 'test (__changes_completion_context) = ""' -a difftool -d 'Compare Git difftool LOCAL and REMOTE files'
complete -c changes -f -n 'test (__changes_completion_context) = ""' -a render -d 'Render a patch from standard input'
complete -c changes -f -n 'test (__changes_completion_context) = ""' -a generate -d 'Generate README command docs and JSON Schema'
complete -c changes -f -n 'test (__changes_completion_context) = ""' -a note -d 'Create and inspect diff notes'
complete -c changes -f -n 'test (__changes_completion_context) = ""' -a provider -d 'Inspect and validate context providers'
complete -c changes -f -n 'test (__changes_completion_context) = ""' -a '(__changes_completion_values_4)'
complete -c changes -f -n 'test (__changes_completion_context) = "completion"' -a 'bash zsh fish nu'
complete -c changes -n 'test (__changes_completion_context) = "difftool"' -f -l color -r -a '(__changes_completion_values_6)' -d 'Color output'
complete -c changes -n 'test (__changes_completion_context) = "difftool"' -l config -r -d 'YAML configuration file'
complete -c changes -n 'test (__changes_completion_context) = "difftool"' -f -l engine -r -a '(__changes_completion_values_7)' -d 'File comparison engine'
complete -c changes -n 'test (__changes_completion_context) = "difftool"' -l difftool -r -d 'Git-compatible difftool executable'
complete -c changes -n 'test (__changes_completion_context) = "difftool"' -f -l layout -r -a '(__changes_completion_values_8)' -d 'Diff layout'
complete -c changes -n 'test (__changes_completion_context) = "difftool"' -l width -r -d 'Render width'
complete -c changes -f -n 'test (__changes_completion_context) = "difftool"' -a '(__changes_completion_values_5)'
complete -c changes -n 'test (__changes_completion_context) = "render"' -f -l color -r -a '(__changes_completion_values_9)' -d 'Color output'
complete -c changes -n 'test (__changes_completion_context) = "render"' -l config -r -d 'YAML configuration file'
complete -c changes -n 'test (__changes_completion_context) = "render"' -f -l engine -r -a '(__changes_completion_values_10)' -d 'Patch display engine'
complete -c changes -n 'test (__changes_completion_context) = "render"' -l filter -r -d 'Standard-input patch filter'
complete -c changes -n 'test (__changes_completion_context) = "render"' -f -l layout -r -a '(__changes_completion_values_11)' -d 'Diff layout'
complete -c changes -n 'test (__changes_completion_context) = "render"' -l width -r -d 'Render width'
complete -c changes -n 'test (__changes_completion_context) = "generate"' -l check -d 'Fail when generated files are stale'
complete -c changes -f -n 'test (__changes_completion_context) = "note"' -a add -d 'Create a note on the selected diff'
complete -c changes -f -n 'test (__changes_completion_context) = "note"' -a generate -d 'Generate notes with a provider and save them'
complete -c changes -f -n 'test (__changes_completion_context) = "note"' -a list -d 'List notes on the selected diff'
complete -c changes -n 'test (__changes_completion_context) = "note add"' -l author -r -d 'Note author'
complete -c changes -n 'test (__changes_completion_context) = "note add"' -f -l commit -r -a '(__changes_completion_values_12)' -d 'First-parent commit comparison'
complete -c changes -n 'test (__changes_completion_context) = "note add"' -l config -r -d 'YAML configuration file'
complete -c changes -n 'test (__changes_completion_context) = "note add"' -l expected-file-sha256 -r -d 'Require the selected file side to match this SHA-256 digest'
complete -c changes -n 'test (__changes_completion_context) = "note add"' -f -l file -r -a '(__changes_completion_values_13)' -d 'Repository file to annotate'
complete -c changes -n 'test (__changes_completion_context) = "note add"' -f -l from -r -a '(__changes_completion_values_14)' -d 'Left revision'
complete -c changes -n 'test (__changes_completion_context) = "note add"' -l json -d 'Print the created note as JSON'
complete -c changes -n 'test (__changes_completion_context) = "note add"' -l line -r -d 'Last line of the note range'
complete -c changes -n 'test (__changes_completion_context) = "note add"' -l message -r -d 'Summary and optional rationale'
complete -c changes -n 'test (__changes_completion_context) = "note add"' -l message-file -r -d 'Read note text from a file or standard input'
complete -c changes -n 'test (__changes_completion_context) = "note add"' -f -l origin -r -a '(__changes_completion_values_15)' -d 'Author kind'
complete -c changes -n 'test (__changes_completion_context) = "note add"' -f -l provider -r -a '(__changes_completion_values_16)' -d 'Writable note provider'
complete -c changes -n 'test (__changes_completion_context) = "note add"' -l session -r -d 'Harness session identifier'
complete -c changes -n 'test (__changes_completion_context) = "note add"' -f -l side -r -a '(__changes_completion_values_17)' -d 'Diff side'
complete -c changes -n 'test (__changes_completion_context) = "note add"' -l staged -d 'Compare the index'
complete -c changes -n 'test (__changes_completion_context) = "note add"' -l start-line -r -d 'First line of a multi-line range'
complete -c changes -n 'test (__changes_completion_context) = "note add"' -f -l to -r -a '(__changes_completion_values_18)' -d 'Right revision'
complete -c changes -n 'test (__changes_completion_context) = "note generate"' -f -l commit -r -a '(__changes_completion_values_19)' -d 'First-parent commit comparison'
complete -c changes -n 'test (__changes_completion_context) = "note generate"' -l config -r -d 'YAML configuration file'
complete -c changes -n 'test (__changes_completion_context) = "note generate"' -f -l from -r -a '(__changes_completion_values_20)' -d 'Left revision'
complete -c changes -n 'test (__changes_completion_context) = "note generate"' -l json -d 'Print generated notes as JSON'
complete -c changes -n 'test (__changes_completion_context) = "note generate"' -f -l provider -r -a '(__changes_completion_values_21)' -d 'Note generator provider'
complete -c changes -n 'test (__changes_completion_context) = "note generate"' -l session -r -d 'Harness session identifier'
complete -c changes -n 'test (__changes_completion_context) = "note generate"' -l staged -d 'Compare the index'
complete -c changes -n 'test (__changes_completion_context) = "note generate"' -f -l store -r -a '(__changes_completion_values_22)' -d 'Writable note provider'
complete -c changes -n 'test (__changes_completion_context) = "note generate"' -f -l to -r -a '(__changes_completion_values_23)' -d 'Right revision'
complete -c changes -n 'test (__changes_completion_context) = "note list"' -f -l commit -r -a '(__changes_completion_values_24)' -d 'First-parent commit comparison'
complete -c changes -n 'test (__changes_completion_context) = "note list"' -l config -r -d 'YAML configuration file'
complete -c changes -n 'test (__changes_completion_context) = "note list"' -f -l from -r -a '(__changes_completion_values_25)' -d 'Left revision'
complete -c changes -n 'test (__changes_completion_context) = "note list"' -l json -d 'Print JSON'
complete -c changes -n 'test (__changes_completion_context) = "note list"' -f -l provider -r -a '(__changes_completion_values_26)' -d 'Note provider'
complete -c changes -n 'test (__changes_completion_context) = "note list"' -l staged -d 'Compare the index'
complete -c changes -n 'test (__changes_completion_context) = "note list"' -f -l to -r -a '(__changes_completion_values_27)' -d 'Right revision'
complete -c changes -f -n 'test (__changes_completion_context) = "provider"' -a list -d 'List configured analysis providers'
complete -c changes -f -n 'test (__changes_completion_context) = "provider"' -a validate -d 'Validate provider commands and JSON behavior'
complete -c changes -n 'test (__changes_completion_context) = "provider list"' -l config -r -d 'YAML configuration file'
complete -c changes -n 'test (__changes_completion_context) = "provider list"' -l json -d 'Print JSON'
complete -c changes -f -n 'test (__changes_completion_context) = "provider list"' -a '(__changes_completion_values_28)'
complete -c changes -n 'test (__changes_completion_context) = "provider validate"' -l config -r -d 'YAML configuration file'
complete -c changes -n 'test (__changes_completion_context) = "provider validate"' -l json -d 'Print JSON'
complete -c changes -f -n 'test (__changes_completion_context) = "provider validate"' -a '(__changes_completion_values_29)'
