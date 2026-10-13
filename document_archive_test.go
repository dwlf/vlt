package vlt

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDocumentArchiveRetainsBytesAndRegistryTruth(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	for _, path := range []string{"issues", ".trash"} {
		if err := os.Mkdir(filepath.Join(dir, path), 0700); err != nil {
			t.Fatal(err)
		}
	}
	v, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("retained source evidence")
	if _, err := v.CreateDocument("issues/one.md", data, 1024); err != nil {
		t.Fatal(err)
	}
	result, err := v.ArchiveDocument("issues/one.md", ".trash/one.md", 1024)
	if err != nil || result.Save != "durable" || result.Integrity != "current" {
		t.Fatalf("archive=%+v: %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "issues/one.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("source still exists")
	}
	read, err := os.ReadFile(filepath.Join(dir, ".trash/one.md"))
	if err != nil || string(read) != string(data) {
		t.Fatal("archive lost original bytes")
	}
	if _, exists := v.registry.entries["issues/one.md"]; exists {
		t.Fatal("removed path remains registered")
	}
	if _, exists := v.registry.entries[".trash/one.md"]; !exists {
		t.Fatal("archive lacks integrity record")
	}
}

func TestDocumentArchiveRefusesOccupiedDestination(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".trash"), 0700); err != nil {
		t.Fatal(err)
	}
	v, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.CreateDocument("one.md", []byte("original"), 1024); err != nil {
		t.Fatal(err)
	}
	if _, err := v.CreateDocument(".trash/one.md", []byte("previous"), 1024); err != nil {
		t.Fatal(err)
	}
	result, err := v.ArchiveDocument("one.md", ".trash/one.md", 1024)
	if err == nil || result.Save != "not-saved" {
		t.Fatal("occupied destination overwritten")
	}
	read, err := os.ReadFile(filepath.Join(dir, "one.md"))
	if err != nil || string(read) != "original" {
		t.Fatal("refusal changed original")
	}
}
