__changes_completion_values_0() {
  printf '%s\n' 'auto' 'always' 'never'
}
__changes_completion_values_1() {
  printf '%s\n' 'builtin' 'filter'
}
__changes_completion_values_2() {
  'changes' '__values' 'group-providers' "${COMP_LINE:0:COMP_POINT}" 2>/dev/null || true
}
__changes_completion_values_3() {
  printf '%s\n' 'unified' 'side-by-side'
}
__changes_completion_values_4() {
  printf '%s\n' 'completion' 'difftool' 'render' 'generate' 'note' 'provider'
  'changes' '__values' 'repository' 2>/dev/null || true
}
__changes_completion_values_5() {
  'changes' '__values' 'paths' 2>/dev/null || true
}
__changes_completion_values_6() {
  printf '%s\n' 'auto' 'always' 'never'
}
__changes_completion_values_7() {
  printf '%s\n' 'builtin' 'difftool'
}
__changes_completion_values_8() {
  printf '%s\n' 'unified' 'side-by-side'
}
__changes_completion_values_9() {
  printf '%s\n' 'auto' 'always' 'never'
}
__changes_completion_values_10() {
  printf '%s\n' 'builtin' 'filter'
}
__changes_completion_values_11() {
  printf '%s\n' 'unified' 'side-by-side'
}
__changes_completion_values_12() {
  'changes' '__values' 'repository' 2>/dev/null || true
}
__changes_completion_values_13() {
  'changes' '__values' 'paths' 2>/dev/null || true
}
__changes_completion_values_14() {
  'changes' '__values' 'repository' 2>/dev/null || true
}
__changes_completion_values_15() {
  printf '%s\n' 'agent' 'user'
}
__changes_completion_values_16() {
  'changes' '__values' 'note-writers' "${COMP_LINE:0:COMP_POINT}" 2>/dev/null || true
}
__changes_completion_values_17() {
  printf '%s\n' 'left' 'right'
}
__changes_completion_values_18() {
  'changes' '__values' 'repository' 2>/dev/null || true
}
__changes_completion_values_19() {
  'changes' '__values' 'repository' 2>/dev/null || true
}
__changes_completion_values_20() {
  'changes' '__values' 'repository' 2>/dev/null || true
}
__changes_completion_values_21() {
  'changes' '__values' 'note-generators' "${COMP_LINE:0:COMP_POINT}" 2>/dev/null || true
}
__changes_completion_values_22() {
  'changes' '__values' 'note-writers' "${COMP_LINE:0:COMP_POINT}" 2>/dev/null || true
}
__changes_completion_values_23() {
  'changes' '__values' 'repository' 2>/dev/null || true
}
__changes_completion_values_24() {
  'changes' '__values' 'repository' 2>/dev/null || true
}
__changes_completion_values_25() {
  'changes' '__values' 'repository' 2>/dev/null || true
}
__changes_completion_values_26() {
  'changes' '__values' 'note-readers' "${COMP_LINE:0:COMP_POINT}" 2>/dev/null || true
}
__changes_completion_values_27() {
  'changes' '__values' 'repository' 2>/dev/null || true
}
__changes_completion_values_28() {
  'changes' '__values' 'providers' "${COMP_LINE:0:COMP_POINT}" 2>/dev/null || true
}
__changes_completion_values_29() {
  'changes' '__values' 'providers' "${COMP_LINE:0:COMP_POINT}" 2>/dev/null || true
}
__changes_completion_filter() {
  local prefix="$1"
  local prepend="${2-}"
  local candidate
  local existing
  local duplicate
  COMPREPLY=()
  while IFS= read -r candidate || [[ -n "$candidate" ]]; do
    [[ "$candidate" == "$prefix"* ]] || continue
    candidate="$prepend$candidate"
    duplicate=0
    for existing in "${COMPREPLY[@]}"; do
      if [[ "$existing" == "$candidate" ]]; then
        duplicate=1
        break
      fi
    done
    (( duplicate )) || COMPREPLY+=("$candidate")
  done
}

