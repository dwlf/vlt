package vlt

import "fmt"

// DocumentSizeError holds a capped prefix solely for classifying a refused source.
// Prefix is incomplete and must never be accepted as a complete document snapshot.
type DocumentSizeError struct {
	Limit  int64
	Size   int64
	Prefix []byte
}

func (e *DocumentSizeError) Error() string {
	return fmt.Sprintf("document exceeds %d byte bound", e.Limit)
}
