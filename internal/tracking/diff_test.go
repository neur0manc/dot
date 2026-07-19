package tracking

import (
	"os"
	"strings"
	"testing"
)

func TestLocalDiffsShowsHostModification(t *testing.T) {
	repositoryPath, homeDirectory, inputPath :=
		createTrackedTestFile(t, "set number\n")

	if err := os.WriteFile(
		inputPath,
		[]byte("set number\nset ruler\n"),
		0o600,
	); err != nil {
		t.Fatalf("modify deployed file: %v", err)
	}

	diffs, err := LocalDiffs(repositoryPath, homeDirectory)
	if err != nil {
		t.Fatalf("create local diffs: %v", err)
	}

	if len(diffs) != 1 {
		t.Fatalf("expected one diff, got %d", len(diffs))
	}

	fileDiff := diffs[0]

	if fileDiff.State != LocalModified {
		t.Fatalf(
			"expected state %q, got %q",
			LocalModified,
			fileDiff.State,
		)
	}

	if fileDiff.Binary {
		t.Fatal("expected a textual diff")
	}

	if !strings.Contains(fileDiff.Patch, "+set ruler") {
		t.Fatalf(
			"expected added line in patch, got:\n%s",
			fileDiff.Patch,
		)
	}

	if !strings.Contains(
		fileDiff.Patch,
		"--- repository:files/home/.vim/vimrc",
	) {
		t.Fatalf(
			"expected repository header, got:\n%s",
			fileDiff.Patch,
		)
	}

	if !strings.Contains(
		fileDiff.Patch,
		"+++ host:${HOME}/.vim/vimrc",
	) {
		t.Fatalf(
			"expected host header, got:\n%s",
			fileDiff.Patch,
		)
	}
}

func TestLocalDiffsOmitsSynchronizedFiles(t *testing.T) {
	repositoryPath, homeDirectory, _ :=
		createTrackedTestFile(t, "set number\n")

	diffs, err := LocalDiffs(repositoryPath, homeDirectory)
	if err != nil {
		t.Fatalf("create local diffs: %v", err)
	}

	if len(diffs) != 0 {
		t.Fatalf("expected no diffs, got %d", len(diffs))
	}
}

func TestLocalDiffsShowsNotAppliedFile(t *testing.T) {
	repositoryPath, homeDirectory, inputPath :=
		createTrackedTestFile(t, "set number\n")

	if err := os.Remove(inputPath); err != nil {
		t.Fatalf("remove deployed file: %v", err)
	}

	diffs, err := LocalDiffs(repositoryPath, homeDirectory)
	if err != nil {
		t.Fatalf("create local diffs: %v", err)
	}

	if len(diffs) != 1 {
		t.Fatalf("expected one diff, got %d", len(diffs))
	}

	fileDiff := diffs[0]

	if fileDiff.State != LocalNotApplied {
		t.Fatalf(
			"expected state %q, got %q",
			LocalNotApplied,
			fileDiff.State,
		)
	}

	if !strings.Contains(fileDiff.Patch, "-set number") {
		t.Fatalf(
			"expected repository content to be removed on host side, got:\n%s",
			fileDiff.Patch,
		)
	}
}

func TestLocalDiffsDoesNotRenderBinaryContent(t *testing.T) {
	repositoryPath, homeDirectory, inputPath :=
		createTrackedTestFile(t, "initial\n")

	if err := os.WriteFile(
		inputPath,
		[]byte{0x00, 0x01, 0x02},
		0o600,
	); err != nil {
		t.Fatalf("write binary deployed file: %v", err)
	}

	diffs, err := LocalDiffs(repositoryPath, homeDirectory)
	if err != nil {
		t.Fatalf("create local diffs: %v", err)
	}

	if len(diffs) != 1 {
		t.Fatalf("expected one diff, got %d", len(diffs))
	}

	if !diffs[0].Binary {
		t.Fatal("expected binary difference")
	}

	if diffs[0].Patch != "" {
		t.Fatalf(
			"expected no textual patch, got %q",
			diffs[0].Patch,
		)
	}
}