_changes_complete() {
  local current="${COMP_WORDS[COMP_CWORD]}"
  local previous=""
  local context=""
  local word
  local index
  local consume_value=0
  local options_done=0
  if (( COMP_CWORD > 0 )); then
    previous="${COMP_WORDS[COMP_CWORD-1]}"
  fi
  for ((index=1; index<COMP_CWORD; index++)); do
    word="${COMP_WORDS[index]}"
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
  case "$context:$previous" in
    ':--color') __changes_completion_filter "$current" < <(__changes_completion_values_0); return ;;
    ':--engine') __changes_completion_filter "$current" < <(__changes_completion_values_1); return ;;
    ':--group-provider') __changes_completion_filter "$current" < <(__changes_completion_values_2); return ;;
    ':--layout') __changes_completion_filter "$current" < <(__changes_completion_values_3); return ;;
    'difftool:--color') __changes_completion_filter "$current" < <(__changes_completion_values_6); return ;;
    'difftool:--engine') __changes_completion_filter "$current" < <(__changes_completion_values_7); return ;;
    'difftool:--layout') __changes_completion_filter "$current" < <(__changes_completion_values_8); return ;;
    'render:--color') __changes_completion_filter "$current" < <(__changes_completion_values_9); return ;;
    'render:--engine') __changes_completion_filter "$current" < <(__changes_completion_values_10); return ;;
    'render:--layout') __changes_completion_filter "$current" < <(__changes_completion_values_11); return ;;
    'note add:--commit') __changes_completion_filter "$current" < <(__changes_completion_values_12); return ;;
    'note add:--file') __changes_completion_filter "$current" < <(__changes_completion_values_13); return ;;
    'note add:--from') __changes_completion_filter "$current" < <(__changes_completion_values_14); return ;;
    'note add:--origin') __changes_completion_filter "$current" < <(__changes_completion_values_15); return ;;
    'note add:--provider') __changes_completion_filter "$current" < <(__changes_completion_values_16); return ;;
    'note add:--side') __changes_completion_filter "$current" < <(__changes_completion_values_17); return ;;
    'note add:--to') __changes_completion_filter "$current" < <(__changes_completion_values_18); return ;;
    'note generate:--commit') __changes_completion_filter "$current" < <(__changes_completion_values_19); return ;;
    'note generate:--from') __changes_completion_filter "$current" < <(__changes_completion_values_20); return ;;
    'note generate:--provider') __changes_completion_filter "$current" < <(__changes_completion_values_21); return ;;
    'note generate:--store') __changes_completion_filter "$current" < <(__changes_completion_values_22); return ;;
    'note generate:--to') __changes_completion_filter "$current" < <(__changes_completion_values_23); return ;;
    'note list:--commit') __changes_completion_filter "$current" < <(__changes_completion_values_24); return ;;
    'note list:--from') __changes_completion_filter "$current" < <(__changes_completion_values_25); return ;;
    'note list:--provider') __changes_completion_filter "$current" < <(__changes_completion_values_26); return ;;
    'note list:--to') __changes_completion_filter "$current" < <(__changes_completion_values_27); return ;;
  esac
  case "$context:$current" in
    ':--color='*) __changes_completion_filter "${current#*=}" "--color=" < <(__changes_completion_values_0); return ;;
    ':--engine='*) __changes_completion_filter "${current#*=}" "--engine=" < <(__changes_completion_values_1); return ;;
    ':--group-provider='*) __changes_completion_filter "${current#*=}" "--group-provider=" < <(__changes_completion_values_2); return ;;
    ':--layout='*) __changes_completion_filter "${current#*=}" "--layout=" < <(__changes_completion_values_3); return ;;
    'difftool:--color='*) __changes_completion_filter "${current#*=}" "--color=" < <(__changes_completion_values_6); return ;;
    'difftool:--engine='*) __changes_completion_filter "${current#*=}" "--engine=" < <(__changes_completion_values_7); return ;;
    'difftool:--layout='*) __changes_completion_filter "${current#*=}" "--layout=" < <(__changes_completion_values_8); return ;;
    'render:--color='*) __changes_completion_filter "${current#*=}" "--color=" < <(__changes_completion_values_9); return ;;
    'render:--engine='*) __changes_completion_filter "${current#*=}" "--engine=" < <(__changes_completion_values_10); return ;;
    'render:--layout='*) __changes_completion_filter "${current#*=}" "--layout=" < <(__changes_completion_values_11); return ;;
    'note add:--commit='*) __changes_completion_filter "${current#*=}" "--commit=" < <(__changes_completion_values_12); return ;;
    'note add:--file='*) __changes_completion_filter "${current#*=}" "--file=" < <(__changes_completion_values_13); return ;;
    'note add:--from='*) __changes_completion_filter "${current#*=}" "--from=" < <(__changes_completion_values_14); return ;;
    'note add:--origin='*) __changes_completion_filter "${current#*=}" "--origin=" < <(__changes_completion_values_15); return ;;
    'note add:--provider='*) __changes_completion_filter "${current#*=}" "--provider=" < <(__changes_completion_values_16); return ;;
    'note add:--side='*) __changes_completion_filter "${current#*=}" "--side=" < <(__changes_completion_values_17); return ;;
    'note add:--to='*) __changes_completion_filter "${current#*=}" "--to=" < <(__changes_completion_values_18); return ;;
    'note generate:--commit='*) __changes_completion_filter "${current#*=}" "--commit=" < <(__changes_completion_values_19); return ;;
    'note generate:--from='*) __changes_completion_filter "${current#*=}" "--from=" < <(__changes_completion_values_20); return ;;
    'note generate:--provider='*) __changes_completion_filter "${current#*=}" "--provider=" < <(__changes_completion_values_21); return ;;
    'note generate:--store='*) __changes_completion_filter "${current#*=}" "--store=" < <(__changes_completion_values_22); return ;;
    'note generate:--to='*) __changes_completion_filter "${current#*=}" "--to=" < <(__changes_completion_values_23); return ;;
    'note list:--commit='*) __changes_completion_filter "${current#*=}" "--commit=" < <(__changes_completion_values_24); return ;;
    'note list:--from='*) __changes_completion_filter "${current#*=}" "--from=" < <(__changes_completion_values_25); return ;;
    'note list:--provider='*) __changes_completion_filter "${current#*=}" "--provider=" < <(__changes_completion_values_26); return ;;
    'note list:--to='*) __changes_completion_filter "${current#*=}" "--to=" < <(__changes_completion_values_27); return ;;
  esac
  case "$context" in
    '')
      __changes_completion_filter "$current" < <(
        printf '%s\n' 'completion' 'difftool' 'render' 'generate' 'note' 'provider' '--budget' '--color' '--config' '--engine' '--filter' '--group-provider' '--interval' '--layout' '--no-calls' '--no-groups' '--no-notes' '--no-symbols' '--recursive' '-r' '--root' '--since' '--staged' '--stat' '-s' '--version' '--watch' '-w' '--width'
        __changes_completion_values_4
      )
      ;;
    'completion')
      __changes_completion_filter "$current" < <(
        printf '%s\n' 'bash' 'zsh' 'fish' 'nu'
      )
      ;;
    'difftool')
      __changes_completion_filter "$current" < <(
        printf '%s\n' '--color' '--config' '--engine' '--difftool' '--layout' '--width'
        __changes_completion_values_5
      )
      ;;
    'render')
      __changes_completion_filter "$current" < <(
        printf '%s\n' '--color' '--config' '--engine' '--filter' '--layout' '--width'
      )
      ;;
    'generate')
      __changes_completion_filter "$current" < <(
        printf '%s\n' '--check'
      )
      ;;
    'note')
      __changes_completion_filter "$current" < <(
        printf '%s\n' 'add' 'generate' 'list'
      )
      ;;
    'note add')
      __changes_completion_filter "$current" < <(
        printf '%s\n' '--author' '--commit' '--config' '--file' '--from' '--json' '--line' '--message' '--message-file' '--origin' '--provider' '--session' '--side' '--staged' '--start-line' '--to'
      )
      ;;
    'note generate')
      __changes_completion_filter "$current" < <(
        printf '%s\n' '--commit' '--config' '--from' '--json' '--provider' '--session' '--staged' '--store' '--to'
      )
      ;;
    'note list')
      __changes_completion_filter "$current" < <(
        printf '%s\n' '--commit' '--config' '--from' '--json' '--provider' '--staged' '--to'
      )
      ;;
    'provider')
      __changes_completion_filter "$current" < <(
        printf '%s\n' 'list' 'validate'
      )
      ;;
    'provider list')
      __changes_completion_filter "$current" < <(
        printf '%s\n' '--config' '--json'
        __changes_completion_values_28
      )
      ;;
    'provider validate')
      __changes_completion_filter "$current" < <(
        printf '%s\n' '--config' '--json'
        __changes_completion_values_29
      )
      ;;
  esac
}
complete -F _changes_complete changes
