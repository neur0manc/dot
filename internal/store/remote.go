package store

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

// defaultRemote is used by push and pull when the current branch has no
// upstream.
const defaultRemote = "origin"

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

// RemoveRemote deletes a remote, its remote-tracking branches and the
// upstream settings that point to it.
func (s *Store) RemoveRemote(name string) error {
	if _, err := s.repo.Remote(name); err != nil {
		return fmt.Errorf("remove remote %q: %w", name, err)
	}

	if err := s.repo.DeleteRemote(name); err != nil {
		return fmt.Errorf("remove remote %q: %w", name, err)
	}

	if err := s.moveTrackingReferences(name, ""); err != nil {
		return fmt.Errorf("remove remote %q: %w", name, err)
	}

	if err := s.renameUpstreams(name, ""); err != nil {
		return fmt.Errorf("remove remote %q: %w", name, err)
	}

	return nil
}

// RenameRemote renames a remote together with its remote-tracking branches
// and the upstream settings that point to it.
func (s *Store) RenameRemote(oldName string, newName string) error {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return errors.New("remote name must not be empty")
	}

	old, err := s.repo.Remote(oldName)
	if err != nil {
		return fmt.Errorf("rename remote %q: %w", oldName, err)
	}

	_, err = s.repo.CreateRemote(&config.RemoteConfig{
		Name: newName,
		URLs: old.Config().URLs,
	})
	if err != nil {
		return fmt.Errorf("rename remote %q to %q: %w", oldName, newName, err)
	}

	if err := s.repo.DeleteRemote(oldName); err != nil {
		return fmt.Errorf("rename remote %q to %q: %w", oldName, newName, err)
	}

	if err := s.moveTrackingReferences(oldName, newName); err != nil {
		return fmt.Errorf("rename remote %q to %q: %w", oldName, newName, err)
	}

	if err := s.renameUpstreams(oldName, newName); err != nil {
		return fmt.Errorf("rename remote %q to %q: %w", oldName, newName, err)
	}

	return nil
}

// SetRemoteURL replaces the URLs of a remote with url.
func (s *Store) SetRemoteURL(name string, url string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return errors.New("remote URL must not be empty")
	}

	configuration, err := s.repo.Config()
	if err != nil {
		return fmt.Errorf("read repository configuration: %w", err)
	}

	remote, ok := configuration.Remotes[name]
	if !ok {
		return fmt.Errorf("set URL of remote %q: %w", name, git.ErrRemoteNotFound)
	}

	remote.URLs = []string{url}

	if err := s.repo.SetConfig(configuration); err != nil {
		return fmt.Errorf("set URL of remote %q: %w", name, err)
	}

	return nil
}

// moveTrackingReferences renames the remote-tracking branches of oldName to
// those of newName, or deletes them when newName is empty.
func (s *Store) moveTrackingReferences(oldName string, newName string) error {
	prefix := plumbing.NewRemoteReferenceName(oldName, "").String()

	references, err := s.repo.References()
	if err != nil {
		return fmt.Errorf("list references: %w", err)
	}

	var tracked []*plumbing.Reference

	err = references.ForEach(func(reference *plumbing.Reference) error {
		if strings.HasPrefix(reference.Name().String(), prefix) {
			tracked = append(tracked, reference)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("list references: %w", err)
	}

	for _, reference := range tracked {
		branch := strings.TrimPrefix(reference.Name().String(), prefix)

		if newName != "" {
			moved := plumbing.NewHashReference(
				plumbing.NewRemoteReferenceName(newName, branch),
				reference.Hash(),
			)
			if err := s.repo.Storer.SetReference(moved); err != nil {
				return fmt.Errorf("move %s: %w", reference.Name(), err)
			}
		}

		if err := s.repo.Storer.RemoveReference(reference.Name()); err != nil {
			return fmt.Errorf("delete %s: %w", reference.Name(), err)
		}
	}

	return nil
}

// renameUpstreams points the branches tracking oldName to newName, or drops
// their upstream when newName is empty.
func (s *Store) renameUpstreams(oldName string, newName string) error {
	configuration, err := s.repo.Config()
	if err != nil {
		return fmt.Errorf("read repository configuration: %w", err)
	}

	for name, branch := range configuration.Branches {
		if branch.Remote != oldName {
			continue
		}

		if newName == "" {
			delete(configuration.Branches, name)
			continue
		}

		branch.Remote = newName
	}

	if err := s.repo.SetConfig(configuration); err != nil {
		return fmt.Errorf("store upstream configuration: %w", err)
	}

	return nil
}

// SetUpstream makes remoteName the default remote of the current branch, so
// that push and pull use it when no remote is named.
func (s *Store) SetUpstream(remoteName string) error {
	if _, err := s.repo.Remote(remoteName); err != nil {
		return fmt.Errorf("open remote %q: %w", remoteName, err)
	}

	branch, err := s.currentBranch()
	if err != nil {
		return err
	}

	return s.setUpstream(branch, remoteName)
}

// ResolveRemote returns remoteName, or when it is empty the upstream of the
// current branch, or origin when the branch has none.
func (s *Store) ResolveRemote(remoteName string) (string, error) {
	if remoteName = strings.TrimSpace(remoteName); remoteName != "" {
		return remoteName, nil
	}

	branch, err := s.currentBranch()
	if err != nil {
		return "", err
	}

	upstream, err := s.upstream(branch)
	if err != nil {
		return "", err
	}

	if upstream == "" {
		return defaultRemote, nil
	}

	return upstream, nil
}

// currentBranch returns the branch HEAD points to, even before its first
// commit.
func (s *Store) currentBranch() (plumbing.ReferenceName, error) {
	head, err := s.repo.Storer.Reference(plumbing.HEAD)
	if err != nil {
		return "", fmt.Errorf("resolve current branch: %w", err)
	}

	if head.Type() != plumbing.SymbolicReference || !head.Target().IsBranch() {
		return "", fmt.Errorf("HEAD is detached at %s", head.Hash())
	}

	return head.Target(), nil
}

// upstream returns the remote configured for branch, or "" if there is none.
func (s *Store) upstream(branch plumbing.ReferenceName) (string, error) {
	configuration, err := s.repo.Config()
	if err != nil {
		return "", fmt.Errorf("read repository configuration: %w", err)
	}

	if tracking, ok := configuration.Branches[branch.Short()]; ok {
		return tracking.Remote, nil
	}

	return "", nil
}
