package store

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
)

// worktreeFile is the state of a tracked path in the working tree.
type worktreeFile struct {
	data    []byte
	mode    filemode.FileMode
	size    int64
	modTime time.Time
}

func (f *worktreeFile) hash() plumbing.Hash {
	return plumbing.ComputeHash(plumbing.BlobObject, f.data)
}

func (s *Store) absolutePath(name string) string {
	return filepath.Join(s.home, filepath.FromSlash(name))
}

// readWorktree reads name from the working tree without following symlinks.
//
// A missing file is reported as an error wrapping os.ErrNotExist. Windows has
// no executable bit, so regular files take on referenceMode there.
func (s *Store) readWorktree(
	name string,
	referenceMode filemode.FileMode,
) (*worktreeFile, error) {
	filename := s.absolutePath(name)

	info, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}

	file := &worktreeFile{modTime: info.ModTime()}

	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(filename)
		if err != nil {
			return nil, err
		}

		file.data = []byte(target)
		file.mode = filemode.Symlink

	case info.Mode().IsRegular():
		file.data, err = os.ReadFile(filename)
		if err != nil {
			return nil, err
		}

		file.mode = filemode.Regular

		switch {
		case runtime.GOOS == "windows" && isRegular(referenceMode):
			file.mode = referenceMode
		case info.Mode().Perm()&0o111 != 0:
			file.mode = filemode.Executable
		}

	default:
		return nil, fmt.Errorf("%q is not a regular file or symlink", filename)
	}

	file.size = int64(len(file.data))

	return file, nil
}

func isRegular(mode filemode.FileMode) bool {
	return mode == filemode.Regular || mode == filemode.Executable
}

func (s *Store) writeBlob(data []byte) (plumbing.Hash, error) {
	object := s.repo.Storer.NewEncodedObject()
	object.SetType(plumbing.BlobObject)
	object.SetSize(int64(len(data)))

	writer, err := object.Writer()
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("write blob: %w", err)
	}

	if _, err := writer.Write(data); err != nil {
		writer.Close()
		return plumbing.ZeroHash, fmt.Errorf("write blob: %w", err)
	}

	if err := writer.Close(); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("write blob: %w", err)
	}

	hash, err := s.repo.Storer.SetEncodedObject(object)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("store blob: %w", err)
	}

	return hash, nil
}

func (s *Store) blobContents(hash plumbing.Hash) ([]byte, error) {
	blob, err := s.repo.BlobObject(hash)
	if err != nil {
		return nil, fmt.Errorf("load blob %s: %w", hash, err)
	}

	reader, err := blob.Reader()
	if err != nil {
		return nil, fmt.Errorf("read blob %s: %w", hash, err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read blob %s: %w", hash, err)
	}

	return data, nil
}

func (s *Store) loadIndex() (*index.Index, error) {
	idx, err := s.repo.Storer.Index()
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}

	return idx, nil
}

func (s *Store) saveIndex(idx *index.Index) error {
	// Entries changed, so a cached tree would be stale.
	idx.Cache = nil

	if err := s.repo.Storer.SetIndex(idx); err != nil {
		return fmt.Errorf("write index: %w", err)
	}

	return nil
}

func setEntry(
	idx *index.Index,
	name string,
	hash plumbing.Hash,
	mode filemode.FileMode,
	size int64,
	modTime time.Time,
) {
	entry, err := idx.Entry(name)
	if err != nil {
		entry = idx.Add(name)
	}

	entry.Hash = hash
	entry.Mode = mode
	entry.Size = uint32(size)
	entry.ModifiedAt = modTime
	entry.CreatedAt = modTime
}

func indexEntry(idx *index.Index, name string) *index.Entry {
	entry, err := idx.Entry(name)
	if err != nil {
		return nil
	}

	return entry
}

// isBinary applies Git's heuristic: a NUL byte near the start.
func isBinary(data []byte) bool {
	const probe = 8000

	if len(data) > probe {
		data = data[:probe]
	}

	return bytes.IndexByte(data, 0) >= 0
}

// validateName rejects tree paths that could escape the working tree or
// touch Git metadata.
func validateName(name string) error {
	if name == "" || strings.ContainsAny(name, "\\\x00") {
		return fmt.Errorf("unsafe path %q in repository", name)
	}

	for _, part := range strings.Split(name, "/") {
		switch {
		case part == "", part == ".", part == "..":
			return fmt.Errorf("unsafe path %q in repository", name)
		case strings.EqualFold(part, ".git"):
			return fmt.Errorf("unsafe path %q in repository", name)
		}
	}

	// A variant must not escape through its path either: "..##os.linux".
	if filePath, conditions := splitVariant(name); conditions != nil {
		return validateName(filePath)
	}

	return nil
}
