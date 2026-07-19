package tracking

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"git.sr.ht/~stephan/dotmgr/internal/gitstore"
)

func TestApplyCreatesMissingTarget(t *testing.T) {
	repositoryPath, homeDirectory, targetPath :=
		createTrackedTestFile(t, "set number\n")

	if err := os.Remove(targetPath); err != nil {
		t.Fatalf("remove target: %v", err)
	}

	result, err := Apply(
		repositoryPath,
		homeDirectory,
		false,
	)
	if err != nil {
		t.Fatalf("apply managed files: %v", err)
	}

	if len(result.Applied) != 1 {
		t.Fatalf(
			"expected one applied file, got %d",
			len(result.Applied),
		)
	}

	if len(result.Overwritten) != 0 {
		t.Fatalf(
			"expected no overwritten files, got %d",
			len(result.Overwritten),
		)
	}

	if len(result.Skipped) != 0 {
		t.Fatalf(
			"expected no skipped files, got %d",
			len(result.Skipped),
		)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read applied target: %v", err)
	}

	if string(data) != "set number\n" {
		t.Fatalf(
			"expected applied content %q, got %q",
			"set number\n",
			string(data),
		)
	}
}

func TestApplySkipsIdenticalTarget(t *testing.T) {
	repositoryPath, homeDirectory, _ :=
		createTrackedTestFile(t, "set number\n")

	result, err := Apply(
		repositoryPath,
		homeDirectory,
		false,
	)
	if err != nil {
		t.Fatalf("apply managed files: %v", err)
	}

	if len(result.Applied) != 0 {
		t.Fatalf(
			"expected no applied files, got %d",
			len(result.Applied),
		)
	}

	if len(result.Overwritten) != 0 {
		t.Fatalf(
			"expected no overwritten files, got %d",
			len(result.Overwritten),
		)
	}

	if len(result.Skipped) != 1 {
		t.Fatalf(
			"expected one skipped file, got %d",
			len(result.Skipped),
		)
	}
}

func TestApplyRejectsModifiedTarget(t *testing.T) {
	repositoryPath, homeDirectory, targetPath :=
		createTrackedTestFile(t, "repository content\n")

	if err := os.WriteFile(
		targetPath,
		[]byte("host content\n"),
		0o600,
	); err != nil {
		t.Fatalf("modify target: %v", err)
	}

	result, err := Apply(
		repositoryPath,
		homeDirectory,
		false,
	)
	if !errors.Is(err, ErrApplyConflicts) {
		t.Fatalf(
			"expected ErrApplyConflicts, got %v",
			err,
		)
	}

	if len(result.Applied) != 0 {
		t.Fatalf(
			"expected no applied files, got %d",
			len(result.Applied),
		)
	}

	if len(result.Overwritten) != 0 {
		t.Fatalf(
			"expected no overwritten files, got %d",
			len(result.Overwritten),
		)
	}

	data, readErr := os.ReadFile(targetPath)
	if readErr != nil {
		t.Fatalf("read preserved target: %v", readErr)
	}

	if string(data) != "host content\n" {
		t.Fatalf(
			"expected host content to be preserved, got %q",
			string(data),
		)
	}
}

func TestApplyForceOverwritesModifiedTarget(t *testing.T) {
	repositoryPath, homeDirectory, targetPath :=
		createTrackedTestFile(t, "repository content\n")

	if err := os.WriteFile(
		targetPath,
		[]byte("host content\n"),
		0o600,
	); err != nil {
		t.Fatalf("modify target: %v", err)
	}

	result, err := Apply(
		repositoryPath,
		homeDirectory,
		true,
	)
	if err != nil {
		t.Fatalf("force apply managed files: %v", err)
	}

	if len(result.Applied) != 0 {
		t.Fatalf(
			"expected no newly applied files, got %d",
			len(result.Applied),
		)
	}

	if len(result.Overwritten) != 1 {
		t.Fatalf(
			"expected one overwritten file, got %d",
			len(result.Overwritten),
		)
	}

	if len(result.Skipped) != 0 {
		t.Fatalf(
			"expected no skipped files, got %d",
			len(result.Skipped),
		)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read overwritten target: %v", err)
	}

	if string(data) != "repository content\n" {
		t.Fatalf(
			"expected repository content, got %q",
			string(data),
		)
	}
}

