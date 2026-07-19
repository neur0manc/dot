package gitstore

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

func TestPullFastForwardsCurrentBranch(t *testing.T) {
	sourcePath, _, clonePath, _ :=
		createPullTestRepositories(t)

	updatedHash := commitPullTestFile(
		t,
		sourcePath,
		"remote update\n",
		"Update example file",
	)

	pushed, err := Push(
		sourcePath,
		"origin",
		io.Discard,
	)
	if err != nil {
		t.Fatalf("push updated source: %v", err)
	}

	if !pushed {
		t.Fatal("expected updated source to be pushed")
	}

	updated, err := Pull(
		clonePath,
		"origin",
		io.Discard,
	)
	if err != nil {
		t.Fatalf("pull update: %v", err)
	}

	if !updated {
		t.Fatal("expected pull to update the clone")
	}

	clonedRepository, err := git.PlainOpen(clonePath)
	if err != nil {
		t.Fatalf("open cloned repository: %v", err)
	}

	clonedHead, err := clonedRepository.Head()
	if err != nil {
		t.Fatalf("resolve cloned HEAD: %v", err)
	}

	if clonedHead.Hash() != updatedHash {
		t.Fatalf(
			"expected cloned HEAD %s, got %s",
			updatedHash,
			clonedHead.Hash(),
		)
	}

	data, err := os.ReadFile(
		filepath.Join(clonePath, "example.txt"),
	)
	if err != nil {
		t.Fatalf("read updated cloned file: %v", err)
	}

	if string(data) != "remote update\n" {
		t.Fatalf(
			"expected updated content %q, got %q",
			"remote update\n",
			string(data),
		)
	}

	updated, err = Pull(
		clonePath,
		"origin",
		io.Discard,
	)
	if err != nil {
		t.Fatalf("repeat pull: %v", err)
	}

	if updated {
		t.Fatal("expected repeated pull to be up to date")
	}
}

func TestPullRejectsDirtyRepositoryBeforeFetch(t *testing.T) {
	sourcePath, _, clonePath, initialHash :=
		createPullTestRepositories(t)

	commitPullTestFile(
		t,
		sourcePath,
		"remote update\n",
		"Update remote file",
	)

	if _, err := Push(
		sourcePath,
		"origin",
		io.Discard,
	); err != nil {
		t.Fatalf("push remote update: %v", err)
	}

	cloneFile := filepath.Join(clonePath, "example.txt")

	if err := os.WriteFile(
		cloneFile,
		[]byte("uncommitted local change\n"),
		0o644,
	); err != nil {
		t.Fatalf("modify cloned worktree: %v", err)
	}

	updated, err := Pull(
		clonePath,
		"origin",
		io.Discard,
	)
	if !errors.Is(err, ErrPullDirtyRepository) {
		t.Fatalf(
			"expected ErrPullDirtyRepository, got %v",
			err,
		)
	}

	if updated {
		t.Fatal("expected dirty pull not to update the repository")
	}

	clonedRepository, err := git.PlainOpen(clonePath)
	if err != nil {
		t.Fatalf("open cloned repository: %v", err)
	}

	clonedHead, err := clonedRepository.Head()
	if err != nil {
		t.Fatalf("resolve cloned HEAD: %v", err)
	}

	if clonedHead.Hash() != initialHash {
		t.Fatalf(
			"expected local HEAD to remain %s, got %s",
			initialHash,
			clonedHead.Hash(),
		)
	}

	remoteReference := plumbing.NewRemoteReferenceName(
		"origin",
		clonedHead.Name().Short(),
	)

	trackingReference, err := clonedRepository.Reference(
		remoteReference,
		true,
	)
	if err != nil {
		t.Fatalf("resolve remote-tracking branch: %v", err)
	}

	if trackingReference.Hash() != initialHash {
		t.Fatalf(
			"expected dirty check to occur before fetch; "+
				"remote-tracking hash is %s, expected %s",
			trackingReference.Hash(),
			initialHash,
		)
	}

	data, err := os.ReadFile(cloneFile)
	if err != nil {
		t.Fatalf("read preserved local change: %v", err)
	}

	if string(data) != "uncommitted local change\n" {
		t.Fatalf(
			"expected local change to be preserved, got %q",
			string(data),
		)
	}
}

