package store

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"slices"
	"strings"
)

// A tracked file can have variants for particular machines. In the
// repository a variant is stored under the name yadm uses,
//
//	<path>##<condition>[,<condition>...]
//
// but on disk only <path> exists, holding the variant that fits the machine
// best. A plain <path> without conditions is the fallback for machines that
// no variant fits. The conditions are
//
//	os.<os>          linux, darwin (or macos), windows, freebsd, ...
//	                 and unix for every system except Windows
//	arch.<arch>      amd64, arm64, ...
//	class.<class>    the value of `dotmgr config local.class`
//	hostname.<host>  the host name without domain
//
// Matching ignores case. All conditions of a variant must match; of the
// matching variants the one with the highest total weight wins.
const variantSeparator = "##"

// Condition keys, as used in variant names.
const (
	ConditionOS       = "os"
	ConditionArch     = "arch"
	ConditionClass    = "class"
	ConditionHostname = "hostname"
)

// Weights of the conditions: the more specific, the heavier. The weights are
// powers of two, so a single specific condition outweighs any combination of
// less specific ones.
var conditionWeights = map[string]int{
	ConditionArch:     1,
	ConditionClass:    2,
	"os-family":       4,
	ConditionOS:       8,
	ConditionHostname: 16,
}

// ClassConfigKey is the configuration key holding the machine's class.
const ClassConfigKey = "local.class"

// Facts describe the machine that variants are matched against.
type Facts struct {
	OS       string
	Arch     string
	Hostname string
	Class    string
}

// MachineFacts describes this machine, apart from its class, which is kept
// in the repository.
func MachineFacts() (Facts, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return Facts{}, fmt.Errorf("determine host name: %w", err)
	}

	hostname, _, _ = strings.Cut(hostname, ".")

	return Facts{OS: runtime.GOOS, Arch: runtime.GOARCH, Hostname: hostname}, nil
}

func (s *Store) loadFacts() error {
	facts, err := MachineFacts()
	if err != nil {
		return err
	}

	facts.Class, err = s.class()
	s.facts = facts

	return err
}

// Facts returns the description of this machine that variants are matched
// against.
func (s *Store) Facts() Facts { return s.facts }

// Condition builds a condition such as "os.darwin" from a key and a value.
// An empty value stands for this machine's own value.
func (f Facts) Condition(key string, value string) (string, error) {
	if value == "" {
		switch key {
		case ConditionOS:
			value = f.OS
		case ConditionArch:
			value = f.Arch
		case ConditionHostname:
			value = f.Hostname
		case ConditionClass:
			value = f.Class
			if value == "" {
				return "", fmt.Errorf(
					"this machine has no class; set one with `dotmgr config %s <class>`",
					ClassConfigKey,
				)
			}
		}
	}

	condition := key + "." + strings.ToLower(value)

	if _, ok := conditionWeight(condition); !ok ||
		strings.ContainsAny(value, ",/\\#") {
		return "", fmt.Errorf("invalid condition %q", condition)
	}

	return condition, nil
}

// SplitName splits the name of a tracked file into the path it has on disk
// and its conditions, which are nil for a plain file.
func SplitName(name string) (string, []string) {
	directory, file := path.Split(name)

	base, conditions, found := strings.Cut(file, variantSeparator)
	if !found || base == "" || conditions == "" {
		return name, nil
	}

	return directory + base, strings.Split(conditions, ",")
}

// VariantName returns the name under which the variant of filePath for the
// given conditions is tracked.
func VariantName(filePath string, conditions []string) string {
	if len(conditions) == 0 {
		return filePath
	}

	return filePath + variantSeparator + strings.Join(conditions, ",")
}

// Describe formats a tracked name for people: ".zshrc (os.darwin)".
func Describe(name string) string {
	filePath, conditions := SplitName(name)
	if conditions == nil {
		return filePath
	}

	return fmt.Sprintf("%s (%s)", filePath, strings.Join(conditions, ", "))
}

func conditionWeight(condition string) (int, bool) {
	key, value, found := strings.Cut(strings.ToLower(condition), ".")
	if !found || value == "" {
		return 0, false
	}

	if key == ConditionOS && value == "unix" {
		return conditionWeights["os-family"], true
	}

	weight, ok := conditionWeights[key]

	return weight, ok && key != "os-family"
}

func normalizeOS(value string) string {
	switch value = strings.ToLower(value); value {
	case "macos", "osx", "mac":
		return "darwin"
	default:
		return value
	}
}

func normalizeArch(value string) string {
	switch value = strings.ToLower(value); value {
	case "x86_64", "x64":
		return "amd64"
	case "aarch64":
		return "arm64"
	default:
		return value
	}
}

// score returns the weight of a variant on this machine, or false if one of
// its conditions does not match. A plain file scores zero.
func (f Facts) score(conditions []string) (int, bool) {
	total := 0

	for _, condition := range conditions {
		weight, known := conditionWeight(condition)
		if !known {
			return 0, false
		}

		key, value, _ := strings.Cut(strings.ToLower(condition), ".")

		var matches bool

		switch key {
		case ConditionOS:
			if value == "unix" {
				matches = f.OS != "windows"
			} else {
				matches = normalizeOS(value) == normalizeOS(f.OS)
			}
		case ConditionArch:
			matches = normalizeArch(value) == normalizeArch(f.Arch)
		case ConditionClass:
			matches = f.Class != "" && strings.EqualFold(value, f.Class)
		case ConditionHostname:
			matches = strings.EqualFold(value, f.Hostname)
		}

		if !matches {
			return 0, false
		}

		total += weight
	}

	return total, true
}

// shownEntry is the tracked entry that a path on disk holds on this machine.
type shownEntry struct {
	name string // the tracked name, e.g. ".zshrc##os.darwin"
	treeEntry
}

// show maps each path on disk to the entry it holds on this machine: a plain
// file to itself and, of a file's variants, the one that fits best. Paths of
// which no variant fits are left out.
func (f Facts) show(files map[string]treeEntry) map[string]shownEntry {
	type best struct {
		shownEntry
		weight int
	}

	winners := map[string]best{}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names) // ties go to the first name

	for _, name := range names {
		filePath, conditions := SplitName(name)

		weight, matches := f.score(conditions)
		if !matches {
			continue
		}

		if current, found := winners[filePath]; !found || weight > current.weight {
			winners[filePath] = best{shownEntry{name, files[name]}, weight}
		}
	}

	shown := make(map[string]shownEntry, len(winners))
	for filePath, winner := range winners {
		shown[filePath] = winner.shownEntry
	}

	return shown
}
