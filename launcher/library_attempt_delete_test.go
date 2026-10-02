package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
)

func TestAttemptDeletePreservesReviewAndRejectsStaleWrites(t *testing.T) {
	previous := disk
	disk = libraryTestStore(t)
	defer func() { disk = previous }()
	packID, err := disk.importPack(sampleLibraryPack())
	if err != nil {
		t.Fatal(err)
	}
	original, err := disk.saveAttempt(libraryAttempt{ID: "original", PackID: packID, UnitID: "reading-1", Mode: "practice", Status: "submitted", Answers: map[string][]string{"q1": {"Tuesday"}}})
	if err != nil {
		t.Fatal(err)
	}
	review, err := disk.saveAttempt(libraryAttempt{ID: "review", PackID: packID, UnitID: "reading-1", Mode: "practice", Status: "draft", ReviewOf: original.ID, ReviewQuestions: []string{"q1"}})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerLibraryAttemptsAPI(mux)
	remove := func(directory, revision string) int {
		req := httptest.NewRequest("DELETE", "/api/library/attempts/original", nil)
		req.Header.Set("X-ELP-Directory", directory)
		req.Header.Set("If-Match", revision)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, req)
		return response.Code
	}
	if code := remove("wrong-directory", strconv.Itoa(original.Revision)); code != 409 {
		t.Fatal(code)
	}
	if code := remove(disk.directoryIDLocked(), "0"); code != 409 {
		t.Fatal(code)
	}
	if code := remove(disk.directoryIDLocked(), strconv.Itoa(original.Revision)); code != 200 {
		t.Fatal(code)
	}
	if _, err := disk.loadAttemptLocked(original.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if _, err := disk.saveAttempt(original); err == nil {
		t.Fatal("stale save recreated deleted attempt")
	}
	review.Answers = map[string][]string{"q1": {"Monday"}}
	review.Status = "submitted"
	if _, err := disk.saveAttempt(review); err != nil {
		t.Fatal("orphaned review cannot be completed", err)
	}
	if _, err := disk.loadPackLocked(packID); err != nil {
		t.Fatal("deletion damaged question pack", err)
	}
}
