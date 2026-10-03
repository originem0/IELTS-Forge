package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestStudyRecoveryKeepsHealthyRecordsAndOriginals(t *testing.T) {
	s := libraryTestStore(t)
	if err := s.save(json.RawMessage(`{"writings":[],"speaking":[],"mistakes":[]}`)); err != nil {
		t.Fatal(err)
	}
	initial, _ := s.snapshot()
	first, err := s.patchStudy(studyPatch{Collections: map[string]studyCollectionPatch{"writings": {Upsert: []json.RawMessage{json.RawMessage(`{"id":"bad","essay":"lost"}`), json.RawMessage(`{"id":"healthy","essay":"preserved"}`)}}}}, "", initial["revision"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.patchStudy(studyPatch{Metadata: map[string]json.RawMessage{"marker": json.RawMessage(`"newest"`)}}, "", first["revision"].(string)); err != nil {
		t.Fatal(err)
	}
	envelope, _ := readDiskEnvelope(s.dataPathLocked(), "")
	var manifest studyManifest
	_ = json.Unmarshal(envelope.Data, &manifest)
	path, _ := s.libraryPathLocked("study-records", manifest.Collections["writings"][0].Hash+".json")
	if err := os.WriteFile(path, []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	primary, _ := os.ReadFile(s.dataPathLocked())
	backup, _ := os.ReadFile(s.backupPathLocked())
	preview, err := s.previewStudyRecoveryLocked()
	if err != nil {
		t.Fatal(err)
	}
	if preview.Counts["writings"] != 1 || len(preview.Issues) != 1 || !bytes.Contains(preview.Data["writings"], []byte("preserved")) {
		t.Fatalf("wrong salvage: %+v", preview)
	}
	previous := disk
	disk = s
	defer func() { disk = previous }()
	mux := http.NewServeMux()
	registerStudyRecoveryAPI(mux)
	request := func(token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/data/recovery", bytes.NewBufferString(`{"token":"`+token+`"}`))
		r.Header.Set("X-ELP-Directory", s.directoryIDLocked())
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	if w := request("stale"); w.Code != 409 {
		t.Fatal("stale preview accepted")
	}
	if w := request(preview.Token); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	for path, expected := range map[string][]byte{s.dataPathLocked(): primary, s.backupPathLocked(): backup} {
		raw, err := os.ReadFile(path + ".damaged-" + hashContent(expected))
		if err != nil || !bytes.Equal(raw, expected) {
			t.Fatal("original lost", err)
		}
	}
	data, err := s.load()
	if err != nil || !bytes.Contains(data, []byte("preserved")) || !bytes.Contains(data, []byte("newest")) {
		t.Fatal("recovered state invalid", err)
	}
	backupBytes(t, s)
}
