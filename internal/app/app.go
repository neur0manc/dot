// Package app implements dot's command-line interface.
package app

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/neur0manc/dot/internal/paths"
	"github.com/neur0manc/dot/internal/store"
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

// cli carries what every command needs.
type cli struct {
	stdout io.Writer
	stderr io.Writer
	build  BuildInfo
}

// open opens the repository, failing with a helpful message if none exists.
// It also converts a home directory set up by dotmgr 0.3 to the current
// layout of variants.
func (c *cli) open() (*store.Store, error) {
	gitDir, home, err := locations()
	if err != nil {
		return nil, err
	}

	s, err := store.Open(gitDir, home)
	if err != nil {
		return nil, err
	}

	notes, err := s.UpgradeLinkedVariants()
	for _, note := range notes {
		fmt.Fprintf(c.stderr, "upgrade: %s\n", note)
	}

	return s, err
}

// parseFlags parses flags anywhere among args, as Git does, and returns the
// other arguments. A "--" ends the flags.
func parseFlags(flags *flag.FlagSet, args []string) ([]string, error) {
	flags.SetOutput(io.Discard)

	var rest []string

	for {
		if err := flags.Parse(args); err != nil {
			return nil, usageError{strings.Replace(err.Error(), "flag provided but not defined:", "unknown flag", 1)}
		}

		remaining := flags.Args()
		if len(remaining) == 0 {
			return rest, nil
		}

		if consumed := len(args) - len(remaining); consumed > 0 && args[consumed-1] == "--" {
			return append(rest, remaining...), nil
		}

		rest = append(rest, remaining[0])
		args = remaining[1:]
	}
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

// aliases are short names for commands.
var aliases = map[string]string{
	"ls": "list",
	"s":  "status",
	"p":  "push",
	"d":  "diff",
}

type command struct {
	name    string
	args    string // argument synopsis shown in help
	summary string
	run     func(c *cli, args []string) error
}

func commands() []command {
	return []command{
		{"init", "", "Create an empty repository", runInit},
		{"clone", "[--force] <url>", "Clone a repository and put its files into your home directory", runClone},
		{"add", "[conditions] <path>... | -u", "Track files, or all files in directories; conditions make them variants; -u stages changes to tracked files", runAdd},
		{"rm", "[conditions] <path>...", "Stop tracking files; the files stay on disk", runRemove},
		{"list", "[<path>...]", "List tracked files and variants, or those at or beneath paths", runList},
		{"status", "", "Show staged and modified tracked files", runStatus},
		{"diff", "[--staged]", "Show changes in tracked files", runDiff},
		{"commit", "[-m <message>]", "Commit staged changes; without -m, write the message in your editor", runCommit},
		{"checkout", "[--force]", "Restore tracked files from the last commit", runCheckout},
		{"config", "<user.name|user.email|local.class> [value]", "Read or set the commit identity or machine class", runConfig},
		{"remote", "[list | add <name> <url> | remove <name> | rename <old> <new> | set-url <name> <url> | default <name>]", "Manage remotes; without an operation, list them", runRemote},
		{"push", "[-u] [remote]", "Push the current branch; defaults to its upstream, else origin", runPush},
		{"pull", "[--force] [remote]", "Fetch a remote (default: the upstream), merge it if needed and update tracked files", runPull},
		{"repo-path", "", "Show the Git directory", runRepoPath},
		{"platform", "", "Show what variants are matched against", runPlatform},
		{"completion", "<bash|zsh|fish>", "Print a shell completion script", runCompletion},
		{"version", "", "Show version information", runVersion},
		{"help", "", "Show this help", runHelp},
	}
}

// Run executes the command-line application and returns a process exit code.
func Run(args []string, stdout io.Writer, stderr io.Writer, build BuildInfo) int {
	c := &cli{stdout: stdout, stderr: stderr, build: build}

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

	if name == "__complete" {
		runComplete(c, args[1:])
		return 0
	}

	if command, ok := aliases[name]; ok {
		name = command
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
			fmt.Fprintf(stderr, "usage: dot %s %s\n", candidate.name, candidate.args)
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
	fmt.Fprintln(output, `dot manages configuration files in your home directory with Git.

Tracked files stay where they are; the repository lives elsewhere.

Usage:
  dot <command> [arguments]

Commands:`)

	for _, entry := range commands() {
		synopsis := entry.name
		for alias, command := range aliases {
			if command == entry.name {
				synopsis += ", " + alias
			}
		}
		if entry.args != "" {
			synopsis += " " + entry.args
		}

		fmt.Fprintf(output, "  %s\n          %s\n", synopsis, entry.summary)
	}

	fmt.Fprintln(output, `
Conditions limit a file to some machines. Without a value they mean this
machine; several must all match:
  --os[=<os>]         linux, darwin (or macos), windows, ...; unix is
                      every system except Windows
  --host[=<name>]     a host name, without domain
  --class[=<class>]   a class of machines, set with dot config local.class
  --arch[=<arch>]     amd64, arm64, ...
Only the variant that fits best is put on disk, under the file's own name.

Environment:
  DOT_HOME   working tree (default: your home directory)
  DOT_REPO   Git directory (default: user data directory)
  DOT_HTTPS_USERNAME, DOT_HTTPS_PASSWORD
                HTTPS credentials for push, pull and clone`)
}
