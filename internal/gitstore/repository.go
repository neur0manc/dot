package gitstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

const defaultGitignore = `# macOS Finder metadata
.DS_Store
`

// Init creates a non-bare Git repository at path.
func Init(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create repository directory: %w", err)
	}

	repository, err := git.PlainInit(path, false)
	if err != nil {
		if errors.Is(err, git.ErrRepositoryAlreadyExists) {
			return fmt.Errorf("repository already exists at %q", path)
		}

		return fmt.Errorf("initialize Git repository: %w", err)
	}

	if err := createDefaultGitignore(path); err != nil {
		return err
	}

	worktree, err := repository.Worktree()
	if err != nil {
		return fmt.Errorf("open repository worktree: %w", err)
	}

	if _, err := worktree.Add(".gitignore"); err != nil {
		return fmt.Errorf("stage .gitignore: %w", err)
	}

	return nil
}

func createDefaultGitignore(repositoryPath string) error {
	filename := filepath.Join(repositoryPath, ".gitignore")

	file, err := os.OpenFile(
		filename,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0o644,
	)
	if errors.Is(err, os.ErrExist) {
		// Do not overwrite a pre-existing .gitignore.
		return nil
	}
	if err != nil {
		return fmt.Errorf("create repository .gitignore: %w", err)
	}

	if _, err := file.WriteString(defaultGitignore); err != nil {
		file.Close()
		return fmt.Errorf("write repository .gitignore: %w", err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("close repository .gitignore: %w", err)
	}

	return nil
}

// Validate verifies that path contains a usable Git repository.
func Validate(path string) error {
	_, err := git.PlainOpen(path)
	if err != nil {
		return fmt.Errorf("open repository at %q: %w", path, err)
	}

	return nil
}

// Status returns the current Git worktree status.
func Status(path string) (git.Status, error) {
	repository, err := git.PlainOpen(path)
	if err != nil {
		return nil, fmt.Errorf("open repository at %q: %w", path, err)
	}

	worktree, err := repository.Worktree()
	if err != nil {
		return nil, fmt.Errorf("open repository worktree: %w", err)
	}

	status, err := worktree.Status()
	if err != nil {
		return nil, fmt.Errorf("read repository status: %w", err)
	}

	return status, nil
}

// Stage adds repository-relative paths to the Git index.
//
// Git repository paths always use forward slashes, including on Windows.
func Stage(repositoryPath string, repositoryPaths ...string) error {
	repository, err := git.PlainOpen(repositoryPath)
	if err != nil {
		return fmt.Errorf("open repository at %q: %w", repositoryPath, err)
	}

	worktree, err := repository.Worktree()
	if err != nil {
		return fmt.Errorf("open repository worktree: %w", err)
	}

	for _, repositoryPath := range repositoryPaths {
		normalized := filepath.ToSlash(repositoryPath)

		if _, err := worktree.Add(normalized); err != nil {
			return fmt.Errorf("stage %q: %w", normalized, err)
		}
	}

	return nil
}

var ErrNoStagedChanges = errors.New("no staged changes")

// Commit creates a commit from the current Git index.
//
// Only staged changes are committed. Deployed host files are not read or
// modified by this operation.
func Commit(
	repositoryPath string,
	message string,
) (plumbing.Hash, error) {
	if message == "" {
		return plumbing.ZeroHash, fmt.Errorf(
			"commit message must not be empty",
		)
	}

	repository, err := git.PlainOpen(repositoryPath)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf(
			"open repository at %q: %w",
			repositoryPath,
			err,
		)
	}

	configuration, err := repository.Config()
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf(
			"read repository configuration: %w",
			err,
		)
	}

	if configuration.User.Name == "" {
		return plumbing.ZeroHash, fmt.Errorf(
			"user.name is not configured; run " +
				"`dotmgr config user.name \"Your Name\"`",
		)
	}

	if configuration.User.Email == "" {
		return plumbing.ZeroHash, fmt.Errorf(
			"user.email is not configured; run " +
				"`dotmgr config user.email \"you@example.com\"`",
		)
	}

	worktree, err := repository.Worktree()
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf(
			"open repository worktree: %w",
			err,
		)
	}

	status, err := worktree.Status()
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf(
			"read repository status: %w",
			err,
		)
	}

	if !hasStagedChanges(status) {
		return plumbing.ZeroHash, ErrNoStagedChanges
	}

	now := time.Now()

	signature := object.Signature{
		Name:  configuration.User.Name,
		Email: configuration.User.Email,
		When:  now,
	}

	hash, err := worktree.Commit(message, &git.CommitOptions{
		Author:    &signature,
		Committer: &signature,
	})
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("create commit: %w", err)
	}

	return hash, nil
}

func hasStagedChanges(status git.Status) bool {
	for _, fileStatus := range status {
		if fileStatus.Staging != git.Unmodified &&
			fileStatus.Staging != git.Untracked {
			return true
		}
	}

	return false
}

// SetUserConfig writes repository-local commit identity configuration.
func SetUserConfig(
	repositoryPath string,
	key string,
	value string,
) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", key)
	}

	repository, err := git.PlainOpen(repositoryPath)
	if err != nil {
		return fmt.Errorf(
			"open repository at %q: %w",
			repositoryPath,
			err,
		)
	}

	configuration, err := repository.Config()
	if err != nil {
		return fmt.Errorf("read repository configuration: %w", err)
	}

	switch key {
	case "user.name":
		configuration.User.Name = value
	case "user.email":
		configuration.User.Email = value
	default:
		return fmt.Errorf("unsupported configuration key %q", key)
	}

	if err := repository.Storer.SetConfig(configuration); err != nil {
		return fmt.Errorf("write repository configuration: %w", err)
	}

	return nil
}

// UserConfig reads a repository-local commit identity value.
func UserConfig(
	repositoryPath string,
	key string,
) (string, error) {
	repository, err := git.PlainOpen(repositoryPath)
	if err != nil {
		return "", fmt.Errorf(
			"open repository at %q: %w",
			repositoryPath,
			err,
		)
	}

	configuration, err := repository.Config()
	if err != nil {
		return "", fmt.Errorf("read repository configuration: %w", err)
	}

	switch key {
	case "user.name":
		return configuration.User.Name, nil
	case "user.email":
		return configuration.User.Email, nil
	default:
		return "", fmt.Errorf("unsupported configuration key %q", key)
	}
}
