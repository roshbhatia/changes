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
    printf '%s\n' 'unified' 'side-by-side'
  end | string match -rv '\t'; or true
end
function __changes_completion_values_3
  begin
    printf '%s\n' 'completion' 'difftool' 'render' 'generate' 'provider'
    command 'changes' '__values' 'repository' 2>/dev/null; or true
  end | string match -rv '\t'; or true
end
function __changes_completion_values_4
  begin
    command 'changes' '__values' 'paths' 2>/dev/null; or true
  end | string match -rv '\t'; or true
end
function __changes_completion_values_5
  begin
    printf '%s\n' 'auto' 'always' 'never'
  end | string match -rv '\t'; or true
end
function __changes_completion_values_6
  begin
    printf '%s\n' 'builtin' 'difftool'
  end | string match -rv '\t'; or true
end
function __changes_completion_values_7
  begin
    printf '%s\n' 'unified' 'side-by-side'
  end | string match -rv '\t'; or true
end
function __changes_completion_values_8
  begin
    printf '%s\n' 'auto' 'always' 'never'
  end | string match -rv '\t'; or true
end
function __changes_completion_values_9
  begin
    printf '%s\n' 'builtin' 'filter'
  end | string match -rv '\t'; or true
end
function __changes_completion_values_10
  begin
    printf '%s\n' 'unified' 'side-by-side'
  end | string match -rv '\t'; or true
end
function __changes_completion_values_11
  begin
    command 'changes' '__values' 'providers' 2>/dev/null; or true
  end | string match -rv '\t'; or true
end
function __changes_completion_values_12
  begin
    command 'changes' '__values' 'providers' 2>/dev/null; or true
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
complete -c changes -n 'test (__changes_completion_context) = ""' -l interval -r -d 'Watch interval'
complete -c changes -n 'test (__changes_completion_context) = ""' -f -l layout -r -a '(__changes_completion_values_2)' -d 'Diff layout'
complete -c changes -n 'test (__changes_completion_context) = ""' -l no-calls -d 'Skip call analysis'
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
complete -c changes -f -n 'test (__changes_completion_context) = ""' -a provider -d 'Inspect and validate analysis providers'
complete -c changes -f -n 'test (__changes_completion_context) = ""' -a '(__changes_completion_values_3)'
complete -c changes -f -n 'test (__changes_completion_context) = "completion"' -a 'bash zsh fish nu'
complete -c changes -n 'test (__changes_completion_context) = "difftool"' -f -l color -r -a '(__changes_completion_values_5)' -d 'Color output'
complete -c changes -n 'test (__changes_completion_context) = "difftool"' -l config -r -d 'YAML configuration file'
complete -c changes -n 'test (__changes_completion_context) = "difftool"' -f -l engine -r -a '(__changes_completion_values_6)' -d 'File comparison engine'
complete -c changes -n 'test (__changes_completion_context) = "difftool"' -l difftool -r -d 'Git-compatible difftool executable'
complete -c changes -n 'test (__changes_completion_context) = "difftool"' -f -l layout -r -a '(__changes_completion_values_7)' -d 'Diff layout'
complete -c changes -n 'test (__changes_completion_context) = "difftool"' -l width -r -d 'Render width'
complete -c changes -f -n 'test (__changes_completion_context) = "difftool"' -a '(__changes_completion_values_4)'
complete -c changes -n 'test (__changes_completion_context) = "render"' -f -l color -r -a '(__changes_completion_values_8)' -d 'Color output'
complete -c changes -n 'test (__changes_completion_context) = "render"' -l config -r -d 'YAML configuration file'
complete -c changes -n 'test (__changes_completion_context) = "render"' -f -l engine -r -a '(__changes_completion_values_9)' -d 'Patch display engine'
complete -c changes -n 'test (__changes_completion_context) = "render"' -l filter -r -d 'Standard-input patch filter'
complete -c changes -n 'test (__changes_completion_context) = "render"' -f -l layout -r -a '(__changes_completion_values_10)' -d 'Diff layout'
complete -c changes -n 'test (__changes_completion_context) = "render"' -l width -r -d 'Render width'
complete -c changes -n 'test (__changes_completion_context) = "generate"' -l check -d 'Fail when generated files are stale'
complete -c changes -f -n 'test (__changes_completion_context) = "provider"' -a list -d 'List configured analysis providers'
complete -c changes -f -n 'test (__changes_completion_context) = "provider"' -a validate -d 'Validate provider commands and JSON behavior'
complete -c changes -n 'test (__changes_completion_context) = "provider list"' -l config -r -d 'YAML configuration file'
complete -c changes -n 'test (__changes_completion_context) = "provider list"' -l json -d 'Print JSON'
complete -c changes -f -n 'test (__changes_completion_context) = "provider list"' -a '(__changes_completion_values_11)'
complete -c changes -n 'test (__changes_completion_context) = "provider validate"' -l config -r -d 'YAML configuration file'
complete -c changes -n 'test (__changes_completion_context) = "provider validate"' -l json -d 'Print JSON'
complete -c changes -f -n 'test (__changes_completion_context) = "provider validate"' -a '(__changes_completion_values_12)'
