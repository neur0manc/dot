package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/filemode"

	"git.sr.ht/~stephan/dotmgr/internal/atomicfile"
)

// Alternate files let one path have different content per system. Tracked
// files named
//
//	<name>##<condition>[,<condition>...]
//
// are alternates of <name>. Alt picks the best matching alternate and makes
// <name> a symbolic link to it (a copy on Windows). A condition is one of
//
//	default             always matches, lowest priority
//	arch.<arch>         amd64, arm64, ...
//	class.<class>       the value of `dotmgr config local.class`
//	os.<os>             linux, darwin (or macos), windows
//	hostname.<host>     the host name without domain
//	user.<user>         the current user
//
// Matching ignores case. All conditions of an alternate must match; among
// the matching alternates the one with the highest total weight wins.
const alternateSeparator = "##"

// Weights of the conditions: the more specific, the heavier.
const (
	weightDefault  = 1
	weightArch     = 2
	weightClass    = 4
	weightOS       = 8
	weightHostname = 16
	weightUser     = 32
)

const altStateFile = "dotmgr-alt.json"

// ClassConfigKey is the configuration key holding the machine's class.
const ClassConfigKey = "local.class"

// Facts describe the machine that alternates are matched against.
type Facts struct {
	OS       string
	Arch     string
	Hostname string
	User     string
	Class    string

	// UseCopies makes Alt copy files instead of creating symbolic links.
	UseCopies bool
}

// SystemFacts describes the current machine.
func (s *Store) SystemFacts() (Facts, error) {
	facts := Facts{
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		UseCopies: runtime.GOOS == "windows",
	}

	hostname, err := os.Hostname()
	if err != nil {
		return facts, fmt.Errorf("determine host name: %w", err)
	}

	facts.Hostname, _, _ = strings.Cut(hostname, ".")

	if current, err := user.Current(); err == nil {
		name := current.Username
		if index := strings.LastIndexAny(name, `\/`); index >= 0 {
			name = name[index+1:] // DOMAIN\user on Windows
		}

		facts.User = name
	}

	facts.Class, err = s.class()
	if err != nil {
		return facts, err
	}

	return facts, nil
}

func (s *Store) class() (string, error) {
	configuration, err := s.repo.Config()
	if err != nil {
		return "", fmt.Errorf("read repository configuration: %w", err)
	}

	return configuration.Raw.Section("dotmgr").Option("class"), nil
}

// parseAlternate splits an alternate's name into the generated file's name
// and its conditions.
func parseAlternate(name string) (string, []string, bool) {
	directory, file := path.Split(name)

	index := strings.Index(file, alternateSeparator)
	if index <= 0 || index+len(alternateSeparator) == len(file) {
		return "", nil, false
	}

	conditions := strings.Split(file[index+len(alternateSeparator):], ",")

	return directory + file[:index], conditions, true
}

func normalizeOS(value string) string {
	switch strings.ToLower(value) {
	case "macos", "osx", "mac":
		return "darwin"
	default:
		return strings.ToLower(value)
	}
}

func normalizeArch(value string) string {
	switch strings.ToLower(value) {
	case "x86_64", "x64":
		return "amd64"
	case "aarch64":
		return "arm64"
	default:
		return strings.ToLower(value)
	}
}

// score returns the weight of an alternate on this machine, or false if one
// of its conditions does not match.
func (f Facts) score(conditions []string) (int, bool) {
	total := 0

	for _, condition := range conditions {
		if strings.EqualFold(condition, "default") {
			total += weightDefault
			continue
		}

		key, value, found := strings.Cut(condition, ".")
		if !found || value == "" {
			return 0, false
		}

		var matches bool
		var weight int

		switch strings.ToLower(key) {
		case "os":
			matches, weight = normalizeOS(value) == normalizeOS(f.OS), weightOS
		case "arch":
			matches, weight = normalizeArch(value) == normalizeArch(f.Arch), weightArch
		case "class":
			matches, weight = f.Class != "" && strings.EqualFold(value, f.Class), weightClass
		case "hostname":
			matches, weight = strings.EqualFold(value, f.Hostname), weightHostname
		case "user":
			matches, weight = strings.EqualFold(value, f.User), weightUser
		}

		if !matches {
			return 0, false
		}

		total += weight
	}

	return total, true
}

// AltChange describes one generated file.
type AltChange struct {
	Name   string // the generated file
	Source string // the alternate it comes from
	Copy   bool   // a copy rather than a link
}

// AltResult summarizes an Alt run.
type AltResult struct {
	Created   []AltChange // new or replaced files
	Unchanged []AltChange
	Removed   []string // generated files without a matching alternate now
	Conflicts []string // existing files that dotmgr did not create
}

type altRecord struct {
	Source string `json:"source"`
	Target string `json:"target,omitempty"` // symlink target
	Hash   string `json:"hash,omitempty"`   // content hash of a copy
}

type altState struct {
	Files map[string]altRecord `json:"files"`
}

