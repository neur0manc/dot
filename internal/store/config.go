package store

import (
	"fmt"
	"os"
	"runtime"

	"github.com/go-git/go-git/v5/config"
)

// The repository configuration section holding the machine's class, and the
// one dotmgr, as the project was called until 0.4, used.
const (
	configSection       = "dot"
	legacyConfigSection = "dotmgr"
)

// SetConfig writes the repository-local commit identity or machine class.
func (s *Store) SetConfig(key string, value string) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", key)
	}

	configuration, err := s.repo.Config()
	if err != nil {
		return fmt.Errorf("read repository configuration: %w", err)
	}

	switch key {
	case "user.name":
		configuration.User.Name = value
	case "user.email":
		configuration.User.Email = value
	case ClassConfigKey:
		configuration.Raw.Section(configSection).SetOption("class", value)
		configuration.Raw.RemoveSection(legacyConfigSection)
		s.facts.Class = value
	default:
		return fmt.Errorf("unsupported configuration key %q", key)
	}

	if err := s.repo.SetConfig(configuration); err != nil {
		return fmt.Errorf("write repository configuration: %w", err)
	}

	return nil
}

// Config reads the commit identity as Git would resolve it, or the machine
// class.
func (s *Store) Config(key string) (string, error) {
	configuration, err := s.repo.ConfigScoped(config.GlobalScope)
	if err != nil {
		return "", fmt.Errorf("read repository configuration: %w", err)
	}

	switch key {
	case "user.name":
		return configuration.User.Name, nil
	case "user.email":
		return configuration.User.Email, nil
	case ClassConfigKey:
		return s.class()
	default:
		return "", fmt.Errorf("unsupported configuration key %q", key)
	}
}

// class reads the machine's class from the repository configuration.
func (s *Store) class() (string, error) {
	configuration, err := s.repo.Config()
	if err != nil {
		return "", fmt.Errorf("read repository configuration: %w", err)
	}

	if class := configuration.Raw.Section(configSection).Option("class"); class != "" {
		return class, nil
	}

	return configuration.Raw.Section(legacyConfigSection).Option("class"), nil
}

// Editor returns the command for editing commit messages, chosen as Git
// chooses it: $GIT_EDITOR, core.editor from the repository's and then the
// user's Git configuration, $VISUAL, $EDITOR, and finally vi (Notepad on
// Windows).
func (s *Store) Editor() string {
	if editor := os.Getenv("GIT_EDITOR"); editor != "" {
		return editor
	}

	if local, err := s.repo.Config(); err == nil {
		if editor := local.Raw.Section("core").Option("editor"); editor != "" {
			return editor
		}
	}

	if global, err := config.LoadConfig(config.GlobalScope); err == nil {
		if editor := global.Raw.Section("core").Option("editor"); editor != "" {
			return editor
		}
	}

	for _, variable := range []string{"VISUAL", "EDITOR"} {
		if editor := os.Getenv(variable); editor != "" {
			return editor
		}
	}

	if runtime.GOOS == "windows" {
		return "notepad"
	}

	return "vi"
}
