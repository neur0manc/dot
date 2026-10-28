package store

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kevinburke/ssh_config"
	sshagent "github.com/xanzy/ssh-agent"
	xssh "golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// passphraseEnvironmentVariable supplies the passphrase of encrypted private
// key files when no terminal is available for a prompt.
const passphraseEnvironmentVariable = "DOT_SSH_PASSPHRASE"

// defaultIdentityFiles are tried after the files named in ~/.ssh/config,
// in the order OpenSSH uses.
var defaultIdentityFiles = []string{
	"id_rsa",
	"id_ecdsa",
	"id_ecdsa_sk",
	"id_ed25519",
	"id_ed25519_sk",
}

// hostKeyAlgorithms prefers Ed25519, like OpenSSH does. Go's default order
// can differ, which makes known_hosts verification fail when only the
// server's Ed25519 key is recorded.
var hostKeyAlgorithms = []string{
	"ssh-ed25519",
	"ecdsa-sha2-nistp256",
	"ecdsa-sha2-nistp384",
	"ecdsa-sha2-nistp521",
	"rsa-sha2-512",
	"rsa-sha2-256",
	"ssh-rsa",
}

// sshAuth authenticates like the ssh command: it offers the keys of the
// running ssh-agent first, then the private keys named by IdentityFile in
// ~/.ssh/config and the default key files in ~/.ssh. Encrypted key files ask
// for their passphrase only when the server gets to them, so a key in the
// agent never causes a prompt. The server's host key is verified against
// ~/.ssh/known_hosts; see hostkey.go.
type sshAuth struct {
	user           string
	host           string
	home           string
	knownHostsPath string
	prompt         io.Writer

	// askPassphrase returns the passphrase of an encrypted key file.
	askPassphrase func(filename string) ([]byte, error)

	// confirmHost decides whether to trust a host that is not in known_hosts.
	confirmHost func(hostname string, key xssh.PublicKey) (bool, error)
}

func newSSHAuth(user string, host string) (*sshAuth, error) {
	if strings.TrimSpace(user) == "" {
		return nil, errors.New("SSH remote URL does not specify a username")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf(
			"determine home directory for SSH authentication: %w",
			err,
		)
	}

	auth := &sshAuth{
		user:           user,
		host:           host,
		home:           home,
		knownHostsPath: filepath.Join(home, ".ssh", "known_hosts"),
		prompt:         os.Stderr,
	}
	auth.askPassphrase = auth.passphrase
	auth.confirmHost = auth.askToTrustHost

	return auth, nil
}

func (a *sshAuth) Name() string { return "dot-ssh" }

func (a *sshAuth) String() string {
	return fmt.Sprintf("user: %s, name: %s", a.user, a.Name())
}

// ClientConfig implements the go-git SSH auth method interface.
func (a *sshAuth) ClientConfig() (*xssh.ClientConfig, error) {
	return &xssh.ClientConfig{
		User:              a.user,
		Auth:              []xssh.AuthMethod{xssh.PublicKeysCallback(a.signers)},
		HostKeyCallback:   a.verifyHostKey,
		HostKeyAlgorithms: hostKeyAlgorithms,
	}, nil
}

