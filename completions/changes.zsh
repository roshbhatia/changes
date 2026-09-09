#compdef changes
__changes_completion_values_0() {
  local -a values
  values=( 'auto' 'always' 'never')
  compadd -a values
}
__changes_completion_values_1() {
  local -a values
  values=( 'builtin' 'filter')
  compadd -a values
}
__changes_completion_values_2() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'group-providers' "${BUFFER[1,CURSOR]}" 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_3() {
  local -a values
  values=( 'unified' 'side-by-side')
  compadd -a values
}
__changes_completion_values_4() {
  local -a values
  values=( 'completion' 'interactive' 'workspace' 'difftool' 'render' 'generate' 'note' 'provider')
  values+=("${(@f)$('changes' '__values' 'repository' 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_5() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'repository' 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_6() {
  local -a values
  values=( 'unified' 'side-by-side')
  compadd -a values
}
__changes_completion_values_7() {
  local -a values
  values=( 'working' 'staged' 'commit')
  compadd -a values
}
__changes_completion_values_8() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'repository' 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_9() {
  local -a values
  values=( 'unified' 'side-by-side')
  compadd -a values
}
__changes_completion_values_10() {
  local -a values
  values=( 'working' 'staged' 'commit')
  compadd -a values
}
__changes_completion_values_11() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'paths' 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_12() {
  local -a values
  values=( 'auto' 'always' 'never')
  compadd -a values
}
__changes_completion_values_13() {
  local -a values
  values=( 'builtin' 'difftool')
  compadd -a values
}
__changes_completion_values_14() {
  local -a values
  values=( 'unified' 'side-by-side')
  compadd -a values
}
__changes_completion_values_15() {
  local -a values
  values=( 'auto' 'always' 'never')
  compadd -a values
}
__changes_completion_values_16() {
  local -a values
  values=( 'builtin' 'filter')
  compadd -a values
}
__changes_completion_values_17() {
  local -a values
  values=( 'unified' 'side-by-side')
  compadd -a values
}
__changes_completion_values_18() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'repository' 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_19() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'paths' 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_20() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'repository' 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_21() {
  local -a values
  values=( 'agent' 'user')
  compadd -a values
}
__changes_completion_values_22() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'note-writers' "${BUFFER[1,CURSOR]}" 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_23() {
  local -a values
  values=( 'left' 'right')
  compadd -a values
}
__changes_completion_values_24() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'repository' 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_25() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'repository' 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_26() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'repository' 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_27() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'note-generators' "${BUFFER[1,CURSOR]}" 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_28() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'note-writers' "${BUFFER[1,CURSOR]}" 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_29() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'repository' 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_30() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'repository' 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_31() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'repository' 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_32() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'note-readers' "${BUFFER[1,CURSOR]}" 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_33() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'repository' 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_34() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'providers' "${BUFFER[1,CURSOR]}" 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_35() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'providers' "${BUFFER[1,CURSOR]}" 2>/dev/null)}")
  compadd -a values
}

