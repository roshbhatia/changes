export extern "changes" [
  --budget: string # Analysis time budget
  --color: string@"__changes_completion_values_0" # Color output
  --config: string # YAML configuration file
  --engine: string@"__changes_completion_values_1" # Patch display engine
  --filter: string # Standard-input patch filter
  --interval: string # Watch interval
  --layout: string@"__changes_completion_values_2" # Diff layout
  --no-calls # Skip call analysis
  --no-symbols # Skip symbol analysis
  --recursive(-r) # Read all workspace repositories
  --root: string # Workspace scan root
  --since: string # Left revision or time
  --staged # Compare the index
  --stat(-s) # Show change summary
  --version # Print the Changes version
  --watch(-w) # Watch for changes
  --width: string # Render width
  ...args: string@"__changes_completion_values_3"
]

export extern "changes completion" [
  shell: string@"nu-complete changes shell"
]

def "nu-complete changes shell" [] { [bash zsh fish nu] }

export extern "changes difftool" [
  --color: string@"__changes_completion_values_5" # Color output
  --config: string # YAML configuration file
  --engine: string@"__changes_completion_values_6" # File comparison engine
  --difftool: string # Git-compatible difftool executable
  --layout: string@"__changes_completion_values_7" # Diff layout
  --width: string # Render width
  ...args: string@"__changes_completion_values_4"
]

export extern "changes render" [
  --color: string@"__changes_completion_values_8" # Color output
  --config: string # YAML configuration file
  --engine: string@"__changes_completion_values_9" # Patch display engine
  --filter: string # Standard-input patch filter
  --layout: string@"__changes_completion_values_10" # Diff layout
  --width: string # Render width
  ...args: string@"__changes_completion_none"
]

export extern "changes generate" [
  --check # Fail when generated files are stale
  ...args: string@"__changes_completion_none"
]

export extern "changes provider" [
  ...args: string@"__changes_completion_none"
]

export extern "changes provider list" [
  --config: string # YAML configuration file
  --json # Print JSON
  ...args: string@"__changes_completion_values_11"
]

export extern "changes provider validate" [
  --config: string # YAML configuration file
  --json # Print JSON
  ...args: string@"__changes_completion_values_12"
]

def "__changes_completion_none" [] { [] }

def "__changes_completion_values_0" [context?: string] {
  [
    "auto"
    "always"
    "never"
  ] | flatten | uniq
}

def "__changes_completion_values_1" [context?: string] {
  [
    "builtin"
    "filter"
  ] | flatten | uniq
}

def "__changes_completion_values_2" [context?: string] {
  [
    "unified"
    "side-by-side"
  ] | flatten | uniq
}

def "__changes_completion_values_3" [context?: string] {
  [
    "completion"
    "difftool"
    "render"
    "generate"
    "provider"
    (try { run-external "changes" "__values" "repository" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_4" [context?: string] {
  [
    (try { run-external "changes" "__values" "paths" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_5" [context?: string] {
  [
    "auto"
    "always"
    "never"
  ] | flatten | uniq
}

def "__changes_completion_values_6" [context?: string] {
  [
    "builtin"
    "difftool"
  ] | flatten | uniq
}

def "__changes_completion_values_7" [context?: string] {
  [
    "unified"
    "side-by-side"
  ] | flatten | uniq
}

def "__changes_completion_values_8" [context?: string] {
  [
    "auto"
    "always"
    "never"
  ] | flatten | uniq
}

def "__changes_completion_values_9" [context?: string] {
  [
    "builtin"
    "filter"
  ] | flatten | uniq
}

def "__changes_completion_values_10" [context?: string] {
  [
    "unified"
    "side-by-side"
  ] | flatten | uniq
}

def "__changes_completion_values_11" [context?: string] {
  [
    (try { run-external "changes" "__values" "providers" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_12" [context?: string] {
  [
    (try { run-external "changes" "__values" "providers" | lines } catch { [] })
  ] | flatten | uniq
}
