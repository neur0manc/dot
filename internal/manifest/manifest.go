package manifest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/pelletier/go-toml/v2"

	"git.sr.ht/~stephan/dotmgr/internal/atomicfile"
)

const (
	FileName       = "manifest.toml"
	CurrentVersion = 1
)

type File struct {
	Source string `toml:"source"`
	Target string `toml:"target"`
}

type Manifest struct {
	Version int    `toml:"version"`
	Files   []File `toml:"files"`
}

// Load reads a manifest. A missing file is treated as an empty manifest.
func Load(filename string) (*Manifest, error) {
	data, err := os.ReadFile(filename)
	if errors.Is(err, os.ErrNotExist) {
		return &Manifest{
			Version: CurrentVersion,
			Files:   []File{},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read manifest %q: %w", filename, err)
	}

	var result Manifest
	if err := toml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode manifest %q: %w", filename, err)
	}

	if result.Version != CurrentVersion {
		return nil, fmt.Errorf(
			"unsupported manifest version %d; expected %d",
			result.Version,
			CurrentVersion,
		)
	}

	if result.Files == nil {
		result.Files = []File{}
	}

	return &result, nil
}

// Upsert adds entry or replaces an existing entry with the same target.
func (m *Manifest) Upsert(entry File) {
	for index := range m.Files {
		if m.Files[index].Target == entry.Target {
			m.Files[index] = entry
			m.sort()
			return
		}
	}

	m.Files = append(m.Files, entry)
	m.sort()
}

func (m *Manifest) sort() {
	sort.Slice(m.Files, func(left, right int) bool {
		return m.Files[left].Target < m.Files[right].Target
	})
}

// Save writes the manifest in a deterministic, human-readable form.
func (m *Manifest) Save(filename string) error {
	if m.Version == 0 {
		m.Version = CurrentVersion
	}

	m.sort()

	data, err := toml.Marshal(m)
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}

	if len(data) == 0 || data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}

	if err := atomicfile.WriteFile(filename, data, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	return nil
}

// Path returns the location of the manifest in repositoryPath.
func Path(repositoryPath string) string {
	return filepath.Join(repositoryPath, FileName)
}
