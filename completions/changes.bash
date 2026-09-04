__changes_completion_values_0() {
  printf '%s\n' 'auto' 'always' 'never'
}
__changes_completion_values_1() {
  printf '%s\n' 'builtin' 'filter'
}
__changes_completion_values_2() {
  printf '%s\n' 'unified' 'side-by-side'
}
__changes_completion_values_3() {
  printf '%s\n' 'completion' 'difftool' 'render' 'generate' 'provider'
  'changes' '__values' 'repository' 2>/dev/null || true
}
__changes_completion_values_4() {
  'changes' '__values' 'paths' 2>/dev/null || true
}
__changes_completion_values_5() {
  printf '%s\n' 'auto' 'always' 'never'
}
__changes_completion_values_6() {
  printf '%s\n' 'builtin' 'difftool'
}
__changes_completion_values_7() {
  printf '%s\n' 'unified' 'side-by-side'
}
__changes_completion_values_8() {
  printf '%s\n' 'auto' 'always' 'never'
}
__changes_completion_values_9() {
  printf '%s\n' 'builtin' 'filter'
}
__changes_completion_values_10() {
  printf '%s\n' 'unified' 'side-by-side'
}
__changes_completion_values_11() {
  'changes' '__values' 'providers' 2>/dev/null || true
}
__changes_completion_values_12() {
  'changes' '__values' 'providers' 2>/dev/null || true
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
      ':provider') context='provider' ;;
      'provider:list') context='provider list' ;;
      'provider:validate') context='provider validate' ;;
    esac
  done
  case "$context:$previous" in
    ':--color') __changes_completion_filter "$current" < <(__changes_completion_values_0); return ;;
    ':--engine') __changes_completion_filter "$current" < <(__changes_completion_values_1); return ;;
    ':--layout') __changes_completion_filter "$current" < <(__changes_completion_values_2); return ;;
    'difftool:--color') __changes_completion_filter "$current" < <(__changes_completion_values_5); return ;;
    'difftool:--engine') __changes_completion_filter "$current" < <(__changes_completion_values_6); return ;;
    'difftool:--layout') __changes_completion_filter "$current" < <(__changes_completion_values_7); return ;;
    'render:--color') __changes_completion_filter "$current" < <(__changes_completion_values_8); return ;;
    'render:--engine') __changes_completion_filter "$current" < <(__changes_completion_values_9); return ;;
    'render:--layout') __changes_completion_filter "$current" < <(__changes_completion_values_10); return ;;
  esac
  case "$context:$current" in
    ':--color='*) __changes_completion_filter "${current#*=}" "--color=" < <(__changes_completion_values_0); return ;;
    ':--engine='*) __changes_completion_filter "${current#*=}" "--engine=" < <(__changes_completion_values_1); return ;;
    ':--layout='*) __changes_completion_filter "${current#*=}" "--layout=" < <(__changes_completion_values_2); return ;;
    'difftool:--color='*) __changes_completion_filter "${current#*=}" "--color=" < <(__changes_completion_values_5); return ;;
    'difftool:--engine='*) __changes_completion_filter "${current#*=}" "--engine=" < <(__changes_completion_values_6); return ;;
    'difftool:--layout='*) __changes_completion_filter "${current#*=}" "--layout=" < <(__changes_completion_values_7); return ;;
    'render:--color='*) __changes_completion_filter "${current#*=}" "--color=" < <(__changes_completion_values_8); return ;;
    'render:--engine='*) __changes_completion_filter "${current#*=}" "--engine=" < <(__changes_completion_values_9); return ;;
    'render:--layout='*) __changes_completion_filter "${current#*=}" "--layout=" < <(__changes_completion_values_10); return ;;
  esac
  case "$context" in
    '')
      __changes_completion_filter "$current" < <(
        printf '%s\n' 'completion' 'difftool' 'render' 'generate' 'provider' '--budget' '--color' '--config' '--engine' '--filter' '--interval' '--layout' '--no-calls' '--no-symbols' '--recursive' '-r' '--root' '--since' '--staged' '--stat' '-s' '--version' '--watch' '-w' '--width'
        __changes_completion_values_3
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
        __changes_completion_values_4
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
    'provider')
      __changes_completion_filter "$current" < <(
        printf '%s\n' 'list' 'validate'
      )
      ;;
    'provider list')
      __changes_completion_filter "$current" < <(
        printf '%s\n' '--config' '--json'
        __changes_completion_values_11
      )
      ;;
    'provider validate')
      __changes_completion_filter "$current" < <(
        printf '%s\n' '--config' '--json'
        __changes_completion_values_12
      )
      ;;
  esac
}
complete -F _changes_complete changes
