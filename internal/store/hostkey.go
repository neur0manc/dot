package store

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	xssh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
)

// acceptNewHostsEnvironmentVariable set to 1 trusts and records unknown hosts
// without asking, for use when no terminal is available. Changed host keys
// are rejected regardless.
const acceptNewHostsEnvironmentVariable = "DOT_SSH_ACCEPT_NEW_HOSTS"

// verifyHostKey checks the server's key against ~/.ssh/known_hosts. A host
// that is not listed is offered for trust on first use; a host whose key
// differs from the recorded one is always refused.
func (a *sshAuth) verifyHostKey(
	hostname string,
	remote net.Addr,
	key xssh.PublicKey,
) error {
	err := a.checkKnownHosts(hostname, remote, key)

	var keyError *knownhosts.KeyError
	if !errors.As(err, &keyError) {
		return err
	}

	if len(keyError.Want) > 0 {
		return fmt.Errorf(
			"host key of %s has changed, which may mean someone is "+
				"intercepting the connection; if the change is expected, "+
				"remove the old entry from %s: %w",
			hostname,
			a.knownHostsPath,
			err,
		)
	}

	trusted, err := a.confirmHost(hostname, key)
	if err != nil {
		return err
	}

	if !trusted {
		return fmt.Errorf("host key of %s was not accepted", hostname)
	}

	return a.recordHostKey(hostname, key)
}

// checkKnownHosts reports a host as unknown, rather than failing, when the
// known_hosts file does not exist yet.
func (a *sshAuth) checkKnownHosts(
	hostname string,
	remote net.Addr,
	key xssh.PublicKey,
) error {
	check, err := knownhosts.New(a.knownHostsPath)
	if errors.Is(err, os.ErrNotExist) {
		return &knownhosts.KeyError{}
	}
	if err != nil {
		return fmt.Errorf(
			"load SSH known-hosts file %q: %w",
			a.knownHostsPath,
			err,
		)
	}

	return check(hostname, remote, key)
}

// askToTrustHost asks on the terminal like ssh does, unless the environment
// variable allows unknown hosts.
func (a *sshAuth) askToTrustHost(
	hostname string,
	key xssh.PublicKey,
) (bool, error) {
	if os.Getenv(acceptNewHostsEnvironmentVariable) == "1" {
		return true, nil
	}

	descriptor := int(os.Stdin.Fd())
	if !term.IsTerminal(descriptor) {
		return false, fmt.Errorf(
			"host %s is not in %s; connect once with ssh to trust it, or set %s=1",
			hostname,
			a.knownHostsPath,
			acceptNewHostsEnvironmentVariable,
		)
	}

	fmt.Fprintf(
		a.prompt,
		"The authenticity of host %s can't be established.\n"+
			"%s key fingerprint is %s.\n"+
			"Are you sure you want to continue connecting (yes/no)? ",
		hostname,
		key.Type(),
		xssh.FingerprintSHA256(key),
	)

	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("read answer: %w", err)
	}

	return strings.EqualFold(strings.TrimSpace(answer), "yes"), nil
}

// recordHostKey appends the host to known_hosts, creating the file and its
// directory if needed.
func (a *sshAuth) recordHostKey(hostname string, key xssh.PublicKey) error {
	if err := os.MkdirAll(filepath.Dir(a.knownHostsPath), 0o700); err != nil {
		return fmt.Errorf("create SSH directory: %w", err)
	}

	line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)

	// Keep the new entry on its own line if the file lacks a final newline.
	if existing, err := os.ReadFile(a.knownHostsPath); err == nil &&
		len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		line = "\n" + line
	}

	file, err := os.OpenFile(
		a.knownHostsPath,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0o600,
	)
	if err != nil {
		return fmt.Errorf("open %q: %w", a.knownHostsPath, err)
	}
	defer file.Close()

	if _, err := fmt.Fprintln(file, line); err != nil {
		return fmt.Errorf("record host key in %q: %w", a.knownHostsPath, err)
	}

	return file.Close()
}
