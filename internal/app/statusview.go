package app

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"

	"github.com/neur0manc/dot/internal/store"
)

const (
	colorGreen = "\x1b[32m"
	colorRed   = "\x1b[31m"
	colorFaint = "\x1b[2m" // dims the terminal's own color, so it suits light and dark themes
	colorReset = "\x1b[0m"
)

// isTerminal reports whether w is a terminal.
func isTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)

	return ok && term.IsTerminal(int(file.Fd()))
}

// supportsColor reports whether w is a terminal that takes colors, and
// colors are not disabled with NO_COLOR.
func supportsColor(w io.Writer) bool {
	return isTerminal(w) && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
}

func codeLabel(code store.Code) string {
	switch code {
	case store.Added:
		return "new file:"
	case store.Deleted:
		return "deleted:"
	default:
		return "modified:"
	}
}

// writeStatus prints the changes that the next commit contains, then the
// changes that are not staged yet.
func writeStatus(w io.Writer, states []store.FileState, color bool) {
	paint := func(code string, text string) string {
		if !color {
			return text
		}

		return code + text + colorReset
	}

	sections := []struct {
		title string
		color string
		code  func(store.FileState) store.Code
	}{
		{
			title: "Staged (will be committed):",
			color: colorGreen,
			code:  func(s store.FileState) store.Code { return s.Staged },
		},
		{
			title: "Not staged (use `dot add -u` to stage all):",
			color: colorRed,
			code:  func(s store.FileState) store.Code { return s.Worktree },
		},
	}

	first := true

	for _, section := range sections {
		var lines []string

		for _, state := range states {
			if code := section.code(state); code != store.Unmodified {
				lines = append(lines, fmt.Sprintf("  %-10s %s", codeLabel(code), store.Describe(state.Name)))
			}
		}

		if len(lines) == 0 {
			continue
		}

		if !first {
			fmt.Fprintln(w)
		}

		first = false

		fmt.Fprintln(w, section.title)

		for _, line := range lines {
			fmt.Fprintln(w, paint(section.color, line))
		}
	}
}
