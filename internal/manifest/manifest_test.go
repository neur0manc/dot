package manifest

import (
	"path/filepath"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	filename := filepath.Join(t.TempDir(), FileName)

	input := &Manifest{
		Version: CurrentVersion,
		Files: []File{
			{
				Source: "files/home/.gitconfig",
				Target: "${HOME}/.gitconfig",
			},
		},
	}

	if err := input.Save(filename); err != nil {
		t.Fatalf("save manifest: %v", err)
	}

	output, err := Load(filename)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	if output.Version != CurrentVersion {
		t.Fatalf(
			"expected version %d, got %d",
			CurrentVersion,
			output.Version,
		)
	}

	if len(output.Files) != 1 {
		t.Fatalf("expected one file, got %d", len(output.Files))
	}

	if output.Files[0] != input.Files[0] {
		t.Fatalf(
			"expected entry %#v, got %#v",
			input.Files[0],
			output.Files[0],
		)
	}
}

func TestUpsertReplacesExistingTarget(t *testing.T) {
	value := &Manifest{
		Version: CurrentVersion,
		Files: []File{
			{
				Source: "old-source",
				Target: "${HOME}/config",
			},
		},
	}

	value.Upsert(File{
		Source: "new-source",
		Target: "${HOME}/config",
	})

	if len(value.Files) != 1 {
		t.Fatalf("expected one file, got %d", len(value.Files))
	}

	if value.Files[0].Source != "new-source" {
		t.Fatalf(
			"expected updated source, got %q",
			value.Files[0].Source,
		)
	}
}
