package vlt

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// openDocumentRoot rejects a symlink as the configured root itself. Ancestors of
// that configured root are canonicalized (including macOS's /var alias).
func openDocumentRoot(dir string) (*os.Root, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("configured document root is not a real directory")
	}
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	return os.OpenRoot(canonical)
}

func (v *Vault) documentPath(title string) (*os.Root, string, error) {
	path, err := v.resolve(title)
	if err != nil {
		return nil, "", err
	}
	rel, err := filepath.Rel(v.dir, path)
	if err != nil {
		return nil, "", err
	}
	root, err := openDocumentRoot(v.dir)
	if err != nil {
		return nil, "", err
	}
	if err := validateDocumentComponents(root, rel); err != nil {
		root.Close()
		return nil, "", err
	}
	return root, rel, nil
}

func validateDocumentComponents(root *os.Root, rel string) error {
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("document escapes vault")
	}
	parts := strings.Split(filepath.Clean(rel), string(os.PathSeparator))
	at := ""
	for i, part := range parts {
		at = filepath.Join(at, part)
		info, err := root.Lstat(at)
		if err != nil {
			return err
		}
		if err := validateDocumentComponent(info, at, i == len(parts)-1); err != nil {
			return err
		}
	}
	return nil
}

func cappedDescriptorRead(f *os.File, maxBytes int64) ([]byte, os.FileInfo, error) {
	if maxBytes <= 0 || maxBytes == math.MaxInt64 {
		return nil, nil, fmt.Errorf("document byte bound must be positive")
	}
	info, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if err := validateDocumentLeaf(info); err != nil {
		return nil, nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, nil, &DocumentSizeError{Limit: maxBytes, Size: info.Size(), Prefix: data[:maxBytes]}
	}
	return data, info, nil
}

func validateDocumentParent(root *os.Root, rel string) error {
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("new document escapes vault")
	}
	parts := strings.Split(filepath.Clean(filepath.Dir(rel)), string(os.PathSeparator))
	at := ""
	for _, part := range parts {
		at = filepath.Join(at, part)
		info, err := root.Lstat(at)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("new document ancestor is not a real directory")
		}
	}
	return nil
}

func validateDocumentComponent(info os.FileInfo, path string, leaf bool) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("document path contains unsupported symlink: %s", path)
	}
	if leaf {
		return validateDocumentLeaf(info)
	}
	if !info.IsDir() {
		return fmt.Errorf("document ancestor is not a directory: %s", path)
	}
	return nil
}
