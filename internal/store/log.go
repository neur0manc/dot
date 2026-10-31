package store

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// LogEntry is one commit of the history.
type LogEntry struct {
	Hash    plumbing.Hash
	Parents []plumbing.Hash
	Author  string
	Email   string
	When    time.Time // in the time zone the author committed in
	Message string
}

// Log returns the commits reachable from HEAD, newest first. A limit above
// zero caps their number. A repository without commits has an empty history.
func (s *Store) Log(limit int) ([]LogEntry, error) {
	iterator, err := s.repo.Log(&git.LogOptions{Order: git.LogOrderCommitterTime})
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read history: %w", err)
	}
	defer iterator.Close()

	var entries []LogEntry

	err = iterator.ForEach(func(commit *object.Commit) error {
		if limit > 0 && len(entries) >= limit {
			return storer.ErrStop
		}

		entries = append(entries, LogEntry{
			Hash:    commit.Hash,
			Parents: commit.ParentHashes,
			Author:  commit.Author.Name,
			Email:   commit.Author.Email,
			When:    commit.Author.When,
			Message: commit.Message,
		})

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read history: %w", err)
	}

	return entries, nil
}
