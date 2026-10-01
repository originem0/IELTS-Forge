//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestPrimaryReplacementFailureDoesNotTruncateOriginal(t *testing.T) {
	s := libraryTestStore(t)
	if err := s.save(json.RawMessage(`{"writings":[{"id":"safe","essay":"original"}]}`)); err != nil {
		t.Fatal(err)
	}
	path := s.dataPathLocked()
	before, _ := os.ReadFile(path)
	utf16, _ := syscall.UTF16PtrFromString(path)
	// Permit reads and writes but deny delete/replace, as a scanner holding a
	// Windows file handle can do. Backup succeeds; primary rename must fail.
	handle, err := syscall.CreateFile(utf16, syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(handle)
	if err := s.save(json.RawMessage(`{"writings":[]}`)); err == nil {
		t.Fatal("expected replacement failure")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("failed replacement changed original bytes")
	}
	leftovers, _ := filepath.Glob(filepath.Join(s.directory, ".library-*"))
	if len(leftovers) != 0 {
		t.Fatal("failed write leaked temporary files")
	}
}
