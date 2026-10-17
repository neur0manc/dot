package store

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	xssh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

func newKey(t *testing.T) (ed25519.PrivateKey, xssh.PublicKey) {
	t.Helper()

	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	sshPublic, err := xssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}

	return private, sshPublic
}

func writeKeyFile(t *testing.T, filename string, key ed25519.PrivateKey, passphrase string) {
	t.Helper()

	var block *pem.Block
	var err error

	if passphrase == "" {
		block, err = xssh.MarshalPrivateKey(key, "test")
	} else {
		block, err = xssh.MarshalPrivateKeyWithPassphrase(key, "test", []byte(passphrase))
	}
	if err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
}

// startSSHServer runs a server that accepts only authorized and records the
// host in ~/.ssh/known_hosts. It returns the server address.
func startSSHServer(t *testing.T, home string, authorized xssh.PublicKey) string {
	t.Helper()

	hostPrivate, _ := newKey(t)
	hostSigner, err := xssh.NewSignerFromKey(hostPrivate)
	if err != nil {
		t.Fatal(err)
	}

	configuration := &xssh.ServerConfig{
		PublicKeyCallback: func(_ xssh.ConnMetadata, key xssh.PublicKey) (*xssh.Permissions, error) {
			if string(key.Marshal()) == string(authorized.Marshal()) {
				return &xssh.Permissions{}, nil
			}

			return nil, os.ErrPermission
		},
	}
	configuration.AddHostKey(hostSigner)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })

	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}

			go func() {
				defer connection.Close()

				server, channels, requests, err := xssh.NewServerConn(connection, configuration)
				if err != nil {
					return
				}
				defer server.Close()

				go xssh.DiscardRequests(requests)
				for channel := range channels {
					channel.Reject(xssh.Prohibited, "test")
				}
			}()
		}
	}()

	address := listener.Addr().String()
	line := knownhosts.Line([]string{knownhosts.Normalize(address)}, hostSigner.PublicKey())

	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "known_hosts"), []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	return address
}

// sshHome prepares an empty home directory without agent for the tests.
func sshHome(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SSH_AUTH_SOCK", "")
	t.Setenv(passphraseEnvironmentVariable, "")

	return home
}

func dial(t *testing.T, address string) error {
	t.Helper()

	auth, err := newSSHAuth("git", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	auth.prompt = &strings.Builder{}

	configuration, err := auth.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}

	client, err := xssh.Dial("tcp", address, configuration)
	if err != nil {
		return err
	}

	return client.Close()
}

func TestSSHFallsBackToDefaultKeyFile(t *testing.T) {
	home := sshHome(t)
	private, public := newKey(t)

	writeKeyFile(t, filepath.Join(home, ".ssh", "id_ed25519"), private, "")
	address := startSSHServer(t, home, public)

	if err := dial(t, address); err != nil {
		t.Fatalf("authentication with key file failed: %v", err)
	}
}

func TestSSHUsesIdentityFileFromConfig(t *testing.T) {
	home := sshHome(t)
	private, public := newKey(t)

	custom := filepath.Join(home, ".ssh", "work_key")
	writeKeyFile(t, custom, private, "")

	config := "Host *\n  IdentityFile " + custom + "\n"
	if err := os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	address := startSSHServer(t, home, public)

	if err := dial(t, address); err != nil {
		t.Fatalf("authentication with IdentityFile failed: %v", err)
	}
}

func TestSSHEncryptedKeyNeedsPassphrase(t *testing.T) {
	home := sshHome(t)
	private, public := newKey(t)

	writeKeyFile(t, filepath.Join(home, ".ssh", "id_ed25519"), private, "secret")
	address := startSSHServer(t, home, public)

	// Without terminal and passphrase the key is skipped with a hint.
	err := dial(t, address)
	if err == nil {
		t.Fatal("expected authentication to fail without a passphrase")
	}

	t.Setenv(passphraseEnvironmentVariable, "secret")

	if err := dial(t, address); err != nil {
		t.Fatalf("authentication with passphrase failed: %v", err)
	}
}

func TestSSHPrefersAgentKeys(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a Unix socket agent")
	}

	home := sshHome(t)
	private, public := newKey(t)

	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: private}); err != nil {
		t.Fatal(err)
	}

	socket := filepath.Join(t.TempDir(), "agent.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })

	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}

			go agent.ServeAgent(keyring, connection)
		}
	}()

	t.Setenv("SSH_AUTH_SOCK", socket)

	address := startSSHServer(t, home, public)

	if err := dial(t, address); err != nil {
		t.Fatalf("authentication with agent key failed: %v", err)
	}
}

func TestSSHWithoutAnyKeyExplainsWhy(t *testing.T) {
	home := sshHome(t)
	_, public := newKey(t)
	address := startSSHServer(t, home, public)

	err := dial(t, address)
	if err == nil {
		t.Fatal("expected failure without keys")
	}

	if !strings.Contains(err.Error(), "no usable SSH keys") ||
		!strings.Contains(err.Error(), "SSH_AUTH_SOCK") {
		t.Fatalf("unhelpful error: %v", err)
	}
}

func TestSSHRejectsChangedHostKey(t *testing.T) {
	home := sshHome(t)
	private, public := newKey(t)

	writeKeyFile(t, filepath.Join(home, ".ssh", "id_ed25519"), private, "")
	address := startSSHServer(t, home, public)

	// Record a different key for the host.
	_, other := newKey(t)
	line := knownhosts.Line([]string{knownhosts.Normalize(address)}, other)
	if err := os.WriteFile(filepath.Join(home, ".ssh", "known_hosts"), []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := dial(t, address); err == nil {
		t.Fatal("expected host key mismatch to be rejected")
	}
}