func (s *Store) loadAltState() (altState, error) {
	state := altState{Files: map[string]altRecord{}}

	data, err := os.ReadFile(filepath.Join(s.gitDir, altStateFile))
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("read alternate file state: %w", err)
	}

	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("decode alternate file state: %w", err)
	}

	if state.Files == nil {
		state.Files = map[string]altRecord{}
	}

	return state, nil
}

func (s *Store) saveAltState(state altState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode alternate file state: %w", err)
	}

	err = atomicfile.WriteFile(
		filepath.Join(s.gitDir, altStateFile),
		append(data, '\n'),
		0o644,
	)
	if err != nil {
		return fmt.Errorf("write alternate file state: %w", err)
	}

	return nil
}

// Alt creates, updates and removes the generated files of all tracked
// alternates. Existing files that dotmgr did not create are left alone and
// reported as conflicts unless force is set.
func (s *Store) Alt(facts Facts, force bool) (AltResult, error) {
	var result AltResult

	tracked, err := s.Tracked()
	if err != nil {
		return result, err
	}

	type candidate struct {
		name   string
		weight int
	}

	winners := map[string]candidate{}

	for _, name := range tracked {
		base, conditions, ok := parseAlternate(name)
		if !ok {
			continue
		}

		weight, matches := facts.score(conditions)
		if !matches {
			continue
		}

		best, found := winners[base]
		if !found || weight > best.weight {
			winners[base] = candidate{name: name, weight: weight}
		}
	}

	state, err := s.loadAltState()
	if err != nil {
		return result, err
	}

	bases := make([]string, 0, len(winners))
	for base := range winners {
		bases = append(bases, base)
	}
	sort.Strings(bases)

	for _, base := range bases {
		source := winners[base].name

		change, record, outcome, err := s.generate(base, source, state, facts, force)
		if err != nil {
			return result, err
		}

		switch outcome {
		case altCreated:
			result.Created = append(result.Created, change)
			state.Files[base] = record
		case altUnchanged:
			result.Unchanged = append(result.Unchanged, change)
			state.Files[base] = record
		case altConflict:
			result.Conflicts = append(result.Conflicts, base)
		}
	}

	var stale []string
	for base := range state.Files {
		if _, selected := winners[base]; !selected {
			stale = append(stale, base)
		}
	}
	sort.Strings(stale)

	for _, base := range stale {
		if s.ownsGenerated(base, state.Files[base]) {
			if err := os.Remove(s.absolutePath(base)); err != nil &&
				!errors.Is(err, os.ErrNotExist) {
				return result, fmt.Errorf("remove %q: %w", base, err)
			}

			result.Removed = append(result.Removed, base)
		}

		delete(state.Files, base)
	}

	if err := s.saveAltState(state); err != nil {
		return result, err
	}

	return result, nil
}

type altOutcome int

const (
	altCreated altOutcome = iota
	altUnchanged
	altConflict
)

// generate makes base follow source.
func (s *Store) generate(
	base string,
	source string,
	state altState,
	facts Facts,
	force bool,
) (AltChange, altRecord, altOutcome, error) {
	change := AltChange{Name: base, Source: source}

	file, err := s.readWorktree(source, 0)
	if err != nil {
		return change, altRecord{}, altConflict,
			fmt.Errorf("read alternate %q: %w", source, err)
	}

	record := altRecord{Source: source}

	switch {
	case file.mode == filemode.Symlink:
		record.Target = string(file.data)
	case facts.UseCopies:
		record.Hash = file.hash().String()
		change.Copy = true
	default:
		record.Target = path.Base(source)
	}

	filename := s.absolutePath(base)

	if _, err := os.Lstat(filename); err == nil {
		if s.matchesGenerated(filename, record, file.data) {
			return change, record, altUnchanged, nil
		}

		previous, known := state.Files[base]
		if !(known && s.ownsGenerated(base, previous)) && !force {
			return change, record, altConflict, nil
		}

		if err := os.Remove(filename); err != nil {
			return change, record, altConflict,
				fmt.Errorf("replace %q: %w", base, err)
		}
	}

	if record.Target != "" {
		err = os.Symlink(record.Target, filename)

		if err != nil && file.mode != filemode.Symlink {
			// No symlink support: fall back to a copy.
			record.Target = ""
			record.Hash = file.hash().String()
			change.Copy = true
			err = writeCopy(filename, file)
		}
	} else {
		err = writeCopy(filename, file)
	}

	if err != nil {
		return change, record, altConflict,
			fmt.Errorf("create %q: %w", base, err)
	}

	return change, record, altCreated, nil
}

func writeCopy(filename string, file *worktreeFile) error {
	permissions := os.FileMode(0o644)
	if file.mode == filemode.Executable {
		permissions = 0o755
	}

	return atomicfile.WriteFile(filename, file.data, permissions)
}

// matchesGenerated reports whether filename already is what record asks for.
func (s *Store) matchesGenerated(
	filename string,
	record altRecord,
	content []byte,
) bool {
	info, err := os.Lstat(filename)
	if err != nil {
		return false
	}

	if record.Target != "" {
		if info.Mode()&os.ModeSymlink == 0 {
			return false
		}

		target, err := os.Readlink(filename)

		return err == nil && target == record.Target
	}

	if !info.Mode().IsRegular() {
		return false
	}

	data, err := os.ReadFile(filename)

	return err == nil && bytes.Equal(data, content)
}

