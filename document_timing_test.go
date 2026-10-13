package vlt

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestDocumentFlushTimings(t *testing.T) {
	dir := t.TempDir()
	for _, size := range []int{32768, 1048576} {
		files, directories := []float64{}, []float64{}
		for index := 0; index < 12; index++ {
			path := filepath.Join(dir, fmt.Sprintf("size-%d-%d", size, index))
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := file.Write(make([]byte, size)); err != nil {
				file.Close()
				t.Fatal(err)
			}
			start := time.Now()
			syncErr := file.Sync()
			files = append(files, float64(time.Since(start))/float64(time.Millisecond))
			closeErr := file.Close()
			if syncErr != nil || closeErr != nil {
				t.Fatalf("file flush=%v close=%v", syncErr, closeErr)
			}
			directory, err := os.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			start = time.Now()
			syncErr = directory.Sync()
			directories = append(directories, float64(time.Since(start))/float64(time.Millisecond))
			closeErr = directory.Close()
			if syncErr != nil || closeErr != nil {
				t.Fatalf("directory flush=%v close=%v", syncErr, closeErr)
			}
		}
		sort.Float64s(files)
		sort.Float64s(directories)
		t.Logf("synthetic_new_file_bytes=%d samples=12 file_sync_median_ms=%.6f file_sync_max_ms=%.6f directory_sync_median_ms=%.6f directory_sync_max_ms=%.6f", size, (files[5]+files[6])/2, files[11], (directories[5]+directories[6])/2, directories[11])
	}
}
