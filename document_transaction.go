package vlt

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// DocumentResult separates authoritative publication from derived integrity tracking.
// Save is not-saved, uncertain, or durable; Integrity is current or stale.
type DocumentResult struct {
	Save      string `json:"save"`
	Integrity string `json:"integrity"`
	SHA256    string `json:"sha256"`
}

// ReplaceDocument replaces complete note bytes after comparing the existing hash.
// The caller must hold its exclusive vault lock across read, transform and replacement.
// An error after rename never means that the document was not saved.
func (v *Vault) ReplaceDocument(
	title string,
	data []byte,
	expected string,
	maxBytes int64,
) (
	result DocumentResult,
	err error,
) {
	v.mu.Lock()
	defer v.mu.Unlock()
	result = DocumentResult{Save: "not-saved", Integrity: "stale"}
	if !validDocumentBound(data, maxBytes) {
		return result, fmt.Errorf("replacement exceeds positive %d byte bound", maxBytes)
	}
	root, rel, err := v.documentPath(title)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	old, err := readRootDocument(root, rel, maxBytes)
	if err != nil {
		return result, err
	}
	if documentHash(old) != expected {
		return result, fmt.Errorf("replace %s: document changed", title)
	}
	return v.publishDocument(root, rel, data)

}

// DurableReadDocument establishes a publication barrier for the visible complete file.
// The caller must hold its exclusive vault lock; this method does not change note bytes.
func (v *Vault) DurableReadDocument(title string, maxBytes int64) (data []byte, err error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	root, rel, err := v.documentPath(title)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	f, err := root.Open(rel)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	data, info, err := cappedDescriptorRead(f, maxBytes)
	if err != nil {
		return nil, err
	}
	if err = f.Sync(); err != nil {
		return nil, fmt.Errorf("flush visible document: %w", err)
	}
	if err = syncRootDirectory(root, filepath.Dir(rel)); err != nil {
		return nil, err
	}
	if err := validateBarrierIdentity(root, rel, info); err != nil {
		return nil, err
	}
	return data, nil
}

func readRootDocument(root *os.Root, rel string, maxBytes int64) (data []byte, err error) {
	f, err := root.Open(rel)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	data, _, err = cappedDescriptorRead(f, maxBytes)
	return data, err
}

func documentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// durableReplace is used for derived registry data, anchored in its own directory.
func durableReplace(
	path string,
	data []byte,
	perm os.FileMode,
	fault func(string) error,
) (
	renamed bool,
	err error,
) {
	root, err := openDocumentRoot(filepath.Dir(path))
	if err != nil {
		return false, err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	return replaceRootDocument(root, filepath.Base(path), data, perm, fault)
}

func replaceRootDocument(
	root *os.Root,
	rel string,
	data []byte,
	perm os.FileMode,
	fault func(string) error,
) (
	renamed bool,
	err error,
) {
	name := filepath.Join(filepath.Dir(rel), "."+filepath.Base(rel)+".tmp-"+rand.Text())
	tmp, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return false, err
	}
	defer func() {
		if !renamed {
			err = errors.Join(err, removeDocumentTemp(root, name))
		}
	}()
	if err = writeDocumentTemp(tmp, data, perm, fault); err != nil {
		return false, err
	}
	if err = documentStage(fault, "rename"); err != nil {
		return false, err
	}
	if err = root.Rename(name, rel); err != nil {
		return false, err
	}
	renamed = true
	if err = documentStage(fault, "directory"); err != nil {
		return true, err
	}
	return true, syncRootDirectory(root, filepath.Dir(rel))
}

func writeDocumentTemp(
	f *os.File,
	data []byte,
	perm os.FileMode,
	fault func(string) error,
) (err error) {
	defer func() { err = errors.Join(err, f.Close()) }()
	if err = documentStage(fault, "write"); err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Chmod(perm); err != nil {
		return err
	}
	if err = documentStage(fault, "file-sync"); err != nil {
		return err
	}
	return f.Sync()
}

func documentStage(fault func(string) error, stage string) error {
	if fault != nil {
		return fault(stage)
	}
	return nil
}

func syncRootDirectory(root *os.Root, dir string) (err error) {
	f, err := root.Open(dir)
	if err != nil {
		return fmt.Errorf("open publication directory: %w", err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	if err = f.Sync(); err != nil {
		return fmt.Errorf("flush publication directory: %w", err)
	}
	return nil
}

// CreateDocument publishes complete bytes at a new path without replacing an existing note.
// Parent directories must already exist and contain no symlink components.
func (v *Vault) CreateDocument(
	rel string,
	data []byte,
	maxBytes int64,
) (
	result DocumentResult,
	err error,
) {
	v.mu.Lock()
	defer v.mu.Unlock()
	result = DocumentResult{Save: "not-saved", Integrity: "stale"}
	if !validDocumentBound(data, maxBytes) {
		return result, fmt.Errorf("new document exceeds positive byte bound")
	}
	root, err := openDocumentRoot(v.dir)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	if err := validateDocumentParent(root, rel); err != nil {
		return result, err
	}
	if _, err := root.Lstat(rel); !errors.Is(err, os.ErrNotExist) {
		return result, fmt.Errorf("new document path already exists or unavailable")
	}
	return v.publishDocument(root, rel, data)

}

// ReadDocumentSnapshot returns capped visible bytes without establishing durability.
// A cooperating caller holds its shared or exclusive vault lock; delivery uses DurableReadDocument.
func (v *Vault) ReadDocumentSnapshot(title string, maxBytes int64) (data []byte, err error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	root, rel, err := v.documentPath(title)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	return readRootDocument(root, rel, maxBytes)
}

// RefreshDocumentIntegrity repairs derived bookkeeping for exact already-durable bytes.
// The caller retains its exclusive native lock and has established a read barrier.
func (v *Vault) RefreshDocumentIntegrity(title string, data []byte) (err error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	root, rel, err := v.documentPath(title)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	return v.registry.registerDurable(v.dir, filepath.Join(v.dir, rel), data)
}

func validateBarrierIdentity(root *os.Root, rel string, info os.FileInfo) error {
	if err := validateDocumentComponents(root, rel); err != nil {
		return err
	}
	now, err := root.Lstat(rel)
	if err != nil {
		return err
	}
	if !os.SameFile(info, now) {
		return fmt.Errorf("document replaced during durability barrier")
	}
	return nil
}

func validDocumentBound(data []byte, maxBytes int64) bool {
	return maxBytes > 0 && maxBytes < math.MaxInt64 && int64(len(data)) <= maxBytes
}

func (v *Vault) publishDocument(root *os.Root, rel string, data []byte) (DocumentResult, error) {
	result := DocumentResult{Save: "not-saved", Integrity: "stale"}
	renamed, err := replaceRootDocument(root, rel, data, 0644, v.documentFault)
	if renamed {
		result.Save = "uncertain"
		result.SHA256 = documentHash(data)
		v.invalidateIndex()
	}
	if err != nil {
		return result, err
	}
	result.Save = "durable"
	if err := v.registry.registerDurable(v.dir, filepath.Join(v.dir, rel), data); err != nil {
		return result, fmt.Errorf("document saved with stale integrity: %w", err)
	}
	result.Integrity = "current"
	return result, nil
}

func removeDocumentTemp(root *os.Root, name string) error {
	err := root.Remove(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
