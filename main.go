package main

import (
	"log/slog"

	"github.com/danieljustus/symaira-corekit/logkit"
	"github.com/danieljustus/symaira-memory/cmd"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	slog.SetDefault(logkit.NewFromEnv("symmemory"))
	cmd.SetVersionInfo(version, commit, date)
	cmd.Execute()
}
