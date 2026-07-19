package tracking

import (
	"os"
	"path/filepath"
	"testing"

	"git.sr.ht/~stephan/dotmgr/internal/gitstore"
	"git.sr.ht/~stephan/dotmgr/internal/manifest"
)

func TestAddCopiesRecordsAndStagesFile(t *testing.T) {
	root := t.TempDir()
	homeDirectory := filepath.Join(root, "home")
	repositoryPath := filepath.Join(root, "repository")

	if err := os.MkdirAll(homeDirectory, 0o755); err != nil {
		t.Fatalf("create home directory: %v", err)
	}

	if err := gitstore.Init(repositoryPath); err != nil {
		t.Fatalf("initialize repository: %v", err)
	}

	inputPath := filepath.Join(
		homeDirectory,
		".config",
		"example",
		"config.txt",
	)

	if err := os.MkdirAll(filepath.Dir(inputPath), 0o755); err != nil {
		t.Fatalf("create input directory: %v", err)
	}

	const inputContent = "example = true\n"

	if err := os.WriteFile(inputPath, []byte(inputContent), 0o600); err != nil {
		t.Fatalf("write input file: %v", err)
	}

	entry, err := Add(repositoryPath, homeDirectory, inputPath)
	if err != nil {
		t.Fatalf("add file: %v", err)
	}

	expectedSource := "files/home/.config/example/config.txt"
	expectedTarget := "${HOME}/.config/example/config.txt"

	if entry.Source != expectedSource {
		t.Fatalf(
			"expected source %q, got %q",
			expectedSource,
			entry.Source,
		)
	}

	if entry.Target != expectedTarget {
		t.Fatalf(
			"expected target %q, got %q",
			expectedTarget,
			entry.Target,
		)
	}

	repositoryFile := filepath.Join(
		repositoryPath,
		filepath.FromSlash(expectedSource),
	)

	copiedContent, err := os.ReadFile(repositoryFile)
	if err != nil {
		t.Fatalf("read repository copy: %v", err)
	}

	if string(copiedContent) != inputContent {
		t.Fatalf(
			"expected copied content %q, got %q",
			inputContent,
			string(copiedContent),
		)
	}

	storedManifest, err := manifest.Load(manifest.Path(repositoryPath))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	if len(storedManifest.Files) != 1 {
		t.Fatalf(
			"expected one manifest entry, got %d",
			len(storedManifest.Files),
		)
	}

	if storedManifest.Files[0] != entry {
		t.Fatalf(
			"expected manifest entry %#v, got %#v",
			entry,
			storedManifest.Files[0],
		)
	}

	status, err := gitstore.Status(repositoryPath)
	if err != nil {
		t.Fatalf("read repository status: %v", err)
	}

	if status.IsClean() {
		t.Fatal("expected staged repository changes")
	}
}

func TestAddUpdatesExistingFile(t *testing.T) {
	root := t.TempDir()
	homeDirectory := filepath.Join(root, "home")
	repositoryPath := filepath.Join(root, "repository")

	if err := os.MkdirAll(homeDirectory, 0o755); err != nil {
		t.Fatalf("create home directory: %v", err)
	}

	if err := gitstore.Init(repositoryPath); err != nil {
		t.Fatalf("initialize repository: %v", err)
	}

	inputPath := filepath.Join(homeDirectory, ".example")

	if err := os.WriteFile(inputPath, []byte("first\n"), 0o600); err != nil {
		t.Fatalf("write first version: %v", err)
	}

	if _, err := Add(repositoryPath, homeDirectory, inputPath); err != nil {
		t.Fatalf("add first version: %v", err)
	}

	if err := os.WriteFile(inputPath, []byte("second\n"), 0o600); err != nil {
		t.Fatalf("write second version: %v", err)
	}

	if _, err := Add(repositoryPath, homeDirectory, inputPath); err != nil {
		t.Fatalf("add second version: %v", err)
	}

	repositoryFile := filepath.Join(
		repositoryPath,
		"files",
		"home",
		".example",
	)

	content, err := os.ReadFile(repositoryFile)
	if err != nil {
		t.Fatalf("read updated repository file: %v", err)
	}

	if string(content) != "second\n" {
		t.Fatalf("expected updated content, got %q", string(content))
	}

	storedManifest, err := manifest.Load(manifest.Path(repositoryPath))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	if len(storedManifest.Files) != 1 {
		t.Fatalf(
			"expected one manifest entry, got %d",
			len(storedManifest.Files),
		)
	}
}

func TestAddRejectsFileOutsideHome(t *testing.T) {
	root := t.TempDir()
	homeDirectory := filepath.Join(root, "home")
	repositoryPath := filepath.Join(root, "repository")
	outsideFile := filepath.Join(root, "outside.txt")

	if err := os.MkdirAll(homeDirectory, 0o755); err != nil {
		t.Fatalf("create home directory: %v", err)
	}

	if err := gitstore.Init(repositoryPath); err != nil {
		t.Fatalf("initialize repository: %v", err)
	}

	if err := os.WriteFile(outsideFile, []byte("outside\n"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	if _, err := Add(
		repositoryPath,
		homeDirectory,
		outsideFile,
	); err == nil {
		t.Fatal("expected a file outside home to be rejected")
	}
}
