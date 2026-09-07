package main

import "strings"

type interactiveBinding struct {
	id          string
	keys        string
	short       string
	description string
	footer      bool
}

var interactiveBindings = []interactiveBinding{
	{id: "focus", keys: "tab", short: "pane", description: "move focus between the explorer and changes panes", footer: true},
	{id: "focus-navigator", keys: "ctrl+h / ctrl+j", short: "explorer", description: "focus the explorer at the left or bottom"},
	{id: "focus-main", keys: "ctrl+l / ctrl+k", short: "changes", description: "focus the changes pane"},
	{id: "move", keys: "j / k", short: "move", description: "select an explorer row or scroll the changes pane", footer: true},
	{id: "page", keys: "ctrl+d / ctrl+u", short: "page", description: "scroll the changes pane by half a page"},
	{id: "activate", keys: "enter", short: "open", description: "open the selected file or commit", footer: true},
	{id: "tab", keys: "f", short: "files/history", description: "switch the explorer between Files and History"},
	{id: "navigator", keys: "t", short: "tree/list", description: "switch the file navigator between tree and list"},
	{id: "dock", keys: "d", short: "dock", description: "move the explorer between the left and bottom"},
	{id: "layout", keys: "s", short: "layout", description: "switch the diff between unified and side-by-side"},
	{id: "view", keys: "v", short: "view", description: "cycle working, staged, and first-parent commit changes"},
	{id: "line", keys: "[ / ]", short: "line", description: "choose the previous or next changed line for a note"},
	{id: "note", keys: "a / n / e", short: "note", description: "add a note with the configured, popup, or editor input"},
	{id: "refresh", keys: "r", short: "refresh", description: "refresh the repository and provider snapshot", footer: true},
	{id: "command", keys: ":", short: "command", description: "open the command line", footer: true},
	{id: "mouse", keys: "click / wheel", short: "mouse", description: "focus or select a pane and scroll it"},
	{id: "help", keys: "?", short: "help", description: "open this key list", footer: true},
	{id: "quit", keys: "q", short: "quit", description: "leave Changes", footer: true},
}

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

func interactiveBindingByID(id string) interactiveBinding {
	for _, binding := range interactiveBindings {
		if binding.id == id {
			return binding
		}
	}
	panic("unknown interactive key binding: " + id)
}

func interactiveBindingHint(id string) string {
	binding := interactiveBindingByID(id)
	return binding.keys + " " + binding.short
}

func interactiveBindingHints(ids ...string) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, interactiveBindingHint(id))
	}
	return strings.Join(parts, "   ")
}
