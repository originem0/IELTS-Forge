package main

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestAuditCommitReplayAcrossRestartAndInterveningWrite(t *testing.T) {
	s := libraryTestStore(t)
	if err := s.save(json.RawMessage(`{"writings":[],"speaking":[]}`)); err != nil {
		t.Fatal(err)
	}
	initial, _ := s.snapshot()
	revision := initial["revision"].(string)
	patch := studyPatch{Metadata: map[string]json.RawMessage{"one": json.RawMessage(`1`)}}
	saved, err := s.patchStudy(patch, "", revision, "lost-response")
	if err != nil {
		t.Fatal(err)
	}
	restarted := &diskStore{directory: s.directory}
	replay, err := restarted.patchStudy(patch, "", revision, "lost-response")
	if err != nil || replay["revision"] != saved["revision"] {
		t.Fatal("receipt did not survive restart", err)
	}
	later, err := s.patchStudy(studyPatch{Metadata: map[string]json.RawMessage{"two": json.RawMessage(`2`)}}, "", saved["revision"].(string), "later")
	if err != nil {
		t.Fatal(err)
	}
	replay, err = s.patchStudy(patch, "", revision, "lost-response")
	if err != nil || replay["revision"] != saved["revision"] || replay["revision"] == later["revision"] {
		t.Fatal("replay adopted another writer", err)
	}
	patch.Metadata["one"] = json.RawMessage(`9`)
	if _, err = s.patchStudy(patch, "", revision, "lost-response"); !errors.Is(err, errDataConflict) {
		t.Fatal("commit ID reused with different payload", err)
	}
	if _, err = s.patchStudy(patch, "", saved["revision"].(string), "fresh"); !errors.Is(err, errDataConflict) {
		t.Fatal("stale write accepted", err)
	}
}

func TestAuditDisjointPatchMergeAndTrueConflict(t *testing.T) {
	s := libraryTestStore(t)
	if err := s.save(json.RawMessage(`{"writings":[{"id":"a","essay":"old"},{"id":"b","essay":"old"}],"speaking":[]}`)); err != nil {
		t.Fatal(err)
	}
	initial, _ := s.snapshot()
	revision := initial["revision"].(string)
	patchFor := func(id, text string) studyPatch {
		return studyPatch{Collections: map[string]studyCollectionPatch{"writings": {Upsert: []json.RawMessage{json.RawMessage(`{"id":"` + id + `","essay":"` + text + `"}`)}}}, Base: &studyPatchBase{Records: map[string]map[string]json.RawMessage{"writings": {id: json.RawMessage(`{"id":"` + id + `","essay":"old"}`)}}}}
	}
	if _, err := s.patchStudy(patchFor("a", "first"), "", revision); err != nil {
		t.Fatal(err)
	}
	second, err := s.patchStudy(patchFor("b", "second"), "", revision)
	if err != nil {
		t.Fatal("disjoint edits conflicted", err)
	}
	if _, err := s.patchStudy(patchFor("a", "overwrite"), "", second["revision"].(string)); !errors.Is(err, errDataConflict) {
		t.Fatal("unseen edit overwritten with current revision", err)
	}
	data, err := s.load()
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	_ = json.Unmarshal(data, &decoded)
	records := decoded["writings"].([]any)
	if records[0].(map[string]any)["essay"] != "first" || records[1].(map[string]any)["essay"] != "second" {
		t.Fatal("merge lost data")
	}
}

func TestAuditAttemptCommitReplay(t *testing.T) {
	s := libraryTestStore(t)
	pack, err := s.importPack(sampleLibraryPack())
	if err != nil {
		t.Fatal(err)
	}
	draft := libraryAttempt{ID: "lost", PackID: pack, UnitID: "reading-1", Mode: "practice", Status: "draft", Answers: map[string][]string{}}
	saved, err := s.saveAttempt(draft, "", "create")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.saveAttempt(draft, "", "create")
	if err != nil || replay.Revision != saved.Revision {
		t.Fatal("create replay failed", err)
	}
	draft = saved
	draft.Answers = map[string][]string{"q1": {"Monday"}}
	saved, err = s.saveAttempt(draft, "", "answer")
	if err != nil {
		t.Fatal(err)
	}
	replay, err = (&diskStore{directory: s.directory}).saveAttempt(draft, "", "answer")
	if err != nil || replay.Revision != saved.Revision {
		t.Fatal("save replay failed", err)
	}
	draft.Answers = map[string][]string{"q1": {"Tuesday"}}
	if _, err := s.saveAttempt(draft, "", "answer"); !errors.Is(err, errAttemptConflict) {
		t.Fatal("reused receipt accepted", err)
	}
}
