package store

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// authenticationForURL picks how to log in to a remote: like ssh for SSH
// URLs, and with DOTMGR_HTTPS_USERNAME and DOTMGR_HTTPS_PASSWORD for HTTPS.
func authenticationForURL(remoteURL string) (transport.AuthMethod, error) {
	endpoint, err := transport.NewEndpoint(remoteURL)
	if err != nil {
		return nil, fmt.Errorf("parse remote URL %q: %w", remoteURL, err)
	}

	switch strings.ToLower(endpoint.Protocol) {
	case "ssh":
		return newSSHAuth(endpoint.User, endpoint.Host)

	case "https":
		return httpsAuthentication()

	case "http":
		if os.Getenv("DOTMGR_HTTPS_USERNAME") != "" ||
			os.Getenv("DOTMGR_HTTPS_PASSWORD") != "" {
			return nil, errors.New("refusing to send credentials over insecure HTTP")
		}

		return nil, nil

	case "file":
		return nil, nil

	default:
		return nil, fmt.Errorf("unsupported remote protocol %q", endpoint.Protocol)
	}
}

func httpsAuthentication() (transport.AuthMethod, error) {
	username := os.Getenv("DOTMGR_HTTPS_USERNAME")
	password := os.Getenv("DOTMGR_HTTPS_PASSWORD")

	if username == "" && password == "" {
		// Public HTTPS repositories need no authentication.
		return nil, nil
	}

	if username == "" {
		return nil, errors.New("DOTMGR_HTTPS_USERNAME is required when DOTMGR_HTTPS_PASSWORD is set")
	}

	if password == "" {
		return nil, errors.New("DOTMGR_HTTPS_PASSWORD is required when DOTMGR_HTTPS_USERNAME is set")
	}

	return &githttp.BasicAuth{Username: username, Password: password}, nil
}
