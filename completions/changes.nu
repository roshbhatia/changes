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
  --quiet # Disable progress output
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

export extern "changes interactive" [
  --commit: string@"__changes_completion_values_5" # Commit to compare with its first parent
  --config: string # YAML configuration file
  --history-limit: string # Commit history limit
  --layout: string@"__changes_completion_values_6" # Diff layout
  --no-calls # Skip call analysis
  --no-groups # Skip logical change grouping
  --no-notes # Skip diff notes
  --no-symbols # Skip symbol analysis
  --quiet # Disable progress output
  --refresh # Bypass cached workspace and provider results
  --view: string@"__changes_completion_values_7" # Git comparison view
  --width: string # Render width
  ...args: string@"__changes_completion_none"
]

export extern "changes workspace" [
  --commit: string@"__changes_completion_values_8" # Commit to compare with its first parent
  --config: string # YAML configuration file
  --history-limit: string # Commit history limit
  --layout: string@"__changes_completion_values_9" # Diff layout
  --no-calls # Skip call analysis
  --no-groups # Skip logical change grouping
  --no-notes # Skip diff notes
  --no-symbols # Skip symbol analysis
  --quiet # Disable progress output
  --refresh # Bypass cached workspace and provider results
  --view: string@"__changes_completion_values_10" # Git comparison view
  --width: string # Render width
  --interval: string # Watch interval
  --watch # Emit JSON Lines refresh events
  ...args: string@"__changes_completion_none"
]

export extern "changes difftool" [
  --color: string@"__changes_completion_values_12" # Color output
  --config: string # YAML configuration file
  --engine: string@"__changes_completion_values_13" # File comparison engine
  --difftool: string # Git-compatible difftool executable
  --layout: string@"__changes_completion_values_14" # Diff layout
  --width: string # Render width
  ...args: string@"__changes_completion_values_11"
]

export extern "changes render" [
  --color: string@"__changes_completion_values_15" # Color output
  --config: string # YAML configuration file
  --engine: string@"__changes_completion_values_16" # Patch display engine
  --filter: string # Standard-input patch filter
  --layout: string@"__changes_completion_values_17" # Diff layout
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
  --commit: string@"__changes_completion_values_18" # First-parent commit comparison
  --config: string # YAML configuration file
  --expected-file-sha256: string # Require the selected file side to match this SHA-256 digest
  --file: string@"__changes_completion_values_19" # Repository file to annotate
  --from: string@"__changes_completion_values_20" # Left revision
  --json # Print the created note as JSON
  --line: string # Last line of the note range
  --message: string # Summary and optional rationale
  --message-file: string # Read note text from a file or standard input
  --origin: string@"__changes_completion_values_21" # Author kind
  --provider: string@"__changes_completion_values_22" # Writable note provider
  --session: string # Harness session identifier
  --side: string@"__changes_completion_values_23" # Diff side
  --staged # Compare the index
  --start-line: string # First line of a multi-line range
  --to: string@"__changes_completion_values_24" # Right revision
  ...args: string@"__changes_completion_none"
]

export extern "changes note generate" [
  --commit: string@"__changes_completion_values_25" # First-parent commit comparison
  --config: string # YAML configuration file
  --from: string@"__changes_completion_values_26" # Left revision
  --json # Print generated notes as JSON
  --provider: string@"__changes_completion_values_27" # Note generator provider
  --session: string # Harness session identifier
  --staged # Compare the index
  --store: string@"__changes_completion_values_28" # Writable note provider
  --to: string@"__changes_completion_values_29" # Right revision
  ...args: string@"__changes_completion_none"
]

export extern "changes note list" [
  --commit: string@"__changes_completion_values_30" # First-parent commit comparison
  --config: string # YAML configuration file
  --from: string@"__changes_completion_values_31" # Left revision
  --json # Print JSON
  --provider: string@"__changes_completion_values_32" # Note provider
  --staged # Compare the index
  --to: string@"__changes_completion_values_33" # Right revision
  ...args: string@"__changes_completion_none"
]

export extern "changes provider" [
  ...args: string@"__changes_completion_none"
]

export extern "changes provider list" [
  --config: string # YAML configuration file
  --json # Print JSON
  ...args: string@"__changes_completion_values_34"
]

export extern "changes provider validate" [
  --config: string # YAML configuration file
  --json # Print JSON
  ...args: string@"__changes_completion_values_35"
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
    "interactive"
    "workspace"
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
    (try { run-external "changes" "__values" "repository" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_6" [context?: string] {
  [
    "unified"
    "side-by-side"
  ] | flatten | uniq
}

def "__changes_completion_values_7" [context?: string] {
  [
    "working"
    "staged"
    "commit"
  ] | flatten | uniq
}

def "__changes_completion_values_8" [context?: string] {
  [
    (try { run-external "changes" "__values" "repository" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_9" [context?: string] {
  [
    "unified"
    "side-by-side"
  ] | flatten | uniq
}

def "__changes_completion_values_10" [context?: string] {
  [
    "working"
    "staged"
    "commit"
  ] | flatten | uniq
}

def "__changes_completion_values_11" [context?: string] {
  [
    (try { run-external "changes" "__values" "paths" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_12" [context?: string] {
  [
    "auto"
    "always"
    "never"
  ] | flatten | uniq
}

def "__changes_completion_values_13" [context?: string] {
  [
    "builtin"
    "difftool"
  ] | flatten | uniq
}

def "__changes_completion_values_14" [context?: string] {
  [
    "unified"
    "side-by-side"
  ] | flatten | uniq
}

def "__changes_completion_values_15" [context?: string] {
  [
    "auto"
    "always"
    "never"
  ] | flatten | uniq
}

def "__changes_completion_values_16" [context?: string] {
  [
    "builtin"
    "filter"
  ] | flatten | uniq
}

def "__changes_completion_values_17" [context?: string] {
  [
    "unified"
    "side-by-side"
  ] | flatten | uniq
}

def "__changes_completion_values_18" [context?: string] {
  [
    (try { run-external "changes" "__values" "repository" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_19" [context?: string] {
  [
    (try { run-external "changes" "__values" "paths" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_20" [context?: string] {
  [
    (try { run-external "changes" "__values" "repository" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_21" [context?: string] {
  [
    "agent"
    "user"
  ] | flatten | uniq
}

def "__changes_completion_values_22" [context?: string] {
  [
    (try { run-external "changes" "__values" "note-writers" ($context | default "") | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_23" [context?: string] {
  [
    "left"
    "right"
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
    (try { run-external "changes" "__values" "repository" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_27" [context?: string] {
  [
    (try { run-external "changes" "__values" "note-generators" ($context | default "") | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_28" [context?: string] {
  [
    (try { run-external "changes" "__values" "note-writers" ($context | default "") | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_29" [context?: string] {
  [
    (try { run-external "changes" "__values" "repository" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_30" [context?: string] {
  [
    (try { run-external "changes" "__values" "repository" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_31" [context?: string] {
  [
    (try { run-external "changes" "__values" "repository" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_32" [context?: string] {
  [
    (try { run-external "changes" "__values" "note-readers" ($context | default "") | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_33" [context?: string] {
  [
    (try { run-external "changes" "__values" "repository" | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_34" [context?: string] {
  [
    (try { run-external "changes" "__values" "providers" ($context | default "") | lines } catch { [] })
  ] | flatten | uniq
}

def "__changes_completion_values_35" [context?: string] {
  [
    (try { run-external "changes" "__values" "providers" ($context | default "") | lines } catch { [] })
  ] | flatten | uniq
}
