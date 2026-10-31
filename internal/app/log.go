package app

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/neur0manc/dot/internal/store"
)

const (
	colorYellow = "\x1b[33m"

	shortHashLength = 7
	dateLayout      = "Mon Jan 2 15:04:05 2006 -0700"
)

// useColor decides whether the output on w is colored. when is the value of
// --color; without it color.ui decides, and without that the output is
// colored if it goes to a terminal that takes colors.
func useColor(w io.Writer, when string, configured string) (bool, error) {
	source := "--color"

	if when == "" {
		when, source = configured, "color.ui"
	}

	switch strings.ToLower(when) {
	case "always":
		return true, nil
	case "never", "false", "no", "off", "0":
		return false, nil
	case "", "auto", "true", "yes", "on", "1":
		return supportsColor(w), nil
	default:
		return false, fmt.Errorf("invalid value %q for %s", when, source)
	}
}

// countShortcut turns Git's "-5" into "-n=5", which the flag package parses.
func countShortcut(args []string) []string {
	converted := make([]string, 0, len(args))

	for _, arg := range args {
		if _, err := strconv.Atoi(strings.TrimPrefix(arg, "-")); err == nil && strings.HasPrefix(arg, "-") {
			arg = "-n=" + arg[1:]
		}

		converted = append(converted, arg)
	}

	return converted
}

func runLog(c *cli, args []string) error {
	flags := newFlags("log")
	oneline := flags.Bool("oneline", false, "")
	limit := flags.Int("n", 0, "")
	flags.IntVar(limit, "max-count", 0, "")
	noPager := flags.Bool("no-pager", false, "")
	when := flags.String("color", "", "")

	args, err := parseFlags(flags, countShortcut(args))
	if err != nil {
		return err
	}

	if len(args) != 0 {
		return usageError{"unexpected argument " + args[0]}
	}

	if *limit < 0 {
		return usageError{"the number of commits must not be negative"}
	}

	s, err := c.open()
	if err != nil {
		return err
	}

	color, err := useColor(c.stdout, *when, s.GitOption("color", "ui"))
	if err != nil {
		return usageError{err.Error()}
	}

	entries, err := s.Log(*limit)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		fmt.Fprintln(c.stdout, "No commits yet.")
		return nil
	}

	output, finish := io.Writer(c.stdout), func() error { return nil }
	if !*noPager {
		output, finish = c.startPager(s.GitOption("core", "pager"))
	}

	// A pager that the user quits early closes the pipe; that is no error.
	writeLog(output, entries, *oneline, color)
	finish()

	return nil
}

// writeLog prints entries in Git's formats: medium, or one line per commit.
func writeLog(w io.Writer, entries []store.LogEntry, oneline bool, color bool) {
	paint := func(code string, text string) string {
		if !color {
			return text
		}

		return code + text + colorReset
	}

	for index, entry := range entries {
		hash := entry.Hash.String()
		subject, body, _ := strings.Cut(strings.TrimSpace(entry.Message), "\n")

		if oneline {
			fmt.Fprintf(w, "%s %s\n", paint(colorYellow, hash[:shortHashLength]), subject)
			continue
		}

		if index > 0 {
			fmt.Fprintln(w)
		}

		fmt.Fprintln(w, paint(colorYellow, "commit "+hash))

		if len(entry.Parents) > 1 {
			parents := make([]string, 0, len(entry.Parents))
			for _, parent := range entry.Parents {
				parents = append(parents, parent.String()[:shortHashLength])
			}

			fmt.Fprintf(w, "Merge: %s\n", strings.Join(parents, " "))
		}

		fmt.Fprintf(w, "Author: %s <%s>\n", entry.Author, entry.Email)
		fmt.Fprintf(w, "Date:   %s\n\n", entry.When.Format(dateLayout))
		fmt.Fprintf(w, "    %s\n", subject)

		if body = strings.TrimSpace(body); body != "" {
			fmt.Fprintln(w)

			for _, line := range strings.Split(body, "\n") {
				fmt.Fprintf(w, "    %s\n", strings.TrimRight(line, " \t\r"))
			}
		}
	}
}
