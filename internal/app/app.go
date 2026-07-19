package app

import (
	"fmt"
	"io"
	"os"
	"runtime"

	"git.sr.ht/~stephan/dotmgr/internal/gitstore"
	"git.sr.ht/~stephan/dotmgr/internal/paths"
	"git.sr.ht/~stephan/dotmgr/internal/tracking"
)

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Run executes the command-line application and returns a process exit code.
func Run(
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	build BuildInfo,
) int {
	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0

	case "version", "--version":
		fmt.Fprintf(
			stdout,
			"dotmgr %s\ncommit: %s\nbuilt: %s\n",
			build.Version,
			build.Commit,
			build.Date,
		)
		return 0

	case "platform":
		fmt.Fprintf(stdout, "%s/%s\n", runtime.GOOS, runtime.GOARCH)
		return 0

	case "repo-path":
		repositoryPath, err := paths.Repository()
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}

		fmt.Fprintln(stdout, repositoryPath)
		return 0

	case "init":
		repositoryPath, err := paths.Repository()
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}

		if err := gitstore.Init(repositoryPath); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}

		fmt.Fprintf(stdout, "Initialized repository at %s\n", repositoryPath)
		return 0

	case "add":
		if len(args) != 2 {
			fmt.Fprintln(stderr, "usage: dotmgr add <file>")
			return 2
		}

		repositoryPath, err := paths.Repository()
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}

		homeDirectory, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(stderr, "error: determine home directory: %v\n", err)
			return 1
		}

		entry, err := tracking.Add(
			repositoryPath,
			homeDirectory,
			args[1],
		)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}

		fmt.Fprintf(
			stdout,
			"Added %s\n  source: %s\n  target: %s\n",
			args[1],
			entry.Source,
			entry.Target,
		)
		return 0

	case "status":
		repositoryPath, err := paths.Repository()
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}

		status, err := gitstore.Status(repositoryPath)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}

		if status.IsClean() {
			fmt.Fprintln(stdout, "Repository is clean.")
			return 0
		}

		fmt.Fprint(stdout, status.String())
		return 0

	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func printUsage(output io.Writer) {
	fmt.Fprintln(output, `dotmgr manages configuration files using an embedded Git repository.

Usage:
  dotmgr <command>

Commands:
  init        Initialize the local repository
  add <file>  Copy a file into the repository and stage it
  status      Show the repository status
  repo-path   Show the local repository path
  platform    Show the operating system and architecture
  version     Show version information
  help        Show this help`)
}
