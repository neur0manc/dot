package store

import (
	"maps"
	"slices"
	"strings"
)

// Some applications keep their configuration in different places on
// different systems. A tracked name that starts with an alias such as
// "@sublime" stands for that application's directory on whatever machine the
// repository is checked out on, so one copy of the files serves all of them:
//
//	@sublime/Packages/User/Preferences.sublime-settings
//
// is ".config/sublime-text/Packages/User/Preferences.sublime-settings" on
// Linux and "Library/Application Support/Sublime Text/Packages/User/..." on
// macOS. Only the directory of the machine's own system is ever created.
// Variants work on top of aliases, so "@sublime/…##hostname.work" is fine.
// The directories are relative to the home directory.
type aliasDirectories struct {
	darwin  string
	windows string
	other   string // Linux and the rest
}

var aliases = map[string]aliasDirectories{
	"@sublime": {
		darwin:  "Library/Application Support/Sublime Text",
		windows: "AppData/Roaming/Sublime Text",
		other:   ".config/sublime-text",
	},
}

func (f Facts) aliasDirectory(directories aliasDirectories) string {
	switch normalizeOS(f.OS) {
	case "darwin":
		return directories.darwin
	case "windows":
		return directories.windows
	default:
		return directories.other
	}
}

// expandAlias replaces the alias that name starts with by the directory it
// stands for on this machine. Other names are returned as they are.
func (f Facts) expandAlias(name string) string {
	alias, rest, found := strings.Cut(name, "/")
	if !found || rest == "" {
		return name
	}

	directories, known := aliases[alias]
	if !known {
		return name
	}

	return f.aliasDirectory(directories) + "/" + rest
}

// collapseAlias is the reverse of expandAlias: a path inside the directory
// of an alias on this machine gets the alias as its start.
func (f Facts) collapseAlias(filePath string) string {
	for _, alias := range slices.Sorted(maps.Keys(aliases)) {
		directory := f.aliasDirectory(aliases[alias])

		if rest, inside := strings.CutPrefix(filePath, directory+"/"); inside {
			return alias + "/" + rest
		}
	}

	return filePath
}
