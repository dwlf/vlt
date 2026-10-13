package vlt

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ArchiveDocument moves one bounded document and durably updates derived registry paths.
// The caller holds its exclusive vault lock; destination parents must already exist.
func (v *Vault) ArchiveDocument(
	title, destination string, maxBytes int64,
) (result DocumentResult, err error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	result = DocumentResult{Save: "not-saved", Integrity: "stale"}
	root, rel, err := v.documentPath(title)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	if err := requireArchiveDestination(root, destination); err != nil {
		return result, err
	}
	data, err := readRootDocument(root, rel, maxBytes)
	if err != nil {
		return result, err
	}
	if err := root.Rename(rel, destination); err != nil {
		return result, err
	}
	result.Save = "uncertain"
	result.SHA256 = documentHash(data)
	v.invalidateIndex()
	if err := flushArchiveDirectories(root, rel, destination); err != nil {
		return result, err
	}
	result.Save = "durable"
	err = v.registry.archiveDurable(v.dir, rel, destination, data)
	if err != nil {
		return result, fmt.Errorf("archive saved; repair stale integrity registry: %w", err)
	}
	result.Integrity = "current"
	return result, nil
}

func requireArchiveDestination(root *os.Root, path string) error {
	if err := validateDocumentParent(root, path); err != nil {
		return err
	}
	if _, err := root.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("archive destination exists or is unavailable")
	}
	return nil
}

func flushArchiveDirectories(root *os.Root, source, destination string) error {
	if err := syncRootDirectory(root, filepath.Dir(source)); err != nil {
		return err
	}
	return syncRootDirectory(root, filepath.Dir(destination))
}