_changes() {
  local context=''
  local word
  local consume_value=0
  local options_done=0
  for word in ${words[2,$((CURRENT - 1))]}; do
    if (( consume_value )); then
      consume_value=0
      continue
    fi
    if (( options_done )); then
      continue
    fi
    if [[ "$word" == '--' ]]; then
      options_done=1
      continue
    fi
    case "$context:$word" in
      ':--budget') consume_value=1; continue ;;
      ':--budget='*) continue ;;
      ':--color') consume_value=1; continue ;;
      ':--color='*) continue ;;
      ':--config') consume_value=1; continue ;;
      ':--config='*) continue ;;
      ':--engine') consume_value=1; continue ;;
      ':--engine='*) continue ;;
      ':--filter') consume_value=1; continue ;;
      ':--filter='*) continue ;;
      ':--group-provider') consume_value=1; continue ;;
      ':--group-provider='*) continue ;;
      ':--interval') consume_value=1; continue ;;
      ':--interval='*) continue ;;
      ':--layout') consume_value=1; continue ;;
      ':--layout='*) continue ;;
      ':--root') consume_value=1; continue ;;
      ':--root='*) continue ;;
      ':--since') consume_value=1; continue ;;
      ':--since='*) continue ;;
      ':--width') consume_value=1; continue ;;
      ':--width='*) continue ;;
      'interactive:--commit') consume_value=1; continue ;;
      'interactive:--commit='*) continue ;;
      'interactive:--config') consume_value=1; continue ;;
      'interactive:--config='*) continue ;;
      'interactive:--history-limit') consume_value=1; continue ;;
      'interactive:--history-limit='*) continue ;;
      'interactive:--layout') consume_value=1; continue ;;
      'interactive:--layout='*) continue ;;
      'interactive:--view') consume_value=1; continue ;;
      'interactive:--view='*) continue ;;
      'interactive:--width') consume_value=1; continue ;;
      'interactive:--width='*) continue ;;
      'workspace:--commit') consume_value=1; continue ;;
      'workspace:--commit='*) continue ;;
      'workspace:--config') consume_value=1; continue ;;
      'workspace:--config='*) continue ;;
      'workspace:--history-limit') consume_value=1; continue ;;
      'workspace:--history-limit='*) continue ;;
      'workspace:--layout') consume_value=1; continue ;;
      'workspace:--layout='*) continue ;;
      'workspace:--view') consume_value=1; continue ;;
      'workspace:--view='*) continue ;;
      'workspace:--width') consume_value=1; continue ;;
      'workspace:--width='*) continue ;;
      'workspace:--interval') consume_value=1; continue ;;
      'workspace:--interval='*) continue ;;
      'difftool:--color') consume_value=1; continue ;;
      'difftool:--color='*) continue ;;
      'difftool:--config') consume_value=1; continue ;;
      'difftool:--config='*) continue ;;
      'difftool:--engine') consume_value=1; continue ;;
      'difftool:--engine='*) continue ;;
      'difftool:--difftool') consume_value=1; continue ;;
      'difftool:--difftool='*) continue ;;
      'difftool:--layout') consume_value=1; continue ;;
      'difftool:--layout='*) continue ;;
      'difftool:--width') consume_value=1; continue ;;
      'difftool:--width='*) continue ;;
      'render:--color') consume_value=1; continue ;;
      'render:--color='*) continue ;;
      'render:--config') consume_value=1; continue ;;
      'render:--config='*) continue ;;
      'render:--engine') consume_value=1; continue ;;
      'render:--engine='*) continue ;;
      'render:--filter') consume_value=1; continue ;;
      'render:--filter='*) continue ;;
      'render:--layout') consume_value=1; continue ;;
      'render:--layout='*) continue ;;
      'render:--width') consume_value=1; continue ;;
      'render:--width='*) continue ;;
      'note add:--author') consume_value=1; continue ;;
      'note add:--author='*) continue ;;
      'note add:--commit') consume_value=1; continue ;;
      'note add:--commit='*) continue ;;
      'note add:--config') consume_value=1; continue ;;
      'note add:--config='*) continue ;;
      'note add:--expected-file-sha256') consume_value=1; continue ;;
      'note add:--expected-file-sha256='*) continue ;;
      'note add:--file') consume_value=1; continue ;;
      'note add:--file='*) continue ;;
      'note add:--from') consume_value=1; continue ;;
      'note add:--from='*) continue ;;
      'note add:--line') consume_value=1; continue ;;
      'note add:--line='*) continue ;;
      'note add:--message') consume_value=1; continue ;;
      'note add:--message='*) continue ;;
      'note add:--message-file') consume_value=1; continue ;;
      'note add:--message-file='*) continue ;;
      'note add:--origin') consume_value=1; continue ;;
      'note add:--origin='*) continue ;;
      'note add:--provider') consume_value=1; continue ;;
      'note add:--provider='*) continue ;;
      'note add:--session') consume_value=1; continue ;;
      'note add:--session='*) continue ;;
      'note add:--side') consume_value=1; continue ;;
      'note add:--side='*) continue ;;
      'note add:--start-line') consume_value=1; continue ;;
      'note add:--start-line='*) continue ;;
      'note add:--to') consume_value=1; continue ;;
      'note add:--to='*) continue ;;
      'note generate:--commit') consume_value=1; continue ;;
      'note generate:--commit='*) continue ;;
      'note generate:--config') consume_value=1; continue ;;
      'note generate:--config='*) continue ;;
      'note generate:--from') consume_value=1; continue ;;
      'note generate:--from='*) continue ;;
      'note generate:--provider') consume_value=1; continue ;;
      'note generate:--provider='*) continue ;;
      'note generate:--session') consume_value=1; continue ;;
      'note generate:--session='*) continue ;;
      'note generate:--store') consume_value=1; continue ;;
      'note generate:--store='*) continue ;;
      'note generate:--to') consume_value=1; continue ;;
      'note generate:--to='*) continue ;;
      'note list:--commit') consume_value=1; continue ;;
      'note list:--commit='*) continue ;;
      'note list:--config') consume_value=1; continue ;;
      'note list:--config='*) continue ;;
      'note list:--from') consume_value=1; continue ;;
      'note list:--from='*) continue ;;
      'note list:--provider') consume_value=1; continue ;;
      'note list:--provider='*) continue ;;
      'note list:--to') consume_value=1; continue ;;
      'note list:--to='*) continue ;;
      'provider list:--config') consume_value=1; continue ;;
      'provider list:--config='*) continue ;;
      'provider validate:--config') consume_value=1; continue ;;
      'provider validate:--config='*) continue ;;
    esac
    case "$context:$word" in
      ':completion') context='completion' ;;
      ':interactive') context='interactive' ;;
      ':workspace') context='workspace' ;;
      ':difftool') context='difftool' ;;
      ':render') context='render' ;;
      ':generate') context='generate' ;;
      ':note') context='note' ;;
      'note:add') context='note add' ;;
      'note:generate') context='note generate' ;;
      'note:list') context='note list' ;;
      ':provider') context='provider' ;;
      'provider:list') context='provider list' ;;
      'provider:validate') context='provider validate' ;;
    esac
  done
  case "$context" in
    '')
      _arguments \
        '--budget[Analysis time budget]:value:' \
        '--color[Color output]:value:__changes_completion_values_0' \
        '--config[YAML configuration file]:value:' \
        '--engine[Patch display engine]:value:__changes_completion_values_1' \
        '--filter[Standard-input patch filter]:value:' \
        '--group-provider[Logical change-group provider]:value:__changes_completion_values_2' \
        '--interval[Watch interval]:value:' \
        '--layout[Diff layout]:value:__changes_completion_values_3' \
        '--no-calls[Skip call analysis]' \
        '--no-groups[Skip logical change grouping]' \
        '--no-notes[Skip diff notes]' \
        '--no-symbols[Skip symbol analysis]' \
        '--quiet[Disable progress output]' \
        '(-r)--recursive[Read all workspace repositories]' \
        '--root[Workspace scan root]:value:' \
        '--since[Left revision or time]:value:' \
        '--staged[Compare the index]' \
        '(-s)--stat[Show change summary]' \
        '--version[Print the Changes and provider spec versions]' \
        '(-w)--watch[Watch for changes]' \
        '--width[Render width]:value:' \
        '*:argument:__changes_completion_values_4'

      ;;
    'completion')
      _arguments \
        '2:shell:(bash zsh fish nu)'
      ;;
    'interactive')
      _arguments \
        '--commit[Commit to compare with its first parent]:value:__changes_completion_values_5' \
        '--config[YAML configuration file]:value:' \
        '--history-limit[Commit history limit]:value:' \
        '--layout[Diff layout]:value:__changes_completion_values_6' \
        '--no-calls[Skip call analysis]' \
        '--no-groups[Skip logical change grouping]' \
        '--no-notes[Skip diff notes]' \
        '--no-symbols[Skip symbol analysis]' \
        '--quiet[Disable progress output]' \
        '--refresh[Bypass cached workspace and provider results]' \
        '--view[Git comparison view]:value:__changes_completion_values_7' \
        '--width[Render width]:value:' \
        '*:argument:'

      ;;
    'workspace')
      _arguments \
        '--commit[Commit to compare with its first parent]:value:__changes_completion_values_8' \
        '--config[YAML configuration file]:value:' \
        '--history-limit[Commit history limit]:value:' \
        '--layout[Diff layout]:value:__changes_completion_values_9' \
        '--no-calls[Skip call analysis]' \
        '--no-groups[Skip logical change grouping]' \
        '--no-notes[Skip diff notes]' \
        '--no-symbols[Skip symbol analysis]' \
        '--quiet[Disable progress output]' \
        '--refresh[Bypass cached workspace and provider results]' \
        '--view[Git comparison view]:value:__changes_completion_values_10' \
        '--width[Render width]:value:' \
        '--interval[Watch interval]:value:' \
        '--watch[Emit JSON Lines refresh events]' \
        '*:argument:'

      ;;
    'difftool')
      _arguments \
        '--color[Color output]:value:__changes_completion_values_12' \
        '--config[YAML configuration file]:value:' \
        '--engine[File comparison engine]:value:__changes_completion_values_13' \
        '--difftool[Git-compatible difftool executable]:value:' \
        '--layout[Diff layout]:value:__changes_completion_values_14' \
        '--width[Render width]:value:' \
        '*:argument:__changes_completion_values_11'

      ;;
    'render')
      _arguments \
        '--color[Color output]:value:__changes_completion_values_15' \
        '--config[YAML configuration file]:value:' \
        '--engine[Patch display engine]:value:__changes_completion_values_16' \
        '--filter[Standard-input patch filter]:value:' \
        '--layout[Diff layout]:value:__changes_completion_values_17' \
        '--width[Render width]:value:' \
        '*:argument:'

      ;;
    'generate')
      _arguments \
        '--check[Fail when generated files are stale]' \
        '*:argument:'

      ;;
    'note')
      _arguments \
        '2:command:(add generate list)'

      ;;
    'note add')
      _arguments \
        '--author[Note author]:value:' \
        '--commit[First-parent commit comparison]:value:__changes_completion_values_18' \
        '--config[YAML configuration file]:value:' \
        '--expected-file-sha256[Require the selected file side to match this SHA-256 digest]:value:' \
        '--file[Repository file to annotate]:value:__changes_completion_values_19' \
        '--from[Left revision]:value:__changes_completion_values_20' \
        '--json[Print the created note as JSON]' \
        '--line[Last line of the note range]:value:' \
        '--message[Summary and optional rationale]:value:' \
        '--message-file[Read note text from a file or standard input]:value:' \
        '--origin[Author kind]:value:__changes_completion_values_21' \
        '--provider[Writable note provider]:value:__changes_completion_values_22' \
        '--session[Harness session identifier]:value:' \
        '--side[Diff side]:value:__changes_completion_values_23' \
        '--staged[Compare the index]' \
        '--start-line[First line of a multi-line range]:value:' \
        '--to[Right revision]:value:__changes_completion_values_24' \
        '*:argument:'

      ;;
    'note generate')
      _arguments \
        '--commit[First-parent commit comparison; repeat for several commits]:value:__changes_completion_values_25' \
        '--config[YAML configuration file]:value:' \
        '--draft[Return validated drafts without writing; requires --json]' \
        '--from[Left revision]:value:__changes_completion_values_26' \
        '--json[Print generated notes as JSON]' \
        '--provider[Note generator provider]:value:__changes_completion_values_27' \
        '--session[Harness session identifier]:value:' \
        '--staged[Compare the index]' \
        '--store[Writable note provider]:value:__changes_completion_values_28' \
        '--to[Right revision]:value:__changes_completion_values_29' \
        '*:argument:'

      ;;
    'note list')
      _arguments \
        '--commit[First-parent commit comparison]:value:__changes_completion_values_30' \
        '--config[YAML configuration file]:value:' \
        '--from[Left revision]:value:__changes_completion_values_31' \
        '--json[Print JSON]' \
        '--provider[Note provider]:value:__changes_completion_values_32' \
        '--staged[Compare the index]' \
        '--to[Right revision]:value:__changes_completion_values_33' \
        '*:argument:'

      ;;
    'provider')
      _arguments \
        '2:command:(list validate)'

      ;;
    'provider list')
      _arguments \
        '--config[YAML configuration file]:value:' \
        '--json[Print JSON]' \
        '*:argument:__changes_completion_values_34'

      ;;
    'provider validate')
      _arguments \
        '--config[YAML configuration file]:value:' \
        '--json[Print JSON]' \
        '*:argument:__changes_completion_values_35'

      ;;
  esac
}
compdef _changes changes
