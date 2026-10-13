//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

package vlt

import (
	"fmt"
	"os"
	"syscall"
)

func validateDocumentLeaf(info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("document is not a regular file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 {
		return fmt.Errorf("document hardlinks are unsupported")
	}
	return nil
}
