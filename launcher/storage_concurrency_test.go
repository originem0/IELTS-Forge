package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestDataCASRejectsStaleAndMissingRevisions(t *testing.T) {
	s := libraryTestStore(t)
	snapshot, err := s.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	revision := snapshot["revision"].(string)
	var wg sync.WaitGroup
	errorsOut := make(chan error, 2)
	for _, value := range []string{`{"writings":[{"id":"a"}]}`, `{"writings":[{"id":"b"}]}`} {
		wg.Add(1)
		go func(value string) {
			defer wg.Done()
			_, err := s.saveConditional(json.RawMessage(value), s.directoryIDLocked(), revision)
			errorsOut <- err
		}(value)
	}
	wg.Wait()
	close(errorsOut)
	successful, conflicts := 0, 0
	for err := range errorsOut {
		if err == nil {
			successful++
		} else if errors.Is(err, errDataConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successful != 1 || conflicts != 1 {
		t.Fatalf("success=%d conflicts=%d", successful, conflicts)
	}
	before, _ := s.load()
	if _, err := s.saveConditional(json.RawMessage(`{}`), s.directoryIDLocked(), ""); !errors.Is(err, errRevisionRequired) {
		t.Fatal(err)
	}
	after, _ := s.load()
	if string(before) != string(after) {
		t.Fatal("rejected write changed the archive")
	}
}

func TestDataHTTPRequiresRevision(t *testing.T) {
	old := disk
	disk = libraryTestStore(t)
	defer func() { disk = old }()
	mux := http.NewServeMux()
	registerAPI(mux)
	load := httptest.NewRecorder()
	mux.ServeHTTP(load, httptest.NewRequest("GET", "/api/data", nil))
	var snapshot struct {
		Revision string `json:"revision"`
	}
	json.Unmarshal(load.Body.Bytes(), &snapshot)
	for index, expected := range []int{200, 409, 428} {
		r := httptest.NewRequest("PUT", "/api/data", strings.NewReader(`{"data":{"writings":[{"id":"first"}]}}`))
		if index < 2 {
			r.Header.Set("If-Match", snapshot.Revision)
		}
		r.Header.Set("X-ELP-Directory", disk.directoryIDLocked())
		result := httptest.NewRecorder()
		mux.ServeHTTP(result, r)
		if result.Code != expected {
			t.Fatalf("got %d want %d: %s", result.Code, expected, result.Body.String())
		}
	}
}

func TestDirectoryLockChild(t *testing.T) {
	directory := os.Getenv("ELP_LOCK_CHILD_DIRECTORY")
	if directory == "" {
		return
	}
	handle, err := lockDataDirectory(directory)
	if err != nil {
		os.Exit(23)
	}
	handle.Close()
	os.Exit(0)
}

func TestDirectoryLockIsCrossProcessAndReleases(t *testing.T) {
	directory := t.TempDir()
	handle, err := lockDataDirectory(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	run := func() error {
		cmd := exec.Command(os.Args[0], "-test.run=^TestDirectoryLockChild$")
		cmd.Env = append(os.Environ(), "ELP_LOCK_CHILD_DIRECTORY="+directory)
		return cmd.Run()
	}
	var exit *exec.ExitError
	if err := run(); !errors.As(err, &exit) || exit.ExitCode() != 23 {
		t.Fatalf("second process acquired active directory: %v", err)
	}
	handle.Close()
	if err := run(); err != nil {
		t.Fatalf("kernel lock survived owner exit: %v", err)
	}
}

func TestBackupFailurePreservesPrimary(t *testing.T) {
	s := libraryTestStore(t)
	if err := s.save(json.RawMessage(`{"writings":[{"id":"safe"}]}`)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.directory, dataFilename)
	before, _ := os.ReadFile(path)
	if err := os.Mkdir(filepath.Join(s.directory, backupName), 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.save(json.RawMessage(`{"writings":[]}`)); err == nil {
		t.Fatal("backup failure was ignored")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("primary changed after backup failure")
	}
}
