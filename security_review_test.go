package vlt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecurityReviewAncestorSymlink(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	outside := t.TempDir()
	old := []byte("outside original")
	target := filepath.Join(outside, "note.md")
	if err := os.WriteFile(target, old, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	v, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	read, readErr := v.DurableReadDocument("escape/note", 1024)
	result, writeErr := v.ReplaceDocument(
		"escape/note",
		[]byte("outside replaced"),
		documentHash(old),
		1024,
	)
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf(
		"read=%q read_error=%v save=%s write_error=%v outside=%q",
		read,
		readErr,
		result.Save,
		writeErr,
		got,
	)
	if readErr == nil || writeErr == nil || string(got) != string(old) {
		t.Fatal("ancestor symlink escaped vault boundary")
	}
}
func TestSecurityReviewReplacementBound(t *testing.T) {
	v, path, old := documentFixture(t)
	next := make([]byte, 2048)
	result, err := v.ReplaceDocument("note", next, documentHash(old), 1024)
	got, _ := os.ReadFile(path)
	t.Logf("save=%s error=%v saved_bytes=%d max=1024", result.Save, err, len(got))
	if err == nil || string(got) != string(old) {
		t.Fatal("replacement exceeds supplied byte bound")
	}
}
