//go:build !darwin && !linux && !freebsd && !netbsd && !openbsd && !dragonfly

package vlt

import (
	"fmt"
	"os"
)

func validateDocumentLeaf(info os.FileInfo) error {
	return fmt.Errorf("durable document ownership is not verified on this platform")
}
