package app

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

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

		homeDirectory, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(
				stderr,
				"error: determine home directory: %v\n",
				err,
			)
			return 1
		}

		localStatuses, err := tracking.LocalStatuses(
			repositoryPath,
			homeDirectory,
		)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}

		repositoryStatus, err := gitstore.Status(repositoryPath)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}

		printLocalStatuses(stdout, localStatuses)
		fmt.Fprintln(stdout)
		printRepositoryStatus(stdout, repositoryStatus.String())

		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func printLocalStatuses(
	output io.Writer,
	statuses []tracking.LocalFileStatus,
) {
	fmt.Fprintln(output, "Managed files:")

	if len(statuses) == 0 {
		fmt.Fprintln(output, "  no managed files")
		return
	}

	changes := 0

	for _, status := range statuses {
		switch status.State {
		case tracking.LocalClean:
			continue

		case tracking.LocalModified:
			fmt.Fprintf(
				output,
				"  M  %s\n",
				status.Entry.Target,
			)

		case tracking.LocalNotApplied:
			fmt.Fprintf(
				output,
				"  N  %s (not applied)\n",
				status.Entry.Target,
			)

		case tracking.LocalSourceMissing:
			fmt.Fprintf(
				output,
				"  !  %s (repository source missing: %s)\n",
				status.Entry.Target,
				status.Entry.Source,
			)

		case tracking.LocalUnsupported:
			fmt.Fprintf(
				output,
				"  !  %s (unsupported file type)\n",
				status.Entry.Target,
			)
		}

		changes++
	}

	if changes == 0 {
		fmt.Fprintln(output, "  all files are in sync")
	}
}

func printRepositoryStatus(
	output io.Writer,
	statusText string,
) {
	fmt.Fprintln(output, "Repository:")

	statusText = strings.TrimSpace(statusText)

	if statusText == "" {
		fmt.Fprintln(output, "  clean")
		return
	}

	for _, line := range strings.Split(statusText, "\n") {
		fmt.Fprintf(output, "  %s\n", line)
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
