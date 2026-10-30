package store

import (
	"io"
	"strings"
	"testing"
)

const sublimePreferences = "Packages/User/Preferences.sublime-settings"

func TestAliasesFollowTheSystem(t *testing.T) {
	for system, directory := range map[string]string{
		"darwin":  "Library/Application Support/Sublime Text",
		"linux":   ".config/sublime-text",
		"freebsd": ".config/sublime-text",
		"windows": "AppData/Roaming/Sublime Text",
	} {
		facts := Facts{OS: system}
		name := "@sublime/" + sublimePreferences
		onDisk := directory + "/" + sublimePreferences

		for _, test := range []struct{ name, filePath, conditions string }{
			{name, onDisk, ""},
			{name + "##hostname.work", onDisk, "hostname.work"},
		} {
			filePath, conditions := facts.SplitName(test.name)
			if filePath != test.filePath || strings.Join(conditions, ",") != test.conditions {
				t.Errorf("%s: SplitName(%q) = %q %v", system, test.name, filePath, conditions)
			}

			var variant []string
			if test.conditions != "" {
				variant = strings.Split(test.conditions, ",")
			}

			if got := facts.VariantName(test.filePath, variant); got != test.name {
				t.Errorf("%s: VariantName(%q) = %q, want %q", system, test.filePath, got, test.name)
			}
		}
	}
}

func TestOtherPathsAreNotAliased(t *testing.T) {
	facts := Facts{OS: "linux"}

	for _, name := range []string{
		"@sublime", "@sublime/", "@unknown/x", "sublime/x",
		"Library/Application Support/Sublime Text/x", // another system's directory
	} {
		if got, _ := facts.SplitName(name); got != name {
			t.Errorf("SplitName(%q) = %q", name, got)
		}
	}
}

func TestAliasedFilesFollowTheMachine(t *testing.T) {
	a := newMachine(t)
	source := a.init(t)

	directory := aliases["@sublime"].other
	if source.facts.OS == "darwin" {
		directory = aliases["@sublime"].darwin
	}

	if source.facts.OS == "windows" {
		directory = aliases["@sublime"].windows
	}

	onDisk := directory + "/" + sublimePreferences
	a.write(t, onDisk, "{}\n")

	added, err := source.Add("~/" + directory + "/Packages")
	if err != nil || len(added) != 1 || added[0].Name != "@sublime/"+sublimePreferences {
		t.Fatalf("add: %+v %v", added, err)
	}

	expectStatus(t, source, "A  @sublime/"+sublimePreferences)
	mustCommit(t, source, "sublime")

	b := newMachine(t)
	clone, result, err := Clone(b.gitDir, b.home, createRemote(t, source), io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Join(result.Written, ",") != onDisk || b.exists("@sublime") {
		t.Fatalf("clone wrote %v", result.Written)
	}

	expectStatus(t, clone)

	// On another system the file lives elsewhere, and nothing of this
	// system's directory is needed.
	clone.facts.OS = otherOS()

	tracked, err := clone.Tracked()
	if err != nil || len(tracked) != 1 || tracked[0].Path == onDisk ||
		!strings.HasSuffix(tracked[0].Path, sublimePreferences) {
		t.Fatalf("tracked: %+v %v", tracked, err)
	}
}
