package app

import (
	"errors"
	"fmt"
	"io"

	"git.sr.ht/~stephan/dotmgr/internal/paths"
	"git.sr.ht/~stephan/dotmgr/internal/store"
)

// BuildInfo describes the running binary.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// usageError makes Run print the command's usage and exit with status 2.
type usageError struct{ message string }

func (e usageError) Error() string { return e.message }

// context carries everything a command needs.
type context struct {
	stdout io.Writer
	stderr io.Writer
	build  BuildInfo
}

// open opens the repository, failing with a helpful message if none exists.
func (c *context) open() (*store.Store, error) {
	gitDir, home, err := locations()
	if err != nil {
		return nil, err
	}

	return store.Open(gitDir, home)
}

func locations() (gitDir string, home string, err error) {
	gitDir, err = paths.Repository()
	if err != nil {
		return "", "", err
	}

	home, err = paths.Home()
	if err != nil {
		return "", "", err
	}

	return gitDir, home, nil
}

type command struct {
	name    string
	args    string // argument synopsis shown in help
	summary string
	run     func(c *context, args []string) error
}

func commands() []command {
	return []command{
		{"init", "", "Create an empty repository", runInit},
		{"clone", "[--force] <url>", "Clone a repository and check its files out into $HOME", runClone},
		{"add", "<path>...", "Track files, or all files in directories, and stage them", runAdd},
		{"rm", "<path>...", "Stop tracking files; the files stay on disk", runRemove},
		{"list", "", "List tracked files", runList},
		{"status", "", "Show staged and modified tracked files", runStatus},
		{"diff", "[--staged]", "Show changes in tracked files", runDiff},
		{"commit", "-m <message>", "Commit staged changes", runCommit},
		{"checkout", "[--force]", "Restore tracked files from the last commit", runCheckout},
		{"config", "<user.name|user.email> [value]", "Read or set the commit identity", runConfig},
		{"remote", "add <name> <url> | list", "Manage remotes", runRemote},
		{"push", "[remote]", "Push the current branch; defaults to origin", runPush},
		{"pull", "[--force] [remote]", "Fast-forward from a remote and update tracked files", runPull},
		{"repo-path", "", "Show the Git directory", runRepoPath},
		{"platform", "", "Show the operating system and architecture", runPlatform},
		{"version", "", "Show version information", runVersion},
		{"help", "", "Show this help", runHelp},
	}
}

// Run executes the command-line application and returns a process exit code.
func Run(
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	build BuildInfo,
) int {
	c := &context{stdout: stdout, stderr: stderr, build: build}

	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}

	name := args[0]

	switch name {
	case "-h", "--help":
		name = "help"
	case "--version":
		name = "version"
	}

	for _, candidate := range commands() {
		if candidate.name != name {
			continue
		}

		err := candidate.run(c, args[1:])

		var usage usageError
		switch {
		case err == nil:
			return 0
		case errors.As(err, &usage):
			fmt.Fprintf(stderr, "error: %v\n", usage)
			fmt.Fprintf(
				stderr,
				"usage: dotmgr %s %s\n",
				candidate.name,
				candidate.args,
			)
			return 2
		default:
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
	}

	fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
	printUsage(stderr)

	return 2
}

func printUsage(output io.Writer) {
	fmt.Fprintln(output, `dotmgr manages configuration files in your home directory with Git.

Tracked files stay where they are; the repository lives elsewhere.

Usage:
  dotmgr <command> [arguments]

Commands:`)

	table := commands()

	for _, entry := range table {
		synopsis := entry.name
		if entry.args != "" {
			synopsis += " " + entry.args
		}

		fmt.Fprintf(output, "  %s\n          %s\n", synopsis, entry.summary)
	}

	fmt.Fprintln(output, `
Environment:
  DOTMGR_HOME   working tree (default: your home directory)
  DOTMGR_REPO   Git directory (default: user data directory)
  DOTMGR_HTTPS_USERNAME, DOTMGR_HTTPS_PASSWORD
                HTTPS credentials for push, pull and clone`)
}