func TestApplyConflictPreflightPreventsOtherWrites(t *testing.T) {
	root := t.TempDir()
	homeDirectory := filepath.Join(root, "home")
	repositoryPath := filepath.Join(root, "repository")

	if err := os.MkdirAll(homeDirectory, 0o755); err != nil {
		t.Fatalf("create home directory: %v", err)
	}

	if err := gitstore.Init(repositoryPath); err != nil {
		t.Fatalf("initialize repository: %v", err)
	}

	firstPath := filepath.Join(homeDirectory, ".first")
	secondPath := filepath.Join(homeDirectory, ".second")

	if err := os.WriteFile(
		firstPath,
		[]byte("first repository content\n"),
		0o600,
	); err != nil {
		t.Fatalf("write first file: %v", err)
	}

	if err := os.WriteFile(
		secondPath,
		[]byte("second repository content\n"),
		0o600,
	); err != nil {
		t.Fatalf("write second file: %v", err)
	}

	if _, err := Add(
		repositoryPath,
		homeDirectory,
		firstPath,
	); err != nil {
		t.Fatalf("add first file: %v", err)
	}

	if _, err := Add(
		repositoryPath,
		homeDirectory,
		secondPath,
	); err != nil {
		t.Fatalf("add second file: %v", err)
	}

	if err := os.WriteFile(
		firstPath,
		[]byte("conflicting host content\n"),
		0o600,
	); err != nil {
		t.Fatalf("create conflict: %v", err)
	}

	if err := os.Remove(secondPath); err != nil {
		t.Fatalf("remove second target: %v", err)
	}

	result, err := Apply(
		repositoryPath,
		homeDirectory,
		false,
	)
	if !errors.Is(err, ErrApplyConflicts) {
		t.Fatalf(
			"expected ErrApplyConflicts, got %v",
			err,
		)
	}

	if len(result.Applied) != 0 {
		t.Fatalf(
			"expected no files to be applied, got %d",
			len(result.Applied),
		)
	}

	if len(result.Overwritten) != 0 {
		t.Fatalf(
			"expected no files to be overwritten, got %d",
			len(result.Overwritten),
		)
	}

	firstData, readErr := os.ReadFile(firstPath)
	if readErr != nil {
		t.Fatalf("read conflicting target: %v", readErr)
	}

	if string(firstData) != "conflicting host content\n" {
		t.Fatalf(
			"expected conflicting content to remain unchanged, got %q",
			string(firstData),
		)
	}

	if _, statErr := os.Stat(secondPath); !errors.Is(
		statErr,
		os.ErrNotExist,
	) {
		t.Fatalf(
			"expected second target to remain absent, got %v",
			statErr,
		)
	}
}

func TestApplyRejectsMissingRepositorySourceBeforeWriting(t *testing.T) {
	root := t.TempDir()
	homeDirectory := filepath.Join(root, "home")
	repositoryPath := filepath.Join(root, "repository")

	if err := os.MkdirAll(homeDirectory, 0o755); err != nil {
		t.Fatalf("create home directory: %v", err)
	}

	if err := gitstore.Init(repositoryPath); err != nil {
		t.Fatalf("initialize repository: %v", err)
	}

	firstPath := filepath.Join(homeDirectory, ".first")
	secondPath := filepath.Join(homeDirectory, ".second")

	if err := os.WriteFile(
		firstPath,
		[]byte("first content\n"),
		0o600,
	); err != nil {
		t.Fatalf("write first file: %v", err)
	}

	if err := os.WriteFile(
		secondPath,
		[]byte("second content\n"),
		0o600,
	); err != nil {
		t.Fatalf("write second file: %v", err)
	}

	firstEntry, err := Add(
		repositoryPath,
		homeDirectory,
		firstPath,
	)
	if err != nil {
		t.Fatalf("add first file: %v", err)
	}

	if _, err := Add(
		repositoryPath,
		homeDirectory,
		secondPath,
	); err != nil {
		t.Fatalf("add second file: %v", err)
	}

	if err := os.Remove(firstPath); err != nil {
		t.Fatalf("remove first target: %v", err)
	}

	if err := os.Remove(secondPath); err != nil {
		t.Fatalf("remove second target: %v", err)
	}

	firstSourcePath := filepath.Join(
		repositoryPath,
		filepath.FromSlash(firstEntry.Source),
	)

	if err := os.Remove(firstSourcePath); err != nil {
		t.Fatalf("remove first repository source: %v", err)
	}

	_, err = Apply(
		repositoryPath,
		homeDirectory,
		false,
	)
	if err == nil {
		t.Fatal("expected missing repository source to be rejected")
	}

	if _, statErr := os.Stat(secondPath); !errors.Is(
		statErr,
		os.ErrNotExist,
	) {
		t.Fatalf(
			"expected second target to remain absent, got %v",
			statErr,
		)
	}
}

func TestApplyRejectsSymlinkedTargetParent(t *testing.T) {
	repositoryPath, homeDirectory, targetPath :=
		createTrackedTestFile(t, "repository content\n")

	if err := os.Remove(targetPath); err != nil {
		t.Fatalf("remove target: %v", err)
	}

	targetDirectory := filepath.Dir(targetPath)

	if err := os.Remove(targetDirectory); err != nil {
		t.Fatalf("remove target directory: %v", err)
	}

	outsideDirectory := filepath.Join(t.TempDir(), "outside")

	if err := os.MkdirAll(outsideDirectory, 0o755); err != nil {
		t.Fatalf("create outside directory: %v", err)
	}

	if err := os.Symlink(
		outsideDirectory,
		targetDirectory,
	); err != nil {
		t.Skipf(
			"symbolic links are unavailable in this environment: %v",
			err,
		)
	}

	_, err := Apply(
		repositoryPath,
		homeDirectory,
		false,
	)
	if err == nil {
		t.Fatal("expected symlinked target parent to be rejected")
	}

	outsideTarget := filepath.Join(
		outsideDirectory,
		filepath.Base(targetPath),
	)

	if _, statErr := os.Stat(outsideTarget); !errors.Is(
		statErr,
		os.ErrNotExist,
	) {
		t.Fatalf(
			"expected no file outside home, got %v",
			statErr,
		)
	}
}
