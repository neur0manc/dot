package store

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
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
		return sshAuthentication(endpoint.User, endpoint.Host)

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
	host string,
) (transport.AuthMethod, error) {
	return newSSHAuth(username, host)
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
