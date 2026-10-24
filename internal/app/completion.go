package app

import (
	_ "embed"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/neur0manc/dot/internal/store"
)

// completeFiles asks the shell to complete file names itself.
const completeFiles = ":files"

var (
	//go:embed completion/dot.bash
	bashCompletion string
	//go:embed completion/dot.zsh
	zshCompletion string
	//go:embed completion/dot.fish
	fishCompletion string
)

var completionScripts = map[string]string{
	"bash": bashCompletion,
	"zsh":  zshCompletion,
	"fish": fishCompletion,
}

// conditionFlagNames are the flags that addConditionFlags defines.
var conditionFlagNames = []string{"--os", "--arch", "--class", "--host"}

// commandFlags lists the flags of each command, for completion.
var commandFlags = map[string][]string{
	"clone":    {"--force"},
	"add":      conditionFlagNames,
	"rm":       conditionFlagNames,
	"diff":     {"--staged"},
	"commit":   {"-m", "--message"},
	"checkout": {"--force"},
	"pull":     {"--force"},
}

func runCompletion(c *cli, args []string) error {
	if len(args) != 1 {
		return usageError{"completion takes a shell: bash, zsh or fish"}
	}

	script, ok := completionScripts[args[0]]
	if !ok {
		return usageError{fmt.Sprintf("unknown shell %q; use bash, zsh or fish", args[0])}
	}

	fmt.Fprint(c.stdout, script)

	return nil
}

// runComplete prints the candidates for the last of args, the word being
// completed, one per line. The other args are the words before it, without
// "dot". It is called by the completion scripts as "dot __complete".
func runComplete(c *cli, args []string) {
	if len(args) == 0 {
		args = []string{""}
	}

	words, current := args[:len(args)-1], args[len(args)-1]

	for _, candidate := range candidates(c, words, current) {
		if candidate == completeFiles || strings.HasPrefix(candidate, current) {
			fmt.Fprintln(c.stdout, candidate)
		}
	}
}

// candidates returns what may come next after words, the words before the
// one being completed. Candidates that don't start with current are dropped
// by the caller.
func candidates(c *cli, words []string, current string) []string {
	if len(words) == 0 {
		names := slices.Collect(maps.Keys(aliases))
		for _, command := range commands() {
			names = append(names, command.name)
		}

		slices.Sort(names)

		return names
	}

	name := words[0]
	if command, ok := aliases[name]; ok {
		name = command
	}

	if flag, _, found := strings.Cut(current, "="); found && strings.HasPrefix(flag, "-") {
		var values []string
		for _, value := range flagValues(c, flag) {
			values = append(values, flag+"="+value)
		}

		return values
	}

	if strings.HasPrefix(current, "-") {
		var flags []string
		for _, flag := range commandFlags[name] {
			flags = append(flags, flag)
			if slices.Contains(conditionFlagNames, flag) {
				flags = append(flags, flag+"=")
			}
		}

		return flags
	}

	arguments := 0 // arguments before current, not counting flags
	for _, word := range words[1:] {
		if !strings.HasPrefix(word, "-") {
			arguments++
		}
	}

	switch name {
	case "add":
		return []string{completeFiles}
	case "rm", "list":
		return trackedPaths(c)
	case "push", "pull":
		if arguments == 0 {
			return remoteNames(c)
		}
	case "remote":
		if arguments == 0 {
			return []string{"add", "list"}
		}
	case "config":
		if arguments == 0 {
			return []string{"user.name", "user.email", store.ClassConfigKey}
		}
	case "completion":
		if arguments == 0 {
			return slices.Sorted(maps.Keys(completionScripts))
		}
	}

	return nil
}

// flagValues returns the values that flag can take.
func flagValues(c *cli, flag string) []string {
	facts, _ := store.MachineFacts()
	if s, err := c.openQuietly(); err == nil {
		facts = s.Facts()
	}

	switch flag {
	case "--os":
		return []string{"darwin", "freebsd", "linux", "macos", "unix", "windows"}
	case "--arch":
		return []string{"amd64", "arm64"}
	case "--class":
		if facts.Class != "" {
			return []string{facts.Class}
		}
	case "--host":
		if facts.Hostname != "" {
			return []string{strings.ToLower(facts.Hostname)}
		}
	}

	return nil
}

// trackedPaths returns the paths of the tracked files, as ~/<path>.
func trackedPaths(c *cli) []string {
	s, err := c.openQuietly()
	if err != nil {
		return nil
	}

	files, err := s.Tracked()
	if err != nil {
		return nil
	}

	var paths []string
	for _, file := range files {
		paths = append(paths, "~/"+file.Path)
	}

	slices.Sort(paths)

	return slices.Compact(paths)
}

func remoteNames(c *cli) []string {
	s, err := c.openQuietly()
	if err != nil {
		return nil
	}

	remotes, err := s.Remotes()
	if err != nil {
		return nil
	}

	var names []string
	for _, remote := range remotes {
		names = append(names, remote.Name)
	}

	return names
}

// openQuietly opens the store without upgrading it, for completion.
func (c *cli) openQuietly() (*store.Store, error) {
	gitDir, home, err := locations()
	if err != nil {
		return nil, err
	}

	return store.Open(gitDir, home)
}
