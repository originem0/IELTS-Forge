package main

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestLibraryIndexReadsOnlyChangedRecords(t *testing.T) {
	s := libraryTestStore(t)
	id, err := s.importPack(sampleLibraryPack())
	if err != nil {
		t.Fatal(err)
	}
	var last libraryAttempt
	for i := 0; i < 120; i++ {
		last, err = s.saveAttempt(libraryAttempt{ID: fmt.Sprintf("record-%d", i), PackID: id, UnitID: "reading-1", Mode: "practice", Status: "draft", Answers: map[string][]string{}})
		if err != nil {
			t.Fatal(err)
		}
	}
	result, err := s.indexedSummary(s.directoryIDLocked())
	if err != nil {
		t.Fatal(err)
	}
	if result["counts"].(map[string]int)["reading"] != 120 {
		t.Fatal(result)
	}
	index := s.libraryIndex
	if index.packReads != 1 || index.attemptReads != 120 {
		t.Fatalf("unexpected cold reads: %d/%d", index.packReads, index.attemptReads)
	}
	if _, err := s.indexedSummary(s.directoryIDLocked()); err != nil {
		t.Fatal(err)
	}
	if index.packReads != 1 || index.attemptReads != 120 {
		t.Fatal("unchanged request reread full contents")
	}
	last.Status = "submitted"
	if _, err := s.saveAttempt(last); err != nil {
		t.Fatal(err)
	}
	result, err = s.indexedSummary(s.directoryIDLocked())
	if err != nil {
		t.Fatal(err)
	}
	if index.attemptReads != 121 || result["counts"].(map[string]int)["completed"] != 1 {
		t.Fatal("single update failed to adjust indexed summary")
	}
	// A blocked catalogue consumer must not hold up autosaving the main archive.
	index.Lock()
	saved := make(chan error, 1)
	go func() { saved <- s.save(json.RawMessage(`{"writings":[]}`)) }()
	select {
	case err := <-saved:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Error("index lock blocked autosave")
	}
	index.Unlock()
}
