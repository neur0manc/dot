package gitstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitIgnoresDSStoreFiles(t *testing.T) {
	repositoryPath := filepath.Join(t.TempDir(), "repository")

	if err := Init(repositoryPath); err != nil {
		t.Fatalf("initialize repository: %v", err)
	}

	filesDirectory := filepath.Join(repositoryPath, "files")

	if err := os.MkdirAll(filesDirectory, 0o755); err != nil {
		t.Fatalf("create files directory: %v", err)
	}

	finderFiles := []string{
		filepath.Join(repositoryPath, ".DS_Store"),
		filepath.Join(filesDirectory, ".DS_Store"),
	}

	for _, filename := range finderFiles {
		if err := os.WriteFile(filename, []byte("finder data"), 0o644); err != nil {
			t.Fatalf("write %q: %v", filename, err)
		}
	}

	status, err := Status(repositoryPath)
	if err != nil {
		t.Fatalf("read status: %v", err)
	}

	if _, found := status[".DS_Store"]; found {
		t.Fatal("root .DS_Store should be ignored")
	}

	if _, found := status["files/.DS_Store"]; found {
		t.Fatal("nested .DS_Store should be ignored")
	}

	if _, found := status[".gitignore"]; !found {
		t.Fatal(".gitignore should be staged")
	}
}
