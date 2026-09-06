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
  values=( 'unified' 'side-by-side')
  compadd -a values
}
__changes_completion_values_3() {
  local -a values
  values=( 'completion' 'difftool' 'render' 'generate' 'note' 'provider')
  values+=("${(@f)$('changes' '__values' 'repository' 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_4() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'paths' 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_5() {
  local -a values
  values=( 'auto' 'always' 'never')
  compadd -a values
}
__changes_completion_values_6() {
  local -a values
  values=( 'builtin' 'difftool')
  compadd -a values
}
__changes_completion_values_7() {
  local -a values
  values=( 'unified' 'side-by-side')
  compadd -a values
}
__changes_completion_values_8() {
  local -a values
  values=( 'auto' 'always' 'never')
  compadd -a values
}
__changes_completion_values_9() {
  local -a values
  values=( 'builtin' 'filter')
  compadd -a values
}
__changes_completion_values_10() {
  local -a values
  values=( 'unified' 'side-by-side')
  compadd -a values
}
__changes_completion_values_11() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'repository' 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_12() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'paths' 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_13() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'repository' 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_14() {
  local -a values
  values=( 'agent' 'user')
  compadd -a values
}
__changes_completion_values_15() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'note-writers' "${BUFFER[1,CURSOR]}" 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_16() {
  local -a values
  values=( 'left' 'right')
  compadd -a values
}
__changes_completion_values_17() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'repository' 2>/dev/null)}")
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
  values+=("${(@f)$('changes' '__values' 'repository' 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_20() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'note-readers' "${BUFFER[1,CURSOR]}" 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_21() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'repository' 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_22() {
  local -a values
  values=()
  values+=("${(@f)$('changes' '__values' 'providers' "${BUFFER[1,CURSOR]}" 2>/dev/null)}")
  compadd -a values
}
__changes_completion_values_23() {
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
      ':difftool') context='difftool' ;;
      ':render') context='render' ;;
      ':generate') context='generate' ;;
      ':note') context='note' ;;
      'note:add') context='note add' ;;
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
        '--interval[Watch interval]:value:' \
        '--layout[Diff layout]:value:__changes_completion_values_2' \
        '--no-calls[Skip call analysis]' \
        '--no-notes[Skip diff notes]' \
        '--no-symbols[Skip symbol analysis]' \
        '(-r)--recursive[Read all workspace repositories]' \
        '--root[Workspace scan root]:value:' \
        '--since[Left revision or time]:value:' \
        '--staged[Compare the index]' \
        '(-s)--stat[Show change summary]' \
        '--version[Print the Changes version]' \
        '(-w)--watch[Watch for changes]' \
        '--width[Render width]:value:' \
        '*:argument:__changes_completion_values_3'

      ;;
    'completion')
      _arguments \
        '2:shell:(bash zsh fish nu)'
      ;;
    'difftool')
      _arguments \
        '--color[Color output]:value:__changes_completion_values_5' \
        '--config[YAML configuration file]:value:' \
        '--engine[File comparison engine]:value:__changes_completion_values_6' \
        '--difftool[Git-compatible difftool executable]:value:' \
        '--layout[Diff layout]:value:__changes_completion_values_7' \
        '--width[Render width]:value:' \
        '*:argument:__changes_completion_values_4'

      ;;
    'render')
      _arguments \
        '--color[Color output]:value:__changes_completion_values_8' \
        '--config[YAML configuration file]:value:' \
        '--engine[Patch display engine]:value:__changes_completion_values_9' \
        '--filter[Standard-input patch filter]:value:' \
        '--layout[Diff layout]:value:__changes_completion_values_10' \
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
        '2:command:(add list)'

      ;;
    'note add')
      _arguments \
        '--author[Note author]:value:' \
        '--commit[First-parent commit comparison]:value:__changes_completion_values_11' \
        '--config[YAML configuration file]:value:' \
        '--file[Repository file to annotate]:value:__changes_completion_values_12' \
        '--from[Left revision]:value:__changes_completion_values_13' \
        '--json[Print the created note as JSON]' \
        '--line[Last line of the note range]:value:' \
        '--message[Summary and optional rationale]:value:' \
        '--message-file[Read note text from a file or standard input]:value:' \
        '--origin[Author kind]:value:__changes_completion_values_14' \
        '--provider[Writable note provider]:value:__changes_completion_values_15' \
        '--session[Harness session identifier]:value:' \
        '--side[Diff side]:value:__changes_completion_values_16' \
        '--staged[Compare the index]' \
        '--start-line[First line of a multi-line range]:value:' \
        '--to[Right revision]:value:__changes_completion_values_17' \
        '*:argument:'

      ;;
    'note list')
      _arguments \
        '--commit[First-parent commit comparison]:value:__changes_completion_values_18' \
        '--config[YAML configuration file]:value:' \
        '--from[Left revision]:value:__changes_completion_values_19' \
        '--json[Print JSON]' \
        '--provider[Note provider]:value:__changes_completion_values_20' \
        '--staged[Compare the index]' \
        '--to[Right revision]:value:__changes_completion_values_21' \
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
        '*:argument:__changes_completion_values_22'

      ;;
    'provider validate')
      _arguments \
        '--config[YAML configuration file]:value:' \
        '--json[Print JSON]' \
        '*:argument:__changes_completion_values_23'

      ;;
  esac
}
compdef _changes changes
