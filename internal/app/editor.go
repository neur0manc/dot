package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/neur0manc/dot/internal/store"
)

const commitTemplate = `
# Please enter the commit message for your changes. Lines starting
# with '#' will be ignored, and an empty message aborts the commit.
#
# Changes to be committed:
`

// editCommitMessage asks for a commit message in the user's editor, as
// git commit does without -m. It returns an empty message when the user
// left none.
func editCommitMessage(s *store.Store) (string, error) {
	states, err := s.Status()
	if err != nil {
		return "", err
	}

	var template strings.Builder
	template.WriteString(commitTemplate)

	for _, state := range states {
		if state.Staged != store.Unmodified {
			fmt.Fprintf(&template, "#\t%c  %s\n", state.Staged, store.Describe(state.Name))
		}
	}

	filename := filepath.Join(s.GitDir(), "COMMIT_EDITMSG")
	if err := os.WriteFile(filename, []byte(template.String()), 0o644); err != nil {
		return "", fmt.Errorf("write commit message template: %w", err)
	}

	editor := s.Editor()
	if err := runEditor(editor, filename); err != nil {
		return "", fmt.Errorf("run editor %q: %w", editor, err)
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("read commit message: %w", err)
	}

	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "#") {
			lines = append(lines, strings.TrimRight(line, " \t\r"))
		}
	}

	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

// runEditor opens filename in editor, which may carry arguments, such as
// "code --wait". Like Git, it lets the shell parse the command, except on
// Windows, where it is split at spaces.
func runEditor(editor string, filename string) error {
	var command *exec.Cmd

	if runtime.GOOS == "windows" {
		fields := strings.Fields(editor)
		command = exec.Command(fields[0], append(fields[1:], filename)...)
	} else {
		command = exec.Command("sh", "-c", editor+` "$@"`, editor, filename)
	}

	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr

	return command.Run()
}