// signers collects every key that could authenticate: agent keys first.
func (a *sshAuth) signers() ([]xssh.Signer, error) {
	var (
		result []xssh.Signer
		seen   = map[string]bool{}
		notes  []string
	)

	add := func(signers ...xssh.Signer) {
		for _, signer := range signers {
			fingerprint := string(signer.PublicKey().Marshal())
			if !seen[fingerprint] {
				seen[fingerprint] = true
				result = append(result, signer)
			}
		}
	}

	agentSigners, err := agentSigners()
	switch {
	case err != nil:
		notes = append(notes, fmt.Sprintf("ssh-agent unavailable (%v)", err))
	case len(agentSigners) == 0:
		notes = append(notes, "ssh-agent holds no keys (see ssh-add -l)")
	default:
		add(agentSigners...)
	}

	// With agent keys on offer, a passphrase is only asked for if the server
	// prefers an encrypted key file over all of them.
	deferPassphrase := len(result) > 0

	for _, filename := range a.identityFiles() {
		signer, err := a.loadKey(filename, deferPassphrase)
		switch {
		case errors.Is(err, os.ErrNotExist):
		case err != nil:
			notes = append(notes, fmt.Sprintf("%s: %v", filename, err))
		default:
			add(signer)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf(
			"no usable SSH keys for %s: %s",
			a.host,
			strings.Join(notes, "; "),
		)
	}

	return result, nil
}

func agentSigners() ([]xssh.Signer, error) {
	if !sshagent.Available() {
		return nil, errors.New("SSH_AUTH_SOCK is not set")
	}

	// The connection stays open: the signers use it during the handshake. It
	// closes when the process exits.
	agent, _, err := sshagent.New()
	if err != nil {
		return nil, err
	}

	return agent.Signers()
}

// identityFiles lists candidate private key files in the order ssh tries
// them: IdentityFile entries of ~/.ssh/config, then the default names.
func (a *sshAuth) identityFiles() []string {
	var files []string
	seen := map[string]bool{}

	add := func(filename string) {
		filename = a.expand(filename)
		if filename != "" && !seen[filename] {
			seen[filename] = true
			files = append(files, filename)
		}
	}

	for _, filename := range a.configuredIdentityFiles() {
		add(filename)
	}

	for _, name := range defaultIdentityFiles {
		add(filepath.Join(a.home, ".ssh", name))
	}

	return files
}

// configuredIdentityFiles returns the IdentityFile entries that apply to the
// host in ~/.ssh/config. The file is read from the configured home directory
// rather than through ssh_config's package-level default, which consults the
// passwd database and caches the first result.
func (a *sshAuth) configuredIdentityFiles() []string {
	file, err := os.Open(filepath.Join(a.home, ".ssh", "config"))
	if err != nil {
		return nil
	}
	defer file.Close()

	configuration, err := ssh_config.Decode(file)
	if err != nil {
		return nil
	}

	files, err := configuration.GetAll(a.host, "IdentityFile")
	if err != nil {
		return nil
	}

	return files
}

func (a *sshAuth) expand(filename string) string {
	switch {
	case filename == "":
		return ""
	case filename == "~":
		return a.home
	case strings.HasPrefix(filename, "~/"):
		return filepath.Join(a.home, filename[2:])
	case filepath.IsAbs(filename):
		return filename
	default:
		return filepath.Join(a.home, ".ssh", filename)
	}
}

// loadKey reads a private key file. An encrypted key is unlocked right away,
// or, with deferPassphrase, only when the server asks for its signature.
func (a *sshAuth) loadKey(
	filename string,
	deferPassphrase bool,
) (xssh.Signer, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	signer, err := xssh.ParsePrivateKey(data)

	var missing *xssh.PassphraseMissingError
	if !errors.As(err, &missing) {
		return signer, err
	}

	unlock := func() (xssh.Signer, error) {
		passphrase, err := a.askPassphrase(filename)
		if err != nil {
			return nil, err
		}

		return xssh.ParsePrivateKeyWithPassphrase(data, passphrase)
	}

	if deferPassphrase && missing.PublicKey != nil {
		return &lockedSigner{publicKey: missing.PublicKey, unlock: unlock}, nil
	}

	unlocked, err := unlock()
	if err != nil {
		return nil, err
	}

	return unlocked, nil
}

func (a *sshAuth) passphrase(filename string) ([]byte, error) {
	if value := os.Getenv(passphraseEnvironmentVariable); value != "" {
		return []byte(value), nil
	}

	descriptor := int(os.Stdin.Fd())
	if !term.IsTerminal(descriptor) {
		return nil, fmt.Errorf(
			"key is encrypted; add it to the agent with ssh-add, or set %s",
			passphraseEnvironmentVariable,
		)
	}

	fmt.Fprintf(a.prompt, "Enter passphrase for %s: ", filename)
	defer fmt.Fprintln(a.prompt)

	passphrase, err := term.ReadPassword(descriptor)
	if err != nil {
		return nil, fmt.Errorf("read passphrase: %w", err)
	}

	return bytes.TrimRight(passphrase, "\r\n"), nil
}

// lockedSigner stands in for an encrypted key file. Its public key is known
// from the file, so the server can accept or reject the key first; the
// passphrase is requested only for the signature.
type lockedSigner struct {
	publicKey xssh.PublicKey
	unlock    func() (xssh.Signer, error)
	signer    xssh.Signer
}

func (l *lockedSigner) PublicKey() xssh.PublicKey { return l.publicKey }

func (l *lockedSigner) Sign(random io.Reader, data []byte) (*xssh.Signature, error) {
	return l.SignWithAlgorithm(random, data, "")
}

func (l *lockedSigner) SignWithAlgorithm(
	random io.Reader,
	data []byte,
	algorithm string,
) (*xssh.Signature, error) {
	if l.signer == nil {
		signer, err := l.unlock()
		if err != nil {
			return nil, err
		}

		l.signer = signer
	}

	if algorithm == "" {
		return l.signer.Sign(random, data)
	}

	algorithmSigner, ok := l.signer.(xssh.AlgorithmSigner)
	if !ok {
		return nil, fmt.Errorf("key does not support algorithm %s", algorithm)
	}

	return algorithmSigner.SignWithAlgorithm(random, data, algorithm)
}
