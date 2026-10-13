package vlt

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func documentFixture(t *testing.T) (*Vault, string, []byte) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	old := []byte("---\ntitle: old\n---\nold body\n")
	path := filepath.Join(dir, "note.md")
	if err := os.WriteFile(path, old, 0644); err != nil {
		t.Fatal(err)
	}
	v, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return v, path, old
}

func TestDocumentReplacementFailureTruth(t *testing.T) {
	for _, stage := range []string{"write", "file-sync", "rename", "directory"} {
		t.Run(stage, func(t *testing.T) {
			v, path, old := documentFixture(t)
			v.documentFault = func(at string) error {
				if at == stage {
					return errors.New("fixture fault")
				}
				return nil
			}
			next := []byte("---\ntitle: new\n---\nnew body\n")
			result, err := v.ReplaceDocument("note", next, documentHash(old), 1024)
			if err == nil {
				t.Fatal("expected fault")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			expected := string(old)
			saved := "not-saved"
			if stage == "directory" {
				expected = string(next)
				saved = "uncertain"
			}
			if string(got) != expected || result.Save != saved {
				t.Fatalf("bytes=%q state=%+v", got, result)
			}
			if stage == "directory" {
				read, err := v.DurableReadDocument("note", 1024)
				if err != nil || string(read) != string(next) {
					t.Fatalf("independent barrier: %q %v", read, err)
				}
			}
		})
	}
}

func TestDocumentReplacementDurableAndConflict(t *testing.T) {
	v, path, old := documentFixture(t)
	next := []byte("new complete bytes\n")
	result, err := v.ReplaceDocument("note", next, documentHash(old), 1024)
	if err != nil || result.Save != "durable" || result.Integrity != "current" {
		t.Fatalf("%+v %v", result, err)
	}
	_, err = v.ReplaceDocument("note", []byte("stale rewrite"), documentHash(old), 1024)
	if err == nil {
		t.Fatal("expected comparison refusal")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(next) {
		t.Fatalf("conflict changed bytes: %q %v", got, err)
	}
}

func TestPostCommitIntegrityFailureStillSaved(t *testing.T) {
	v, path, old := documentFixture(t)
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	v.registry.dir = blocked
	result, err := v.ReplaceDocument("note", []byte("saved"), documentHash(old), 1024)
	if err == nil || result.Save != "durable" || result.Integrity != "stale" {
		t.Fatalf("%+v %v", result, err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "saved" {
		t.Fatalf("%q %v", got, err)
	}
}

func TestDurableDocumentReadRejectsOversizeAndSymlink(t *testing.T) {
	v, path, _ := documentFixture(t)
	if _, err := v.DurableReadDocument("note", 2); err == nil {
		t.Fatal("expected size bound")
	}
	link := filepath.Join(filepath.Dir(path), "link.md")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := v.DurableReadDocument("link", 1024); err == nil {
		t.Fatal("expected symlink refusal")
	}
}

func TestDocumentRootLeafAndHardlinkPolicy(t *testing.T) {
	for _, kind := range []string{"root-symlink", "leaf-symlink", "hardlink"} {
		t.Run(
			kind,
			func(t *testing.T) {
				v, path, old := documentFixture(t)
				switch kind {
				case "root-symlink":
					link := filepath.Join(t.TempDir(), "vault-link")
					if err := os.Symlink(v.dir, link); err != nil {
						t.Fatal(err)
					}
					v.dir = link
				case "leaf-symlink":
					outside := filepath.Join(t.TempDir(), "outside.md")
					if err := os.WriteFile(outside, old, 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(outside, path); err != nil {
						t.Fatal(err)
					}
				case "hardlink":
					if err := os.Link(path, filepath.Join(t.TempDir(), "outside.md")); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := v.DurableReadDocument("note", 1024); err == nil {
					t.Fatal("unsupported ownership allowed read")
				}
				if _, err := v.ReplaceDocument("note", []byte("changed"), documentHash(old), 1024); err == nil {
					t.Fatal("unsupported ownership allowed write")
				}
				got, err := os.ReadFile(path)
				if err != nil || string(got) != string(old) {
					t.Fatalf("old bytes changed: %q %v", got, err)
				}
			},
		)
	}
}

func TestDocumentReplacementExactByteBoundary(t *testing.T) {
	for _, cap := range []int64{-1, 0, 31, 32} {
		t.Run(fmt.Sprint(cap), func(t *testing.T) {
			v, path, old := documentFixture(t)
			next := bytes.Repeat([]byte("x"), 32)
			result, err := v.ReplaceDocument("note", next, documentHash(old), cap)
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if cap < 32 {
				if err == nil || result.Save != "not-saved" || !bytes.Equal(got, old) {
					t.Fatalf("cap refusal: %+v %v %q", result, err, got)
				}
			} else if err != nil || !bytes.Equal(got, next) {
				t.Fatalf("exact boundary: %+v %v", result, err)
			}
		})
	}
}

func TestDocumentBoundOverflowRefusesBeforeMutation(t *testing.T) {
	v, path, old := documentFixture(t)
	if _, err := v.ReplaceDocument(
		"note",
		[]byte("replacement"),
		documentHash(old),
		math.MaxInt64,
	); err == nil {
		t.Fatal("overflowing replacement bound accepted")
	}
	if _, err := v.DurableReadDocument("note", math.MaxInt64); err == nil {
		t.Fatal("overflowing read bound accepted")
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, old) {
		t.Fatalf("overflow changed bytes: %q %v", got, err)
	}
}
