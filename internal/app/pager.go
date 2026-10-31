package app

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const defaultPager = "less"

// pagerCommand chooses the pager as Git does: $GIT_PAGER, core.pager (passed
// in as configured), $PAGER, and finally less. An empty value or "cat" means
// no pager.
func pagerCommand(configured string) string {
	choice := defaultPager

	switch value, set := os.LookupEnv("GIT_PAGER"); {
	case set:
		choice = value
	case configured != "":
		choice = configured
	default:
		if value, set := os.LookupEnv("PAGER"); set {
			choice = value
		}
	}

	choice = strings.TrimSpace(choice)
	if choice == "cat" {
		return ""
	}

	return choice
}

// startPager returns the writer for the output of a command and a function
// that ends it. When stdout is a terminal and a pager is configured, the
// output goes through the pager; otherwise it goes to stdout directly. A
// pager that cannot be started is reported and skipped, because the output
// matters more than its presentation.
func (c *cli) startPager(configured string) (io.Writer, func() error) {
	direct := func() error { return nil }

	command := pagerCommand(configured)
	if command == "" || !isTerminal(c.stdout) {
		return c.stdout, direct
	}

	if command == defaultPager {
		// less may be missing, notably on Windows.
		if _, err := exec.LookPath(defaultPager); err != nil {
			return c.stdout, direct
		}
	}

	pager := shellCommand(command)
	pager.Stdout = c.stdout
	pager.Stderr = c.stderr
	pager.Env = pagerEnvironment()

	input, err := pager.StdinPipe()
	if err == nil {
		err = pager.Start()
	}
	if err != nil {
		fmt.Fprintf(c.stderr, "warning: cannot start pager %q: %v\n", command, err)
		return c.stdout, direct
	}

	return input, func() error {
		input.Close()

		return pager.Wait()
	}
}

// pagerEnvironment adds the options Git gives less when the user has not
// chosen any: quit if the output fits one screen, show colors, do not clear
// the screen.
func pagerEnvironment() []string {
	environment := os.Environ()

	if _, set := os.LookupEnv("LESS"); !set {
		environment = append(environment, "LESS=FRX")
	}

	if _, set := os.LookupEnv("LV"); !set && runtime.GOOS != "windows" {
		environment = append(environment, "LV=-c")
	}

	return environment
}
