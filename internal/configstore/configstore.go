// Package configstore persists saved pkg/config.AttackConfig JSON
// documents on disk, keyed by a user-chosen name, so the web UI's
// builder.html can list, load, and re-run a previously saved attack
// config instead of only "Preview JSON" + "Load testserver example" — the
// saved-config gallery STATUS.md's "Next step" #1 asked for.
//
// A Store is just a directory of "<name>.json" files. Each Save is
// validated with pkg/config.Load (the same parser cmd/cli's config-file
// mode and /ws/config both use) before being written, so a malformed
// document fails loudly at save time — consistent with pkg/config.Load's
// own "a typo should fail loudly, not silently do nothing" convention —
// rather than silently corrupting the gallery or only failing later when
// someone tries to load and run it.
package configstore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"intruder/pkg/config"
)

// ErrNotFound is returned by Load and Delete when name has no saved
// config.
var ErrNotFound = errors.New("configstore: not found")

// ErrInvalidName is returned by ValidateName (and anything that calls it)
// when name fails validation.
var ErrInvalidName = errors.New("configstore: invalid name")

// ErrInvalidConfig is returned by Save when data doesn't parse as a
// pkg/config.AttackConfig document. Kept distinct from ErrInvalidName (a
// bad name) and from an unwrapped I/O failure, so a caller — notably
// internal/server's HTTP handlers — can tell "the client sent something
// wrong" (ErrInvalidName or ErrInvalidConfig, both a 400) apart from "the
// store itself failed" (anything else, a 500) without re-parsing data
// itself.
var ErrInvalidConfig = errors.New("configstore: invalid config document")

// validName matches the names Store accepts: 1-100 of letters, digits,
// spaces, dashes, underscores, or dots. Deliberately excludes every path
// separator (no "/", no "\"), which also rules out any ".."-style
// traversal — a name built entirely from this charset can never escape
// Store's directory, so Save/Load/Delete need no separate traversal check
// beyond calling ValidateName first.
var validName = regexp.MustCompile(`^[A-Za-z0-9 _.-]{1,100}$`)

// ValidateName reports whether name is a legal saved-config name.
func ValidateName(name string) error {
	if !validName.MatchString(name) || strings.TrimSpace(name) != name || strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: %q must be 1-100 characters, letters/digits/spaces/._- only, no leading/trailing space", ErrInvalidName, name)
	}
	return nil
}

// Store is a directory of saved AttackConfig JSON documents, one file per
// name.
type Store struct {
	dir string
}

// New returns a Store backed by dir, creating it (and any missing
// parents) if it doesn't already exist.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("configstore: creating %s: %w", dir, err)
	}
	return &Store{dir: dir}, nil
}

func (s *Store) path(name string) string {
	return filepath.Join(s.dir, name+".json")
}

// List returns the name of every saved config, sorted.
func (s *Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("configstore: listing %s: %w", s.dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".json"))
	}
	sort.Strings(names)
	return names, nil
}

// Load returns the raw JSON previously written by Save under name.
func (s *Store) Load(name string) (json.RawMessage, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(s.path(name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("configstore: %q: %w", name, ErrNotFound)
		}
		return nil, fmt.Errorf("configstore: reading %q: %w", name, err)
	}
	return json.RawMessage(data), nil
}

// Save validates data as a pkg/config.AttackConfig document (the same
// parse cmd/cli's config-file mode and /ws/config both run on load/run)
// and writes it under name, overwriting any existing config of that name.
// The write is atomic — a temp file in the same directory, then renamed
// into place — so a crash or a concurrent Load never observes a
// half-written file.
func (s *Store) Save(name string, data []byte) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if _, err := config.Load(bytes.NewReader(data)); err != nil {
		return fmt.Errorf("configstore: %q: %w: %w", name, ErrInvalidConfig, err)
	}

	tmp, err := os.CreateTemp(s.dir, "save-*.tmp")
	if err != nil {
		return fmt.Errorf("configstore: %q: %w", name, err)
	}
	tmpPath := tmp.Name()
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(tmpPath)
		if writeErr != nil {
			return fmt.Errorf("configstore: %q: %w", name, writeErr)
		}
		return fmt.Errorf("configstore: %q: %w", name, closeErr)
	}
	if err := os.Rename(tmpPath, s.path(name)); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("configstore: %q: %w", name, err)
	}
	return nil
}

// Delete removes the saved config under name.
func (s *Store) Delete(name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if err := os.Remove(s.path(name)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("configstore: %q: %w", name, ErrNotFound)
		}
		return fmt.Errorf("configstore: deleting %q: %w", name, err)
	}
	return nil
}
