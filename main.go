package main

import (
	"log/slog"

	"github.com/danieljustus/symaira-memory/cmd"
	"github.com/danieljustus/symaira-corekit/logkit"
)

var (
	version = "0.1.0"
	commit  = "none"
	date    = "unknown"
)

func main() {
	slog.SetDefault(logkit.NewFromEnv("symmemory"))
	cmd.SetVersionInfo(version, commit, date)
	cmd.Execute()
}
