package app

import (
	"errors"
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

	case "diff":
		if len(args) != 1 {
			fmt.Fprintln(stderr, "usage: dotmgr diff")
			return 2
		}

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

		diffs, err := tracking.LocalDiffs(
			repositoryPath,
			homeDirectory,
		)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}

		printLocalDiffs(stdout, diffs)
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

	case "commit":
		if len(args) != 3 ||
			(args[1] != "-m" && args[1] != "--message") {
			fmt.Fprintln(
				stderr,
				"usage: dotmgr commit -m <message>",
			)
			return 2
		}

		message := strings.TrimSpace(args[2])
		if message == "" {
			fmt.Fprintln(
				stderr,
				"error: commit message must not be empty",
			)
			return 2
		}

		repositoryPath, err := paths.Repository()
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}

		hash, err := gitstore.Commit(repositoryPath, message)
		if errors.Is(err, gitstore.ErrNoStagedChanges) {
			fmt.Fprintln(stdout, "Nothing to commit.")
			return 0
		}
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}

		hashText := hash.String()

		fmt.Fprintf(
			stdout,
			"Committed %s: %s\n",
			hashText[:12],
			message,
		)

		return 0

	case "config":
		if len(args) != 2 && len(args) != 3 {
			fmt.Fprintln(
				stderr,
				"usage: dotmgr config <user.name|user.email> [value]",
			)
			return 2
		}

		repositoryPath, err := paths.Repository()
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}

		key := args[1]

		if len(args) == 2 {
			value, err := gitstore.UserConfig(repositoryPath, key)
			if err != nil {
				fmt.Fprintf(stderr, "error: %v\n", err)
				return 1
			}

			if value == "" {
				fmt.Fprintf(stderr, "error: %s is not configured\n", key)
				return 1
			}

			fmt.Fprintln(stdout, value)
			return 0
		}

		if err := gitstore.SetUserConfig(
			repositoryPath,
			key,
			args[2],
		); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}

		return 0

	case "push":
		if len(args) > 2 {
			fmt.Fprintln(
				stderr,
				"usage: dotmgr push [remote]",
			)
			return 2
		}

		remoteName := "origin"
		if len(args) == 2 {
			remoteName = args[1]
		}

		repositoryPath, err := paths.Repository()
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}

		pushed, err := gitstore.Push(
			repositoryPath,
			remoteName,
			stderr,
		)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}

		if !pushed {
			fmt.Fprintln(stdout, "Everything up-to-date.")
			return 0
		}

		fmt.Fprintf(
			stdout,
			"Pushed current branch to %s.\n",
			remoteName,
		)

		return 0

	case "clone":
		if len(args) != 2 {
			fmt.Fprintln(
				stderr,
				"usage: dotmgr clone <url>",
			)
			return 2
		}

		repositoryPath, err := paths.Repository()
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}

		if err := gitstore.Clone(
			repositoryPath,
			args[1],
			stderr,
		); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}

		fmt.Fprintf(
			stdout,
			"Cloned %s into %s.\n",
			args[1],
			repositoryPath,
		)

		fmt.Fprintln(
			stdout,
			"Managed files were not applied; run dotmgr status to inspect them.",
		)

		return 0

	case "remote":
		if len(args) < 2 {
			fmt.Fprintln(
				stderr,
				"usage: dotmgr remote <add|list>",
			)
			return 2
		}

		repositoryPath, err := paths.Repository()
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}

		switch args[1] {
		case "add":
			if len(args) != 4 {
				fmt.Fprintln(
					stderr,
					"usage: dotmgr remote add <name> <url>",
				)
				return 2
			}

			if err := gitstore.AddRemote(
				repositoryPath,
				args[2],
				args[3],
			); err != nil {
				fmt.Fprintf(stderr, "error: %v\n", err)
				return 1
			}

			fmt.Fprintf(
				stdout,
				"Added remote %s: %s\n",
				args[2],
				args[3],
			)

			return 0

		case "list":
			if len(args) != 2 {
				fmt.Fprintln(
					stderr,
					"usage: dotmgr remote list",
				)
				return 2
			}

			remotes, err := gitstore.ListRemotes(repositoryPath)
			if err != nil {
				fmt.Fprintf(stderr, "error: %v\n", err)
				return 1
			}

			for _, remote := range remotes {
				for _, url := range remote.URLs {
					fmt.Fprintf(
						stdout,
						"%s\t%s\n",
						remote.Name,
						url,
					)
				}
			}

			return 0

		default:
			fmt.Fprintf(
				stderr,
				"error: unknown remote operation %q\n",
				args[1],
			)
			return 2
		}

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

func printLocalDiffs(
	output io.Writer,
	diffs []tracking.LocalFileDiff,
) {
	for index, fileDiff := range diffs {
		if index > 0 {
			fmt.Fprintln(output)
		}

		switch fileDiff.State {
		case tracking.LocalModified:
			fmt.Fprintf(
				output,
				"diff --dotmgr repository:%s host:%s\n",
				fileDiff.Entry.Source,
				fileDiff.Entry.Target,
			)

			if fileDiff.Binary {
				fmt.Fprintf(
					output,
					"Binary files repository:%s and host:%s differ\n",
					fileDiff.Entry.Source,
					fileDiff.Entry.Target,
				)
				continue
			}

			fmt.Fprint(output, fileDiff.Patch)

		case tracking.LocalNotApplied:
			fmt.Fprintf(
				output,
				"diff --dotmgr repository:%s host:%s\n",
				fileDiff.Entry.Source,
				fileDiff.Entry.Target,
			)
			fmt.Fprintf(
				output,
				"not applied: %s\n",
				fileDiff.Entry.Target,
			)

			if fileDiff.Binary {
				fmt.Fprintf(
					output,
					"Binary repository file %s is not applied\n",
					fileDiff.Entry.Source,
				)
				continue
			}

			fmt.Fprint(output, fileDiff.Patch)

		case tracking.LocalSourceMissing:
			fmt.Fprintf(
				output,
				"cannot diff %s: repository source missing: %s\n",
				fileDiff.Entry.Target,
				fileDiff.Entry.Source,
			)

		case tracking.LocalUnsupported:
			fmt.Fprintf(
				output,
				"cannot diff %s: unsupported file type\n",
				fileDiff.Entry.Target,
			)
		}
	}
}

func printUsage(output io.Writer) {
	fmt.Fprintln(output, `dotmgr manages configuration files using an embedded Git repository.

Usage:
  dotmgr <command>

Commands:
  init        Initialize the local repository
  add <file>  Copy a host file into the repository and stage it
  status      Compare managed host files and show repository status
  diff        Show differences between repository and host files
  config <key> [value]
              Read or set repository-local configuration
  commit -m <message>
              Commit staged repository changes
  remote add <name> <url>
              Add a Git remote
  remote list
              List configured Git remotes
  push [remote]
              Push the current branch; defaults to origin
  clone <url>
              Clone into the internal repository without applying files
  repo-path   Show the local repository path
  platform    Show the operating system and architecture
  version     Show version information
  help        Show this help`)
}
