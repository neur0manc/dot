package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	xssh "golang.org/x/crypto/ssh"
)

// unknownHostSetup prepares a server whose key is not in known_hosts and
// returns its address together with the known_hosts path.
func unknownHostSetup(t *testing.T) (string, string) {
	t.Helper()

	home := sshHome(t)
	private, public := newKey(t)

	writeKeyFile(t, filepath.Join(home, ".ssh", "id_ed25519"), private, "")
	address := startSSHServer(t, home, public)

	knownHosts := filepath.Join(home, ".ssh", "known_hosts")
	if err := os.Remove(knownHosts); err != nil {
		t.Fatal(err)
	}

	return address, knownHosts
}

func TestSSHTrustsUnknownHostOnFirstUse(t *testing.T) {
	address, knownHosts := unknownHostSetup(t)

	asked := 0
	trust := func(string, xssh.PublicKey) (bool, error) {
		asked++

		return true, nil
	}

	if err := dialConfirming(t, address, trust); err != nil {
		t.Fatalf("connection to unknown host failed: %v", err)
	}

	if asked != 1 {
		t.Fatalf("asked %d times, want 1", asked)
	}

	recorded, err := os.ReadFile(knownHosts)
	if err != nil || !strings.Contains(string(recorded), "ssh-ed25519") {
		t.Fatalf("host key not recorded: %q, %v", recorded, err)
	}

	// The recorded host is known now and needs no further question.
	if err := dial(t, address); err != nil {
		t.Fatalf("connection to recorded host failed: %v", err)
	}
}

func TestSSHDeclinedHostIsRefusedAndNotRecorded(t *testing.T) {
	address, knownHosts := unknownHostSetup(t)

	decline := func(string, xssh.PublicKey) (bool, error) { return false, nil }

	if err := dialConfirming(t, address, decline); err == nil {
		t.Fatal("expected declined host to be refused")
	}

	if _, err := os.Stat(knownHosts); !os.IsNotExist(err) {
		t.Fatalf("declined host was recorded (stat error: %v)", err)
	}
}

func TestSSHAppendsToKnownHostsWithoutFinalNewline(t *testing.T) {
	address, knownHosts := unknownHostSetup(t)

	_, other := newKey(t)
	line := knownhostsLine("example.org:22", other)
	if err := os.WriteFile(knownHosts, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}

	trust := func(string, xssh.PublicKey) (bool, error) { return true, nil }

	if err := dialConfirming(t, address, trust); err != nil {
		t.Fatalf("connection failed: %v", err)
	}

	// knownhosts.New parses every line, so a joined line would fail here.
	if err := dial(t, address); err != nil {
		t.Fatalf("known_hosts corrupted by append: %v", err)
	}
}

func TestSSHChangedHostKeyNeverAsksToTrust(t *testing.T) {
	home := sshHome(t)
	private, public := newKey(t)

	writeKeyFile(t, filepath.Join(home, ".ssh", "id_ed25519"), private, "")
	address := startSSHServer(t, home, public)

	_, other := newKey(t)
	line := knownhostsLine(address, other)
	if err := os.WriteFile(filepath.Join(home, ".ssh", "known_hosts"), []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := dial(t, address)
	if err == nil || !strings.Contains(err.Error(), "has changed") {
		t.Fatalf("expected a changed-key error, got: %v", err)
	}
}
