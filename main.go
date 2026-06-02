// Symaira Memory is a local-first persistent memory context manager with MCP and TUI support.
package main

import "github.com/danieljustus/symaira-memory/cmd"

var (
	version = "0.1.0"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.SetVersionInfo(version, commit, date)
	cmd.Execute()
}