func TestPullRejectsDivergentHistory(t *testing.T) {
	sourcePath, _, clonePath, _ :=
		createPullTestRepositories(t)

	if err := SetUserConfig(
		clonePath,
		"user.name",
		"Clone User",
	); err != nil {
		t.Fatalf("set clone user name: %v", err)
	}

	if err := SetUserConfig(
		clonePath,
		"user.email",
		"clone@example.com",
	); err != nil {
		t.Fatalf("set clone user email: %v", err)
	}

	cloneFile := filepath.Join(clonePath, "example.txt")

	if err := os.WriteFile(
		cloneFile,
		[]byte("local committed update\n"),
		0o644,
	); err != nil {
		t.Fatalf("write local update: %v", err)
	}

	if err := Stage(clonePath, "example.txt"); err != nil {
		t.Fatalf("stage local update: %v", err)
	}

	localHash, err := Commit(
		clonePath,
		"Create local commit",
	)
	if err != nil {
		t.Fatalf("commit local update: %v", err)
	}

	remoteHash := commitPullTestFile(
		t,
		sourcePath,
		"remote committed update\n",
		"Create remote commit",
	)

	if _, err := Push(
		sourcePath,
		"origin",
		io.Discard,
	); err != nil {
		t.Fatalf("push remote commit: %v", err)
	}

	updated, err := Pull(
		clonePath,
		"origin",
		io.Discard,
	)
	if !errors.Is(err, ErrPullNonFastForward) {
		t.Fatalf(
			"expected ErrPullNonFastForward, got %v",
			err,
		)
	}

	if updated {
		t.Fatal("expected divergent pull not to update the branch")
	}

	clonedRepository, err := git.PlainOpen(clonePath)
	if err != nil {
		t.Fatalf("open cloned repository: %v", err)
	}

	clonedHead, err := clonedRepository.Head()
	if err != nil {
		t.Fatalf("resolve cloned HEAD: %v", err)
	}

	if clonedHead.Hash() != localHash {
		t.Fatalf(
			"expected local HEAD to remain %s, got %s",
			localHash,
			clonedHead.Hash(),
		)
	}

	remoteReference := plumbing.NewRemoteReferenceName(
		"origin",
		clonedHead.Name().Short(),
	)

	trackingReference, err := clonedRepository.Reference(
		remoteReference,
		true,
	)
	if err != nil {
		t.Fatalf("resolve fetched remote branch: %v", err)
	}

	if trackingReference.Hash() != remoteHash {
		t.Fatalf(
			"expected fetched remote hash %s, got %s",
			remoteHash,
			trackingReference.Hash(),
		)
	}

	data, err := os.ReadFile(cloneFile)
	if err != nil {
		t.Fatalf("read preserved local file: %v", err)
	}

	if string(data) != "local committed update\n" {
		t.Fatalf(
			"expected local worktree content to remain unchanged, got %q",
			string(data),
		)
	}
}

func createPullTestRepositories(
	t *testing.T,
) (
	sourcePath string,
	remotePath string,
	clonePath string,
	initialHash plumbing.Hash,
) {
	t.Helper()

	root := t.TempDir()

	sourcePath = filepath.Join(root, "source")
	remotePath = filepath.Join(root, "remote.git")
	clonePath = filepath.Join(root, "clone")

	if err := Init(sourcePath); err != nil {
		t.Fatalf("initialize source repository: %v", err)
	}

	if err := SetUserConfig(
		sourcePath,
		"user.name",
		"Test User",
	); err != nil {
		t.Fatalf("set source user name: %v", err)
	}

	if err := SetUserConfig(
		sourcePath,
		"user.email",
		"test@example.com",
	); err != nil {
		t.Fatalf("set source user email: %v", err)
	}

	initialHash = commitPullTestFile(
		t,
		sourcePath,
		"initial content\n",
		"Create initial commit",
	)

	if _, err := git.PlainInit(remotePath, true); err != nil {
		t.Fatalf("initialize bare remote: %v", err)
	}

	if err := AddRemote(
		sourcePath,
		"origin",
		remotePath,
	); err != nil {
		t.Fatalf("add source remote: %v", err)
	}

	pushed, err := Push(
		sourcePath,
		"origin",
		io.Discard,
	)
	if err != nil {
		t.Fatalf("push initial commit: %v", err)
	}

	if !pushed {
		t.Fatal("expected initial commit to be pushed")
	}

	if err := Clone(
		clonePath,
		remotePath,
		io.Discard,
	); err != nil {
		t.Fatalf("clone repository: %v", err)
	}

	return sourcePath, remotePath, clonePath, initialHash
}

func commitPullTestFile(
	t *testing.T,
	repositoryPath string,
	content string,
	message string,
) plumbing.Hash {
	t.Helper()

	filename := filepath.Join(repositoryPath, "example.txt")

	if err := os.WriteFile(
		filename,
		[]byte(content),
		0o644,
	); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	if err := Stage(repositoryPath, "example.txt"); err != nil {
		t.Fatalf("stage test file: %v", err)
	}

	hash, err := Commit(repositoryPath, message)
	if err != nil {
		t.Fatalf("commit test file: %v", err)
	}

	return hash
}
