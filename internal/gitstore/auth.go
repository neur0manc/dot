package gitstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	xssh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func authenticationForURL(
	remoteURL string,
) (transport.AuthMethod, error) {
	endpoint, err := transport.NewEndpoint(remoteURL)
	if err != nil {
		return nil, fmt.Errorf(
			"parse remote URL %q: %w",
			remoteURL,
			err,
		)
	}

	switch strings.ToLower(endpoint.Protocol) {
	case "ssh":
		return sshAuthentication(endpoint.User)

	case "https":
		return httpsAuthentication()

	case "http":
		if os.Getenv("DOTMGR_HTTPS_USERNAME") != "" ||
			os.Getenv("DOTMGR_HTTPS_PASSWORD") != "" {
			return nil, fmt.Errorf(
				"refusing to send credentials over insecure HTTP",
			)
		}

		return nil, nil

	case "file":
		return nil, nil

	default:
		return nil, fmt.Errorf(
			"unsupported remote protocol %q",
			endpoint.Protocol,
		)
	}
}

func sshAuthentication(
	username string,
) (transport.AuthMethod, error) {
	if strings.TrimSpace(username) == "" {
		return nil, fmt.Errorf(
			"SSH remote URL does not specify a username",
		)
	}

	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf(
			"determine home directory for SSH authentication: %w",
			err,
		)
	}

	knownHostsPath := strings.TrimSpace(
		os.Getenv("DOTMGR_SSH_KNOWN_HOSTS"),
	)
	if knownHostsPath == "" {
		knownHostsPath = filepath.Join(
			homeDirectory,
			".ssh",
			"known_hosts",
		)
	}

	hostKeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf(
			"load SSH known-hosts file %q: %w",
			knownHostsPath,
			err,
		)
	}

	keyPath, err := findSSHKey(homeDirectory)
	if err != nil {
		return nil, err
	}

	passphrase := os.Getenv("DOTMGR_SSH_KEY_PASSPHRASE")

	publicKeys, err := gitssh.NewPublicKeysFromFile(
		username,
		keyPath,
		passphrase,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"load SSH private key %q: %w",
			keyPath,
			err,
		)
	}

	publicKeys.HostKeyCallback = hostKeyCallback

	return &publicKeysWithHostKeyAlgorithms{
		PublicKeys: publicKeys,
	}, nil
}

func findSSHKey(homeDirectory string) (string, error) {
	configuredKey := strings.TrimSpace(
		os.Getenv("DOTMGR_SSH_KEY"),
	)

	if configuredKey != "" {
		keyPath, err := expandHomePath(
			homeDirectory,
			configuredKey,
		)
		if err != nil {
			return "", err
		}

		if _, err := os.Stat(keyPath); err != nil {
			return "", fmt.Errorf(
				"inspect SSH private key %q: %w",
				keyPath,
				err,
			)
		}

		return keyPath, nil
	}

	candidates := []string{
		filepath.Join(homeDirectory, ".ssh", "id_ed25519"),
		filepath.Join(homeDirectory, ".ssh", "id_rsa"),
	}

	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() {
			return candidate, nil
		}

		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf(
				"inspect SSH private key %q: %w",
				candidate,
				err,
			)
		}
	}

	return "", fmt.Errorf(
		"no SSH private key found; set DOTMGR_SSH_KEY or create " +
			"~/.ssh/id_ed25519",
	)
}

func expandHomePath(
	homeDirectory string,
	path string,
) (string, error) {
	if path == "~" {
		return homeDirectory, nil
	}

	if strings.HasPrefix(path, "~/") ||
		strings.HasPrefix(path, `~\`) {
		return filepath.Join(
			homeDirectory,
			filepath.FromSlash(
				strings.ReplaceAll(path[2:], `\`, `/`),
			),
		), nil
	}

	if strings.HasPrefix(path, "~") {
		return "", fmt.Errorf(
			"unsupported home-relative path %q",
			path,
		)
	}

	return filepath.Clean(path), nil
}

func httpsAuthentication() (transport.AuthMethod, error) {
	username := os.Getenv("DOTMGR_HTTPS_USERNAME")
	password := os.Getenv("DOTMGR_HTTPS_PASSWORD")

	if username == "" && password == "" {
		// Public HTTPS repositories need no authentication.
		return nil, nil
	}

	if username == "" {
		return nil, fmt.Errorf(
			"DOTMGR_HTTPS_USERNAME is required when HTTPS " +
				"authentication is configured",
		)
	}

	if password == "" {
		return nil, fmt.Errorf(
			"DOTMGR_HTTPS_PASSWORD is required when HTTPS " +
				"authentication is configured",
		)
	}

	return &githttp.BasicAuth{
		Username: username,
		Password: password,
	}, nil
}

// publicKeysWithHostKeyAlgorithms adjusts host-key negotiation while retaining
// go-git's public-key authentication and strict known_hosts verification.
//
// OpenSSH prefers Ed25519 host keys. Go's default preference can differ, which
// may produce a known_hosts key mismatch when only the server's Ed25519 key is
// recorded.
type publicKeysWithHostKeyAlgorithms struct {
	*gitssh.PublicKeys
}

func (authentication *publicKeysWithHostKeyAlgorithms) ClientConfig() (
	*xssh.ClientConfig,
	error,
) {
	configuration, err := authentication.PublicKeys.ClientConfig()
	if err != nil {
		return nil, err
	}

	configuration.HostKeyAlgorithms = []string{
		"ssh-ed25519",
		"ecdsa-sha2-nistp256",
		"ecdsa-sha2-nistp384",
		"ecdsa-sha2-nistp521",
		"rsa-sha2-512",
		"rsa-sha2-256",
		"ssh-rsa",
	}

	return configuration, nil
}
