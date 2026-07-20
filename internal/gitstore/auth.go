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

	knownHostsPath := filepath.Join(
		homeDirectory,
		".ssh",
		"known_hosts",
	)

	hostKeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf(
			"load SSH known-hosts file %q: %w",
			knownHostsPath,
			err,
		)
	}

	agentAuthentication, err := gitssh.NewSSHAgentAuth(username)
	if err != nil {
		return nil, fmt.Errorf(
			"connect to SSH agent: %w",
			err,
		)
	}

	agentAuthentication.HostKeyCallback = hostKeyCallback

	return &agentAuthenticationWithHostKeyAlgorithms{
		PublicKeysCallback: agentAuthentication,
	}, nil
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

// agentAuthenticationWithHostKeyAlgorithms adjusts host-key negotiation while
// retaining SSH-agent authentication and strict known_hosts verification.
//
// OpenSSH prefers Ed25519 host keys. Go's default preference can differ, which
// may produce a known_hosts key mismatch when only the server's Ed25519 key is
// recorded.
type agentAuthenticationWithHostKeyAlgorithms struct {
	*gitssh.PublicKeysCallback
}

func (authentication *agentAuthenticationWithHostKeyAlgorithms) ClientConfig() (
	*xssh.ClientConfig,
	error,
) {
	configuration, err :=
		authentication.PublicKeysCallback.ClientConfig()
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
