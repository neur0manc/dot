package gitstore

import (
	"fmt"
	"sort"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
)

type Remote struct {
	Name string
	URLs []string
}

// AddRemote adds a named remote to the repository.
func AddRemote(
	repositoryPath string,
	name string,
	url string,
) error {
	name = strings.TrimSpace(name)
	url = strings.TrimSpace(url)

	if name == "" {
		return fmt.Errorf("remote name must not be empty")
	}

	if url == "" {
		return fmt.Errorf("remote URL must not be empty")
	}

	repository, err := git.PlainOpen(repositoryPath)
	if err != nil {
		return fmt.Errorf(
			"open repository at %q: %w",
			repositoryPath,
			err,
		)
	}

	_, err = repository.CreateRemote(&config.RemoteConfig{
		Name: name,
		URLs: []string{url},
	})
	if err != nil {
		return fmt.Errorf(
			"add remote %q: %w",
			name,
			err,
		)
	}

	return nil
}

// ListRemotes returns all configured remotes sorted by name.
func ListRemotes(repositoryPath string) ([]Remote, error) {
	repository, err := git.PlainOpen(repositoryPath)
	if err != nil {
		return nil, fmt.Errorf(
			"open repository at %q: %w",
			repositoryPath,
			err,
		)
	}

	gitRemotes, err := repository.Remotes()
	if err != nil {
		return nil, fmt.Errorf("list remotes: %w", err)
	}

	result := make([]Remote, 0, len(gitRemotes))

	for _, gitRemote := range gitRemotes {
		configuration := gitRemote.Config()

		result = append(result, Remote{
			Name: configuration.Name,
			URLs: append([]string(nil), configuration.URLs...),
		})
	}

	sort.Slice(result, func(left int, right int) bool {
		return result[left].Name < result[right].Name
	})

	return result, nil
}
