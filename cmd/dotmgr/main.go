package main

import (
	"os"

	"git.sr.ht/~stephan/dotmgr/internal/app"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	exitCode := app.Run(
		os.Args[1:],
		os.Stdout,
		os.Stderr,
		app.BuildInfo{
			Version: version,
			Commit:  commit,
			Date:    date,
		},
	)

	os.Exit(exitCode)
}
