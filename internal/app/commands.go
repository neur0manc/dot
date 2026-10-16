package app

import (
	"errors"
	"fmt"
	"runtime"
	"strings"

	"git.sr.ht/~stephan/dotmgr/internal/store"
)

// takeFlag removes flag (or its short form) from args and reports whether it
// was present.
func takeFlag(args []string, long string, short string) ([]string, bool) {
	var rest []string
	found := false

	for _, arg := range args {
		if arg == long || (short != "" && arg == short) {
			found = true
			continue
		}

		rest = append(rest, arg)
	}

	return rest, found
}

func runInit(c *context, args []string) error {
	if len(args) != 0 {
		return usageError{"init takes no arguments"}
	}

	gitDir, home, err := locations()
	if err != nil {
		return err
	}

	if _, err := store.Init(gitDir, home); err != nil {
		return err
	}

	fmt.Fprintf(c.stdout, "Initialized repository at %s\n", gitDir)
	fmt.Fprintf(c.stdout, "Working tree: %s\n", home)

	return nil
}

func runClone(c *context, args []string) error {
	args, force := takeFlag(args, "--force", "-f")
	if len(args) != 1 {
		return usageError{"clone needs exactly one URL"}
	}

	gitDir, home, err := locations()
	if err != nil {
		return err
	}

	s, result, err := store.Clone(gitDir, home, args[0], c.stderr)

	var conflict *store.ConflictError
	if errors.As(err, &conflict) && force {
		result, err = s.Checkout(true)
	}

	if errors.As(err, &conflict) {
		fmt.Fprintf(c.stdout, "Cloned %s into %s.\n", args[0], gitDir)
		return fmt.Errorf(
			"%w\nNo files were written; review them with `dotmgr diff`, "+
				"then run `dotmgr checkout --force` to overwrite them",
			err,
		)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(c.stdout, "Cloned %s into %s.\n", args[0], gitDir)
	printResult(c, result)

	return nil
}

func printResult(c *context, result store.SyncResult) {
	for _, name := range result.Written {
		fmt.Fprintf(c.stdout, "  wrote   %s\n", name)
	}

	for _, name := range result.Removed {
		fmt.Fprintf(c.stdout, "  removed %s\n", name)
	}
}

func runAdd(c *context, args []string) error {
	if len(args) == 0 {
		return usageError{"add needs at least one path"}
	}

	s, err := c.open()
	if err != nil {
		return err
	}

	names, err := s.Add(args...)
	if err != nil {
		return err
	}

	for _, name := range names {
		fmt.Fprintf(c.stdout, "Added %s\n", name)
	}

	return nil
}

func runRemove(c *context, args []string) error {
	if len(args) == 0 {
		return usageError{"rm needs at least one path"}
	}

	s, err := c.open()
	if err != nil {
		return err
	}

	names, err := s.Remove(args...)
	if err != nil {
		return err
	}

	for _, name := range names {
		fmt.Fprintf(c.stdout, "Untracked %s\n", name)
	}

	return nil
}

func runList(c *context, args []string) error {
	if len(args) != 0 {
		return usageError{"list takes no arguments"}
	}

	s, err := c.open()
	if err != nil {
		return err
	}

	names, err := s.Tracked()
	if err != nil {
		return err
	}

	for _, name := range names {
		fmt.Fprintln(c.stdout, name)
	}

	return nil
}

func runStatus(c *context, args []string) error {
	if len(args) != 0 {
		return usageError{"status takes no arguments"}
	}

	s, err := c.open()
	if err != nil {
		return err
	}

	states, err := s.Status()
	if err != nil {
		return err
	}

	for _, state := range states {
		fmt.Fprintln(c.stdout, state)
	}

	if len(states) == 0 {
		fmt.Fprintln(c.stdout, "All tracked files are in sync.")
	}

	return nil
}

func runDiff(c *context, args []string) error {
	args, staged := takeFlag(args, "--staged", "--cached")
	if len(args) != 0 {
		return usageError{"unexpected argument " + args[0]}
	}

	s, err := c.open()
	if err != nil {
		return err
	}

	diffs, err := s.Diff(staged)
	if err != nil {
		return err
	}

	for _, diff := range diffs {
		fmt.Fprintf(c.stdout, "diff --dotmgr a/%s b/%s\n", diff.Name, diff.Name)

		if diff.Binary {
			fmt.Fprintf(c.stdout, "Binary files differ: %s\n", diff.Name)
			continue
		}

		fmt.Fprint(c.stdout, diff.Patch)
	}

	return nil
}

func runCommit(c *context, args []string) error {
	if len(args) != 2 || (args[0] != "-m" && args[0] != "--message") {
		return usageError{"commit needs a message"}
	}

	message := strings.TrimSpace(args[1])
	if message == "" {
		return usageError{"commit message must not be empty"}
	}

	s, err := c.open()
	if err != nil {
		return err
	}

	hash, err := s.Commit(message)
	if errors.Is(err, store.ErrNoStagedChanges) {
		fmt.Fprintln(c.stdout, "Nothing to commit.")
		return nil
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(c.stdout, "Committed %.12s: %s\n", hash, message)

	return nil
}

func runCheckout(c *context, args []string) error {
	args, force := takeFlag(args, "--force", "-f")
	if len(args) != 0 {
		return usageError{"unexpected argument " + args[0]}
	}

	s, err := c.open()
	if err != nil {
		return err
	}

	result, err := s.Checkout(force)
	if err != nil {
		return conflictHint(err, "--force")
	}

	printResult(c, result)

	if len(result.Written) == 0 && len(result.Removed) == 0 {
		fmt.Fprintln(c.stdout, "Nothing to restore.")
	}

	return nil
}

// conflictHint explains how to resolve a ConflictError.
func conflictHint(err error, flag string) error {
	var conflict *store.ConflictError
	if errors.As(err, &conflict) {
		return fmt.Errorf(
			"%w\nNothing was changed; rerun with %s to overwrite them",
			err,
			flag,
		)
	}

	return err
}

func runConfig(c *context, args []string) error {
	if len(args) != 1 && len(args) != 2 {
		return usageError{"config needs a key and an optional value"}
	}

	s, err := c.open()
	if err != nil {
		return err
	}

	if len(args) == 2 {
		return s.SetUserConfig(args[0], args[1])
	}

	value, err := s.UserConfig(args[0])
	if err != nil {
		return err
	}

	if value == "" {
		return fmt.Errorf("%s is not configured", args[0])
	}

	fmt.Fprintln(c.stdout, value)

	return nil
}

func runRemote(c *context, args []string) error {
	if len(args) == 0 {
		return usageError{"remote needs an operation: add or list"}
	}

	switch {
	case args[0] == "add" && len(args) != 3:
		return usageError{"remote add needs a name and a URL"}
	case args[0] == "list" && len(args) != 1:
		return usageError{"remote list takes no arguments"}
	case args[0] != "add" && args[0] != "list":
		return usageError{fmt.Sprintf("unknown remote operation %q", args[0])}
	}

	s, err := c.open()
	if err != nil {
		return err
	}

	if args[0] == "add" {
		if err := s.AddRemote(args[1], args[2]); err != nil {
			return err
		}

		fmt.Fprintf(c.stdout, "Added remote %s: %s\n", args[1], args[2])

		return nil
	}

	remotes, err := s.Remotes()
	if err != nil {
		return err
	}

	for _, remote := range remotes {
		for _, url := range remote.URLs {
			fmt.Fprintf(c.stdout, "%s\t%s\n", remote.Name, url)
		}
	}

	return nil
}

func remoteArgument(args []string) (string, error) {
	switch len(args) {
	case 0:
		return "origin", nil
	case 1:
		return args[0], nil
	default:
		return "", usageError{"expected at most one remote name"}
	}
}

func runPush(c *context, args []string) error {
	remote, err := remoteArgument(args)
	if err != nil {
		return err
	}

	s, err := c.open()
	if err != nil {
		return err
	}

	pushed, err := s.Push(remote, c.stderr)
	if err != nil {
		return err
	}

	if !pushed {
		fmt.Fprintln(c.stdout, "Everything up-to-date.")
		return nil
	}

	fmt.Fprintf(c.stdout, "Pushed current branch to %s.\n", remote)

	return nil
}

func runPull(c *context, args []string) error {
	args, force := takeFlag(args, "--force", "-f")

	remote, err := remoteArgument(args)
	if err != nil {
		return err
	}

	s, err := c.open()
	if err != nil {
		return err
	}

	updated, err := s.Pull(remote, force, c.stderr)
	if err != nil {
		return conflictHint(err, "--force")
	}

	if !updated {
		fmt.Fprintln(c.stdout, "Already up-to-date.")
		return nil
	}

	fmt.Fprintf(c.stdout, "Updated current branch from %s.\n", remote)

	return nil
}

func runRepoPath(c *context, args []string) error {
	if len(args) != 0 {
		return usageError{"repo-path takes no arguments"}
	}

	gitDir, _, err := locations()
	if err != nil {
		return err
	}

	fmt.Fprintln(c.stdout, gitDir)

	return nil
}

func runPlatform(c *context, args []string) error {
	fmt.Fprintf(c.stdout, "%s/%s\n", runtime.GOOS, runtime.GOARCH)
	return nil
}

func runVersion(c *context, args []string) error {
	fmt.Fprintf(
		c.stdout,
		"dotmgr %s\ncommit: %s\nbuilt: %s\n",
		c.build.Version,
		c.build.Commit,
		c.build.Date,
	)

	return nil
}

func runHelp(c *context, args []string) error {
	printUsage(c.stdout)
	return nil
}
