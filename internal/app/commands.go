package app

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/neur0manc/dot/internal/store"
)

// newFlags returns an empty flag set for a command.
func newFlags(name string) *flag.FlagSet {
	return flag.NewFlagSet(name, flag.ContinueOnError)
}

// optionalValue is a flag that can be given bare, as in --os, or with a
// value, as in --os=linux.
type optionalValue struct {
	set   bool
	value string
}

func (v *optionalValue) String() string { return v.value }

// IsBoolFlag lets the flag package accept the flag without a value.
func (v *optionalValue) IsBoolFlag() bool { return true }

func (v *optionalValue) Set(value string) error {
	v.set = true
	if value != "true" { // what the flag package passes for a bare flag
		v.value = value
	}

	return nil
}

// conditionFlags are the flags that limit files to some machines.
type conditionFlags struct {
	keys   []string
	values map[string]*optionalValue
}

func addConditionFlags(flags *flag.FlagSet) *conditionFlags {
	c := &conditionFlags{values: map[string]*optionalValue{}}

	for _, flagKey := range []struct{ flag, key string }{
		{"os", store.ConditionOS},
		{"arch", store.ConditionArch},
		{"class", store.ConditionClass},
		{"host", store.ConditionHostname},
	} {
		value := &optionalValue{}
		flags.Var(value, flagKey.flag, "")
		c.keys = append(c.keys, flagKey.key)
		c.values[flagKey.key] = value
	}

	return c
}

// conditions returns the given conditions in a fixed order, so that the same
// flags always name the same variant.
func (c *conditionFlags) conditions(facts store.Facts, args []string) ([]string, error) {
	var conditions []string

	for _, key := range c.keys {
		value := c.values[key]
		if !value.set {
			continue
		}

		// "--host macbook" makes macbook a path. Point that out instead
		// of failing to find the file.
		if value.value == "" {
			for _, arg := range args {
				if _, err := os.Lstat(arg); err != nil && !strings.ContainsAny(arg, `/\~`) {
					return nil, usageError{fmt.Sprintf(
						"%q is not a file; to name a value, write --%s=%s",
						arg, flagName(key), arg,
					)}
				}
			}
		}

		condition, err := facts.Condition(key, value.value)
		if err != nil {
			return nil, err
		}

		conditions = append(conditions, condition)
	}

	return conditions, nil
}

func flagName(key string) string {
	if key == store.ConditionHostname {
		return "host"
	}

	return key
}

