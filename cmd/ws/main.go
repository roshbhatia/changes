package main

import (
	"github.com/roshbhatia/changes/internal/workspacecli"
	"os"
)

func main() { os.Exit(workspace.Run(os.Args[1:])) }
