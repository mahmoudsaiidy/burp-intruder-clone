package configstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validConfigJSON = `{"template":"x","mode":"sniper","generators":[]}`

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestValidateName(t *testing.T) {
	valid := []string{"a", "my config", "login-bruteforce_v2", "a.b.c", "A1 _-."}
	for _, name := range valid {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q): unexpected error: %v", name, err)
		}
	}

	invalid := []string{"", "../escape", "a/b", "a\\b", strings.Repeat("x", 101), "  "}
	for _, name := range invalid {
		if err := ValidateName(name); !errors.Is(err, ErrInvalidName) {
			t.Errorf("ValidateName(%q): got %v, want ErrInvalidName", name, err)
		}
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	s := newTestStore(t)

	if err := s.Save("sniper-login", []byte(validConfigJSON)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Load("sniper-login")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(got) != validConfigJSON {
		t.Errorf("Load = %s, want %s", got, validConfigJSON)
	}
}

func TestSave_OverwritesExisting(t *testing.T) {
	s := newTestStore(t)
	const updated = `{"template":"y","mode":"sniper","generators":[]}`

	if err := s.Save("cfg", []byte(validConfigJSON)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Save("cfg", []byte(updated)); err != nil {
		t.Fatalf("Save (overwrite): %v", err)
	}

	got, err := s.Load("cfg")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(got) != updated {
		t.Errorf("Load after overwrite = %s, want %s", got, updated)
	}
}

func TestSave_RejectsInvalidJSON(t *testing.T) {
	s := newTestStore(t)

	cases := []string{
		`not json`,
		`{"template":"x","mode":"sniper","generators":[],"bogus":true}`, // unknown field
	}
	for _, c := range cases {
		if err := s.Save("bad", []byte(c)); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("Save(%q): got %v, want ErrInvalidConfig", c, err)
		}
	}

	if _, err := s.Load("bad"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Load after failed Save: got %v, want ErrNotFound (nothing should have been written)", err)
	}
}

func TestSave_RejectsInvalidName(t *testing.T) {
	s := newTestStore(t)
	if err := s.Save("../escape", []byte(validConfigJSON)); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("Save: got %v, want ErrInvalidName", err)
	}
}

func TestLoad_NotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Load("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Load: got %v, want ErrNotFound", err)
	}
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)
	if err := s.Save("cfg", []byte(validConfigJSON)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Delete("cfg"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Load("cfg"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Load after Delete: got %v, want ErrNotFound", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	s := newTestStore(t)
	if err := s.Delete("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete: got %v, want ErrNotFound", err)
	}
}

func TestList_SortedAndFiltersNonJSON(t *testing.T) {
	s := newTestStore(t)
	for _, name := range []string{"zeta", "alpha", "mid"} {
		if err := s.Save(name, []byte(validConfigJSON)); err != nil {
			t.Fatalf("Save(%q): %v", name, err)
		}
	}
	// A stray non-JSON file and a subdirectory in the store's directory
	// shouldn't show up in List.
	if err := os.WriteFile(filepath.Join(s.dir, "notes.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("writing stray file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(s.dir, "subdir"), 0o755); err != nil {
		t.Fatalf("making subdir: %v", err)
	}

	got, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"alpha", "mid", "zeta"}
	if len(got) != len(want) {
		t.Fatalf("List = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List = %v, want %v", got, want)
		}
	}
}

func TestNew_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "configs")
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("New did not create %s", dir)
	}
	if list, err := s.List(); err != nil || len(list) != 0 {
		t.Fatalf("List on fresh store = %v, %v, want empty, nil", list, err)
	}
}
