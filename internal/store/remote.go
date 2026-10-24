package store

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/go-git/go-git/v5/config"
)

// Remote is a configured Git remote.
type Remote struct {
	Name string
	URLs []string
}

// AddRemote adds a named remote.
func (s *Store) AddRemote(name string, url string) error {
	name = strings.TrimSpace(name)
	url = strings.TrimSpace(url)

	if name == "" {
		return errors.New("remote name must not be empty")
	}

	if url == "" {
		return errors.New("remote URL must not be empty")
	}

	_, err := s.repo.CreateRemote(&config.RemoteConfig{
		Name: name,
		URLs: []string{url},
	})
	if err != nil {
		return fmt.Errorf("add remote %q: %w", name, err)
	}

	return nil
}

// Remotes returns all configured remotes sorted by name.
func (s *Store) Remotes() ([]Remote, error) {
	gitRemotes, err := s.repo.Remotes()
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

	slices.SortFunc(result, func(a, b Remote) int { return strings.Compare(a.Name, b.Name) })

	return result, nil
}