// ownsGenerated reports whether base still is the file dotmgr created, as
// opposed to something the user replaced it with.
func (s *Store) ownsGenerated(base string, record altRecord) bool {
	filename := s.absolutePath(base)

	info, err := os.Lstat(filename)
	if err != nil {
		return false
	}

	if record.Target != "" {
		if info.Mode()&os.ModeSymlink == 0 {
			return false
		}

		target, err := os.Readlink(filename)

		return err == nil && target == record.Target
	}

	if !info.Mode().IsRegular() {
		return false
	}

	data, err := os.ReadFile(filename)

	return err == nil && (&worktreeFile{data: data}).hash().String() == record.Hash
}

// hasAlternates reports whether a sibling named <file>##... exists, which
// makes <file> a generated file that must not be tracked itself.
func hasAlternates(filename string) bool {
	entries, err := os.ReadDir(filepath.Dir(filename))
	if err != nil {
		return false
	}

	prefix := filepath.Base(filename) + alternateSeparator

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) &&
			len(entry.Name()) > len(prefix) {
			return true
		}
	}

	return false
}

// validateConditions checks the conditions of a new alternate.
func validateConditions(conditions []string) error {
	known := map[string]bool{
		"os": true, "arch": true, "class": true, "hostname": true, "user": true,
	}

	for _, condition := range conditions {
		if strings.EqualFold(condition, "default") {
			continue
		}

		key, value, found := strings.Cut(condition, ".")
		if !found || value == "" || !known[strings.ToLower(key)] {
			return fmt.Errorf(
				"invalid condition %q; use default, os.<os>, arch.<arch>, "+
					"class.<class>, hostname.<host> or user.<user>",
				condition,
			)
		}
	}

	return nil
}

// AddAlternateResult reports what AddAlternate did.
type AddAlternateResult struct {
	Added  []string // the new ## files, now tracked
	Moved  []string // originals that were renamed to their ## name
	Copied []string // originals that were kept, because the alternate does
	// not apply to this machine
}

// AddAlternate turns existing files into alternates named
// <file>##<conditions> and tracks them.
//
// When the alternate applies to this machine, the file is renamed and the
// caller is expected to run Alt, which links the original name back to it.
// Otherwise the file is copied, so that it keeps working here.
func (s *Store) AddAlternate(
	conditions string,
	arguments ...string,
) (AddAlternateResult, error) {
	var result AddAlternateResult

	conditions = strings.TrimPrefix(conditions, alternateSeparator)
	parts := strings.Split(conditions, ",")

	if conditions == "" {
		return result, errors.New("alternate conditions must not be empty")
	}

	if err := validateConditions(parts); err != nil {
		return result, err
	}

	if len(arguments) == 0 {
		return result, errors.New("no paths given")
	}

	facts, err := s.SystemFacts()
	if err != nil {
		return result, err
	}

	_, applies := facts.score(parts)

	type plan struct{ full, target, name string }

	var plans []plan

	for _, argument := range arguments {
		full, name, err := s.resolve(argument)
		if err != nil {
			return result, err
		}

		info, err := os.Lstat(full)
		if err != nil {
			return result, fmt.Errorf("inspect %q: %w", argument, err)
		}

		switch {
		case !info.Mode().IsRegular():
			return result, fmt.Errorf(
				"%q is not a regular file; only plain files can become alternates",
				argument,
			)
		case strings.Contains(filepath.Base(full), alternateSeparator):
			return result, fmt.Errorf("%q already is an alternate", argument)
		case hasAlternates(full):
			return result, fmt.Errorf(
				"%q is generated from its ## alternates; "+
					"create or edit those instead",
				argument,
			)
		}

		target := full + alternateSeparator + conditions

		if _, err := os.Lstat(target); err == nil {
			return result, fmt.Errorf("%q already exists", target)
		}

		plans = append(plans, plan{
			full:   full,
			target: target,
			name:   name + alternateSeparator + conditions,
		})
	}

	for _, p := range plans {
		var err error

		if applies {
			err = os.Rename(p.full, p.target)
		} else {
			err = copyFile(p.full, p.target)
		}
		if err != nil {
			return result, fmt.Errorf("create %q: %w", p.target, err)
		}

		if _, err := s.Add(p.target); err != nil {
			// Put things back as they were.
			if applies {
				os.Rename(p.target, p.full)
			} else {
				os.Remove(p.target)
			}

			return result, err
		}

		result.Added = append(result.Added, p.name)

		original := strings.TrimSuffix(p.name, alternateSeparator+conditions)
		if applies {
			result.Moved = append(result.Moved, original)
		} else {
			result.Copied = append(result.Copied, original)
		}
	}

	return result, nil
}

// copyFile copies a regular file, keeping its permissions.
func copyFile(source string, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}

	return atomicfile.WriteFile(destination, data, info.Mode().Perm())
}
