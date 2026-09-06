export extern "changes" [
  --budget: string # Analysis time budget
  --color: string@"__changes_completion_values_0" # Color output
  --config: string # YAML configuration file
  --engine: string@"__changes_completion_values_1" # Patch display engine
  --filter: string # Standard-input patch filter
  --group-provider: string@"__changes_completion_values_2" # Logical change-group provider
  --interval: string # Watch interval
  --layout: string@"__changes_completion_values_3" # Diff layout
  --no-calls # Skip call analysis
  --no-groups # Skip logical change grouping
  --no-notes # Skip diff notes
  --no-symbols # Skip symbol analysis
  --recursive(-r) # Read all workspace repositories
  --root: string # Workspace scan root
  --since: string # Left revision or time
  --staged # Compare the index
  --stat(-s) # Show change summary
  --version # Print the Changes version
  --watch(-w) # Watch for changes
  --width: string # Render width
  ...args: string@"__changes_completion_values_4"
]

export extern "changes completion" [
  shell: string@"nu-complete changes shell"
]

def "nu-complete changes shell" [] { [bash zsh fish nu] }

export extern "changes difftool" [
  --color: string@"__changes_completion_values_6" # Color output
  --config: string # YAML configuration file
  --engine: string@"__changes_completion_values_7" # File comparison engine
  --difftool: string # Git-compatible difftool executable
  --layout: string@"__changes_completion_values_8" # Diff layout
  --width: string # Render width
  ...args: string@"__changes_completion_values_5"
]

export extern "changes render" [
  --color: string@"__changes_completion_values_9" # Color output
  --config: string # YAML configuration file
  --engine: string@"__changes_completion_values_10" # Patch display engine
  --filter: string # Standard-input patch filter
  --layout: string@"__changes_completion_values_11" # Diff layout
  --width: string # Render width
  ...args: string@"__changes_completion_none"
]

export extern "changes generate" [
  --check # Fail when generated files are stale
  ...args: string@"__changes_completion_none"
]

export extern "changes note" [
  ...args: string@"__changes_completion_none"
]

export extern "changes note add" [
  --author: string # Note author
  --commit: string@"__changes_completion_values_12" # First-parent commit comparison
  --config: string # YAML configuration file
  --file: string@"__changes_completion_values_13" # Repository file to annotate
  --from: string@"__changes_completion_values_14" # Left revision
  --json # Print the created note as JSON
  --line: string # Last line of the note range
  --message: string # Summary and optional rationale
  --message-file: string # Read note text from a file or standard input
  --origin: string@"__changes_completion_values_15" # Author kind
  --provider: string@"__changes_completion_values_16" # Writable note provider
  --session: string # Harness session identifier
  --side: string@"__changes_completion_values_17" # Diff side
  --staged # Compare the index
  --start-line: string # First line of a multi-line range
  --to: string@"__changes_completion_values_18" # Right revision
  ...args: string@"__changes_completion_none"
]

export extern "changes note generate" [
  --commit: string@"__changes_completion_values_19" # First-parent commit comparison
  --config: string # YAML configuration file
  --from: string@"__changes_completion_values_20" # Left revision
  --json # Print generated notes as JSON
  --provider: string@"__changes_completion_values_21" # Note generator provider
  --session: string # Harness session identifier
  --staged # Compare the index
  --store: string@"__changes_completion_values_22" # Writable note provider
  --to: string@"__changes_completion_values_23" # Right revision
  ...args: string@"__changes_completion_none"
]

export extern "changes note list" [
  --commit: string@"__changes_completion_values_24" # First-parent commit comparison
  --config: string # YAML configuration file
  --from: string@"__changes_completion_values_25" # Left revision
  --json # Print JSON
  --provider: string@"__changes_completion_values_26" # Note provider
  --staged # Compare the index
  --to: string@"__changes_completion_values_27" # Right revision
  ...args: string@"__changes_completion_none"
]

export extern "changes provider" [
  ...args: string@"__changes_completion_none"
]

export extern "changes provider list" [
  --config: string # YAML configuration file
  --json # Print JSON
  ...args: string@"__changes_completion_values_28"
]

export extern "changes provider validate" [
  --config: string # YAML configuration file
  --json # Print JSON
  ...args: string@"__changes_completion_values_29"
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
    (try { run-external "changes" "__values" "group-providers" ($context | default "") | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_3" [context?: string] {
  [
    "unified"
    "side-by-side"
  ] | flatten | uniq
}

def "__changes_completion_values_4" [context?: string] {
  [
    "completion"
    "difftool"
    "render"
    "generate"
    "note"
    "provider"
    (try { run-external "changes" "__values" "repository" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_5" [context?: string] {
  [
    (try { run-external "changes" "__values" "paths" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_6" [context?: string] {
  [
    "auto"
    "always"
    "never"
  ] | flatten | uniq
}

def "__changes_completion_values_7" [context?: string] {
  [
    "builtin"
    "difftool"
  ] | flatten | uniq
}

def "__changes_completion_values_8" [context?: string] {
  [
    "unified"
    "side-by-side"
  ] | flatten | uniq
}

def "__changes_completion_values_9" [context?: string] {
  [
    "auto"
    "always"
    "never"
  ] | flatten | uniq
}

def "__changes_completion_values_10" [context?: string] {
  [
    "builtin"
    "filter"
  ] | flatten | uniq
}

def "__changes_completion_values_11" [context?: string] {
  [
    "unified"
    "side-by-side"
  ] | flatten | uniq
}

def "__changes_completion_values_12" [context?: string] {
  [
    (try { run-external "changes" "__values" "repository" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_13" [context?: string] {
  [
    (try { run-external "changes" "__values" "paths" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_14" [context?: string] {
  [
    (try { run-external "changes" "__values" "repository" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_15" [context?: string] {
  [
    "agent"
    "user"
  ] | flatten | uniq
}

def "__changes_completion_values_16" [context?: string] {
  [
    (try { run-external "changes" "__values" "note-writers" ($context | default "") | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_17" [context?: string] {
  [
    "left"
    "right"
  ] | flatten | uniq
}

def "__changes_completion_values_18" [context?: string] {
  [
    (try { run-external "changes" "__values" "repository" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_19" [context?: string] {
  [
    (try { run-external "changes" "__values" "repository" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_20" [context?: string] {
  [
    (try { run-external "changes" "__values" "repository" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_21" [context?: string] {
  [
    (try { run-external "changes" "__values" "note-generators" ($context | default "") | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_22" [context?: string] {
  [
    (try { run-external "changes" "__values" "note-writers" ($context | default "") | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_23" [context?: string] {
  [
    (try { run-external "changes" "__values" "repository" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_24" [context?: string] {
  [
    (try { run-external "changes" "__values" "repository" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_25" [context?: string] {
  [
    (try { run-external "changes" "__values" "repository" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_26" [context?: string] {
  [
    (try { run-external "changes" "__values" "note-readers" ($context | default "") | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_27" [context?: string] {
  [
    (try { run-external "changes" "__values" "repository" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_28" [context?: string] {
  [
    (try { run-external "changes" "__values" "providers" ($context | default "") | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_29" [context?: string] {
  [
    (try { run-external "changes" "__values" "providers" ($context | default "") | lines } catch { [] })
  ] | flatten | uniq
}
