package main

import sharedkeymap "github.com/roshbhatia/go-utils/keymap"

var interactiveKeyCatalog = sharedkeymap.Must(
	sharedkeymap.Binding{
		ID: "focus", Keys: []string{"ctrl+h", "ctrl+j", "ctrl+k", "ctrl+l"},
		Display: "ctrl+h/j/k/l", Short: "pane", Description: "move focus to the pane in that screen direction",
	},
	sharedkeymap.Binding{
		ID: "move", Keys: []string{"j", "k", "up", "down"},
		Display: "j / k", Short: "move", Description: "select an explorer row or scroll the changes pane",
	},
	sharedkeymap.Binding{
		ID: "page", Keys: []string{"ctrl+d", "ctrl+u"},
		Display: "ctrl+d / ctrl+u", Short: "page", Description: "scroll the changes pane by half a page",
	},
	sharedkeymap.Binding{ID: "activate", Keys: []string{"enter"}, Short: "open", Description: "open the selected file or commit"},
	sharedkeymap.Binding{
		ID: "tab", Keys: []string{"tab", "shift+tab"},
		Display: "tab / shift+tab", Short: "tab", Description: "move through tabs inside the focused pane",
	},
	sharedkeymap.Binding{ID: "mark", Keys: []string{"space"}, Short: "mark", Description: "include or exclude the selected History commit"},
	sharedkeymap.Binding{ID: "generate", Keys: []string{"g"}, Short: "generate", Description: "generate reviewable notes for marked commits or the cursor"},
	sharedkeymap.Binding{ID: "navigator", Keys: []string{"t"}, Short: "tree/list", Description: "switch the file navigator between tree and list"},
	sharedkeymap.Binding{ID: "dock", Keys: []string{"d"}, Short: "dock", Description: "move the explorer between the left and bottom"},
	sharedkeymap.Binding{ID: "layout", Keys: []string{"s"}, Short: "layout", Description: "switch the diff between unified and side-by-side"},
	sharedkeymap.Binding{ID: "view", Keys: []string{"v"}, Short: "view", Description: "cycle working, staged, and first-parent commit changes"},
	sharedkeymap.Binding{
		ID: "line", Keys: []string{"[", "]"},
		Display: "[ / ]", Short: "line", Description: "choose the previous or next changed line for a note",
	},
	sharedkeymap.Binding{
		ID: "note", Keys: []string{"a", "n", "e"},
		Display: "a / n / e", Short: "note", Description: "add a note with the configured, popup, or editor input",
	},
	sharedkeymap.Binding{ID: "refresh", Keys: []string{"r"}, Short: "refresh", Description: "refresh the repository and provider snapshot"},
	sharedkeymap.Binding{ID: "command", Keys: []string{":"}, Short: "command", Description: "open the command line"},
	sharedkeymap.Binding{
		ID: "mouse", Keys: []string{"click", "wheel"},
		Display: "click / wheel", Short: "mouse", Description: "focus panes, activate tabs, select rows, and scroll",
	},
	sharedkeymap.Binding{ID: "help", Keys: []string{"?"}, Short: "help", Description: "open this key list"},
	sharedkeymap.Binding{ID: "quit", Keys: []string{"q"}, Short: "quit", Description: "leave Changes"},
)

var interactivePaletteValues = map[string][]string{
	"dock":      {"bottom", "left"},
	"layout":    {"side-by-side", "unified"},
	"navigator": {"list", "tree"},
	"note":      {"editor", "popup"},
	"tab":       {"files", "history"},
	"view":      {"commit", "staged", "working"},
}

func interactivePaletteCommands() []string {
	return []string{"dock", "layout", "navigator", "note", "quit", "refresh", "tab", "view"}
}

func interactiveBindingHint(id string) string {
	hint, err := interactiveKeyCatalog.Hint(id)
	if err != nil {
		panic(err)
	}
	return hint.String()
}

func interactiveBindingHints(ids ...string) string {
	line, err := interactiveKeyCatalog.HintLine("   ", ids...)
	if err != nil {
		panic(err)
	}
	return line
}
