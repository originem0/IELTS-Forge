package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type boundedZeroReader struct{ largest int }

func (reader *boundedZeroReader) Read(buffer []byte) (int, error) {
	if len(buffer) > reader.largest {
		reader.largest = len(buffer)
	}
	clear(buffer)
	return len(buffer), nil
}

func TestImportMediaStreamsWithBoundedReads(t *testing.T) {
	stage := t.TempDir()
	preview := &bankImportPreview{Stage: stage, Media: map[string]bool{}}
	zeros := &boundedZeroReader{}
	const size = 8 << 20
	prefix := []byte{137, 80, 78, 71, 13, 10, 26, 10}
	input := io.MultiReader(bytes.NewReader(prefix), io.LimitReader(zeros, size-int64(len(prefix))))
	if err := preview.readMedia(input, size); err != nil {
		t.Fatal(err)
	}
	if zeros.largest > 64<<10 {
		t.Fatalf("large media read allocated %d bytes", zeros.largest)
	}
	if len(preview.Media) != 1 {
		t.Fatal("streamed media was not indexed")
	}
}

func TestImportCleanupSkipsActiveAndUnrelatedDirectories(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	stale := filepath.Join(root, "elp-bank-import-stale")
	active := filepath.Join(root, "elp-bank-import-active")
	unrelated := filepath.Join(root, "user-data")
	for _, dir := range []string{stale, active, unrelated} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	handle, err := lockDataDirectory(active)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	for _, dir := range []string{stale, active, unrelated} {
		if err := os.Chtimes(dir, now.Add(-48*time.Hour), now.Add(-48*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	cleanupImportStagesAt(root, now)
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("abandoned stage not removed")
	}
	for _, dir := range []string{active, unrelated} {
		if _, err := os.Stat(dir); err != nil {
			t.Fatal("cleanup removed protected directory", err)
		}
	}
}