func runInit(c *cli, args []string) error {
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

func runClone(c *cli, args []string) error {
	flags := newFlags("clone")
	force := flags.Bool("force", false, "")

	args, err := parseFlags(flags, args)
	if err != nil {
		return err
	}

	if len(args) != 1 {
		return usageError{"clone needs exactly one URL"}
	}

	gitDir, home, err := locations()
	if err != nil {
		return err
	}

	s, result, err := store.Clone(gitDir, home, args[0], c.stderr)

	var conflict *store.ConflictError
	if errors.As(err, &conflict) && *force {
		result, err = s.Checkout(true)
	}

	if errors.As(err, &conflict) {
		fmt.Fprintf(c.stdout, "Cloned %s into %s.\n", args[0], gitDir)
		return fmt.Errorf(
			"%w\nNo files were written; review them with `dot diff`, "+
				"then run `dot checkout --force` to overwrite them",
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

func printResult(c *cli, result store.SyncResult) {
	for _, name := range result.Written {
		fmt.Fprintf(c.stdout, "  wrote   %s\n", name)
	}

	for _, name := range result.Removed {
		fmt.Fprintf(c.stdout, "  removed %s\n", name)
	}
}

func runAdd(c *cli, args []string) error {
	flags := newFlags("add")
	conditionFlags := addConditionFlags(flags)

	args, err := parseFlags(flags, args)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return usageError{"add needs at least one path"}
	}

	s, err := c.open()
	if err != nil {
		return err
	}

	conditions, err := conditionFlags.conditions(s.Facts(), args)
	if err != nil {
		return err
	}

	var added []store.AddedFile
	if len(conditions) == 0 {
		added, err = s.Add(args...)
	} else {
		added, err = s.AddVariant(conditions, args...)
	}
	if err != nil {
		return err
	}

	for _, file := range added {
		switch {
		case file.Replaces != "":
			fmt.Fprintf(c.stdout, "Added %s, which replaces the plain %s\n", store.Describe(file.Name), file.Replaces)
		case !file.Used:
			fmt.Fprintf(c.stdout, "Added %s; this machine does not use it\n", store.Describe(file.Name))
		default:
			fmt.Fprintf(c.stdout, "Added %s\n", store.Describe(file.Name))
		}
	}

	return nil
}

func runRemove(c *cli, args []string) error {
	flags := newFlags("rm")
	conditionFlags := addConditionFlags(flags)

	args, err := parseFlags(flags, args)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return usageError{"rm needs at least one path"}
	}

	s, err := c.open()
	if err != nil {
		return err
	}

	conditions, err := conditionFlags.conditions(s.Facts(), args)
	if err != nil {
		return err
	}

	names, err := s.Remove(conditions, args...)
	if err != nil {
		return err
	}

	for _, name := range names {
		fmt.Fprintf(c.stdout, "Untracked %s\n", store.Describe(name))
	}

	return nil
}

func runList(c *cli, args []string) error {
	s, err := c.open()
	if err != nil {
		return err
	}

	files, err := s.Tracked(args...)
	if err != nil {
		return err
	}

	for _, file := range files {
		line := store.Describe(file.Name)
		if !file.Used {
			line += " [not on this machine]"
		}

		fmt.Fprintln(c.stdout, line)
	}

	return nil
}

func runStatus(c *cli, args []string) error {
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
		fmt.Fprintf(c.stdout, "%c%c %s\n", state.Staged, state.Worktree, store.Describe(state.Name))
	}

	if len(states) == 0 {
		fmt.Fprintln(c.stdout, "All tracked files are in sync.")
	}

	return nil
}

func runDiff(c *cli, args []string) error {
	flags := newFlags("diff")
	staged := flags.Bool("staged", false, "")
	flags.BoolVar(staged, "cached", false, "")

	args, err := parseFlags(flags, args)
	if err != nil {
		return err
	}

	if len(args) != 0 {
		return usageError{"unexpected argument " + args[0]}
	}

	s, err := c.open()
	if err != nil {
		return err
	}

	diffs, err := s.Diff(*staged)
	if err != nil {
		return err
	}

	for _, diff := range diffs {
		fmt.Fprintf(c.stdout, "diff --dot %s\n", store.Describe(diff.Name))

		if diff.Binary {
			fmt.Fprintf(c.stdout, "Binary files differ: %s\n", diff.Path)
			continue
		}

		fmt.Fprint(c.stdout, diff.Patch)
	}

	return nil
}

func runCommit(c *cli, args []string) error {
	flags := newFlags("commit")
	message := flags.String("m", "", "")
	flags.StringVar(message, "message", "", "")

	args, err := parseFlags(flags, args)
	if err != nil {
		return err
	}

	if len(args) != 0 {
		return usageError{"unexpected argument " + args[0]}
	}

	*message = strings.TrimSpace(*message)
	if *message == "" {
		return usageError{"commit needs a message"}
	}

	s, err := c.open()
	if err != nil {
		return err
	}

	hash, err := s.Commit(*message)
	if errors.Is(err, store.ErrNoStagedChanges) {
		fmt.Fprintln(c.stdout, "Nothing to commit.")
		return nil
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(c.stdout, "Committed %.12s: %s\n", hash, *message)

	return nil
}

func runCheckout(c *cli, args []string) error {
	flags := newFlags("checkout")
	force := flags.Bool("force", false, "")

	args, err := parseFlags(flags, args)
	if err != nil {
		return err
	}

	if len(args) != 0 {
		return usageError{"unexpected argument " + args[0]}
	}

	s, err := c.open()
	if err != nil {
		return err
	}

	result, err := s.Checkout(*force)
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
		return fmt.Errorf("%w\nNothing was changed; rerun with %s to overwrite them", err, flag)
	}

	return err
}

func runConfig(c *cli, args []string) error {
	if len(args) != 1 && len(args) != 2 {
		return usageError{"config needs a key and an optional value"}
	}

	s, err := c.open()
	if err != nil {
		return err
	}

	if len(args) == 1 {
		value, err := s.Config(args[0])
		if err != nil {
			return err
		}

		if value == "" {
			return fmt.Errorf("%s is not configured", args[0])
		}

		fmt.Fprintln(c.stdout, value)

		return nil
	}

	if err := s.SetConfig(args[0], args[1]); err != nil {
		return err
	}

	if args[0] != store.ClassConfigKey {
		return nil
	}

	// Other variants may fit now.
	if _, err := s.Head(); err != nil {
		return nil // nothing committed yet
	}

	result, err := s.Checkout(false)

	var conflict *store.ConflictError
	if errors.As(err, &conflict) {
		fmt.Fprintf(c.stderr, "warning: %v\nRun `dot checkout` once they are committed, "+
			"or `dot checkout --force` to overwrite them\n", err)
		return nil
	}

	printResult(c, result)

	return err
}

func runRemote(c *cli, args []string) error {
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

func runPush(c *cli, args []string) error {
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

func runPull(c *cli, args []string) error {
	flags := newFlags("pull")
	force := flags.Bool("force", false, "")

	args, err := parseFlags(flags, args)
	if err != nil {
		return err
	}

	remote, err := remoteArgument(args)
	if err != nil {
		return err
	}

	s, err := c.open()
	if err != nil {
		return err
	}

	updated, err := s.Pull(remote, *force, c.stderr)
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

func runRepoPath(c *cli, args []string) error {
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

// runPlatform shows the facts that variants are matched against. The class
// needs a repository; without one it is left out.
func runPlatform(c *cli, args []string) error {
	if len(args) != 0 {
		return usageError{"platform takes no arguments"}
	}

	gitDir, home, err := locations()
	if err != nil {
		return err
	}

	s, err := store.Open(gitDir, home)
	if err != nil && !errors.Is(err, store.ErrNotInitialized) {
		return err
	}

	var facts store.Facts
	if s != nil {
		facts = s.Facts()
	} else if facts, err = store.MachineFacts(); err != nil {
		return err
	}

	fmt.Fprintf(c.stdout, "os:    %s\narch:  %s\nhost:  %s\n", facts.OS, facts.Arch, facts.Hostname)

	if facts.Class != "" {
		fmt.Fprintf(c.stdout, "class: %s\n", facts.Class)
	}

	return nil
}

func runVersion(c *cli, args []string) error {
	fmt.Fprintf(c.stdout, "dot %s\ncommit: %s\nbuilt: %s\n", c.build.Version, c.build.Commit, c.build.Date)
	return nil
}

func runHelp(c *cli, args []string) error {
	printUsage(c.stdout)
	return nil
}
