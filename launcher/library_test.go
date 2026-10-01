package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sampleLibraryPack() libraryPack {
	return libraryPack{Version: 1, Title: "Synthetic reading", Source: librarySource{Name: "Test fixture", Status: "generated"}, Units: []libraryUnit{{ID: "reading-1", Skill: "reading", Part: "academic", Title: "A local library", Prompt: "Read and answer.", Minutes: 20,
		Passages: []libraryParagraph{{ID: "A", Text: "The library opens on Monday."}},
		Groups:   []libraryGroup{{ID: "g1", Kind: "text", Instruction: "ONE WORD ONLY", MaxWords: 1, Questions: []libraryQuestion{{ID: "q1", Label: "1", Text: "The library opens on ____.", Answers: []string{"Monday"}, Evidence: "A"}}}},
	}}}
}

func libraryTestStore(t *testing.T) *diskStore {
	t.Helper()
	store := &diskStore{directory: t.TempDir()}
	t.Cleanup(store.close)
	return store
}

func TestLibraryImportIsImmutableAndLeavesLegacyDataAlone(t *testing.T) {
	s := libraryTestStore(t)
	legacy := json.RawMessage(`{"writings":[{"id":"old","answer":"preserve"}],"legacyReading":{"anything":true}}`)
	if err := s.save(legacy); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(s.directory, dataFilename))
	pack := sampleLibraryPack()
	id, err := s.importPack(pack)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := s.importPack(pack)
	if err != nil || duplicate != id {
		t.Fatal("duplicate import must be idempotent", err)
	}
	pack.Units[0].Title = "Revised title"
	revised, err := s.importPack(pack)
	if err != nil || revised == id {
		t.Fatal("updates must create immutable versions", err)
	}
	restarted := &diskStore{directory: s.directory}
	old, err := restarted.loadPackLocked(id)
	if err != nil || old.Units[0].Title != "A local library" {
		t.Fatal("old question changed", err)
	}
	after, _ := os.ReadFile(filepath.Join(s.directory, dataFilename))
	if !bytes.Equal(before, after) {
		t.Fatal("import modified legacy data")
	}
}

func TestLibraryRejectsMalformedPacksBeforeWriting(t *testing.T) {
	cases := map[string]func(*libraryPack){
		"traversal":            func(p *libraryPack) { p.Units[0].ID = "../escape" },
		"unknown skill":        func(p *libraryPack) { p.Units[0].Skill = "alien" },
		"unknown part":         func(p *libraryPack) { p.Units[0].Part = "1" },
		"combined parts":       func(p *libraryPack) { p.Units[0].Part = "academic general" },
		"missing passage":      func(p *libraryPack) { p.Units[0].Passages = nil },
		"missing answer":       func(p *libraryPack) { p.Units[0].Groups[0].Questions[0].Answers = nil },
		"unknown evidence":     func(p *libraryPack) { p.Units[0].Groups[0].Questions[0].Evidence = "B" },
		"missing option":       func(p *libraryPack) { p.Units[0].Groups[0].Kind = "single" },
		"unknown related unit": func(p *libraryPack) { p.Units[0].Related = []string{"missing"} },
		"unsafe source":        func(p *libraryPack) { p.Source.URL = "javascript:alert(1)" },
		"missing media":        func(p *libraryPack) { p.Units[0].Images = []string{strings.Repeat("a", 64) + ".png"} },
		"duplicate question": func(p *libraryPack) {
			p.Units[0].Groups[0].Questions = append(p.Units[0].Groups[0].Questions, p.Units[0].Groups[0].Questions[0])
		},
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			s := libraryTestStore(t)
			p := sampleLibraryPack()
			edit(&p)
			if _, err := s.importPack(p); err == nil {
				t.Fatal("invalid pack accepted")
			}
			entries, _ := os.ReadDir(filepath.Join(s.directory, "library", "packs"))
			if len(entries) != 0 {
				t.Fatal("invalid pack persisted")
			}
		})
	}
	if _, err := (&diskStore{}).importPack(sampleLibraryPack()); err == nil {
		t.Fatal("unbound import accepted")
	}
}

func TestLibraryAttemptsPersistConflictAndPreserveSubmittedHistory(t *testing.T) {
	s := libraryTestStore(t)
	id, err := s.importPack(sampleLibraryPack())
	if err != nil {
		t.Fatal(err)
	}
	a := libraryAttempt{ID: "attempt-1", PackID: id, UnitID: "reading-1", Mode: "practice", Status: "draft", Answers: map[string][]string{"q1": {"Monday"}}}
	saved, err := s.saveAttempt(a)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 1 || saved.CreatedAt == "" {
		t.Fatal("missing server revision/time")
	}
	if _, err := s.saveAttempt(a); !errors.Is(err, errAttemptConflict) {
		t.Fatal("stale write not rejected", err)
	}
	saved.Status = "submitted"
	saved, err = s.saveAttempt(saved)
	if err != nil {
		t.Fatal(err)
	}
	restarted := &diskStore{directory: s.directory}
	loaded, err := restarted.loadAttemptLocked(a.ID)
	if err != nil || loaded.Status != "submitted" || loaded.Answers["q1"][0] != "Monday" {
		t.Fatal("restart lost answer", err)
	}
	if _, err := restarted.saveAttempt(loaded); err == nil {
		t.Fatal("submitted history overwritten")
	}
	raw, err := os.ReadFile(filepath.Join(s.directory, "library", "attempts", a.ID+".backup.json"))
	if err != nil {
		t.Fatal(err)
	}
	var backup libraryAttempt
	if json.Unmarshal(raw, &backup) != nil || backup.Status != "draft" {
		t.Fatal("backup missing prior state")
	}
	review := a
	review.ID = "review-1"
	review.ReviewOf = a.ID
	if _, err := s.saveAttempt(review); err != nil {
		t.Fatal("cannot review submitted attempt", err)
	}
	review.ID = "review-2"
	review.ReviewOf = "missing"
	if _, err := s.saveAttempt(review); err == nil {
		t.Fatal("unknown review source accepted")
	}
	bad := a
	bad.ID = "bad"
	bad.Answers = map[string][]string{"unknown": {"Monday"}}
	if _, err := s.saveAttempt(bad); err == nil {
		t.Fatal("unknown question accepted")
	}
}

func TestLibraryRoutesMediaRangeStrictJSONAndDirectoryIsolation(t *testing.T) {
	previous := disk
	disk = libraryTestStore(t)
	defer func() { disk = previous }()
	mux := http.NewServeMux()
	registerAPI(mux)
	call := func(method, path, contentType string, body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Content-Type", contentType)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	raw, _ := json.Marshal(sampleLibraryPack())
	for _, invalid := range [][]byte{append(append([]byte{}, raw...), []byte(` {}`)...), bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"unknown":true`), 1)} {
		if w := call("POST", "/api/library/packs", "application/json", invalid); w.Code != 400 {
			t.Fatal("invalid JSON accepted", w.Code)
		}
	}
	w := call("POST", "/api/library/packs", "application/json", raw)
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var imported map[string]string
	json.Unmarshal(w.Body.Bytes(), &imported)
	if w := call("GET", "/api/library/packs/"+imported["id"], "", nil); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	// Minimal RIFF/WAVE header exercises real MIME detection and byte-range replay.
	audio := []byte("RIFF\x18\x00\x00\x00WAVEfmt \x10\x00\x00\x00test-audio-data")
	w = call("POST", "/api/library/media", "audio/wav", audio)
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var media map[string]string
	json.Unmarshal(w.Body.Bytes(), &media)
	r := httptest.NewRequest("GET", "/api/library/media/"+media["id"], nil)
	r.Header.Set("Range", "bytes=0-3")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 206 || w.Body.String() != "RIFF" {
		t.Fatal("audio seeking failed", w.Code, w.Body.String())
	}
	if w := call("POST", "/api/library/media", "audio/wav", []byte("<script>bad</script>")); w.Code != 400 {
		t.Fatal("invalid media accepted")
	}
	p := sampleLibraryPack()
	p.Units[0].Skill = "listening"
	p.Units[0].Part = "1"
	p.Units[0].Audio = media["id"]
	p.Units[0].Transcript = "The library opens on Monday."
	if _, err := disk.importPack(p); err != nil {
		t.Fatal("valid attached audio rejected", err)
	}
	// A new bound directory must never see the prior folder's imported bank.
	disk.directory = t.TempDir()
	w = call("GET", "/api/library/packs", "", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"packs":[]`) {
		t.Fatal("directory isolation failed", w.Body.String())
	}
}

func TestLibraryDetectsCorruptionAndRejectsSymlinkEscape(t *testing.T) {
	s := libraryTestStore(t)
	id, err := s.importPack(sampleLibraryPack())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.directory, "library", "packs", id+".json")
	if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.loadPackLocked(id); err == nil {
		t.Fatal("corrupt pack accepted")
	}
	if _, err := s.importPack(sampleLibraryPack()); err == nil {
		t.Fatal("silently overwrote corrupt version")
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "library")); err != nil {
		t.Skip("symlink creation unavailable", err)
	}
	if _, err := (&diskStore{directory: root}).importPack(sampleLibraryPack()); err == nil {
		t.Fatal("symlink escape accepted")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) > 0 {
		t.Fatal("wrote outside bound directory")
	}
}

func TestLibraryConcurrentSavesAllowOnlyOneRevision(t *testing.T) {
	s := libraryTestStore(t)
	id, err := s.importPack(sampleLibraryPack())
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.saveAttempt(libraryAttempt{ID: "concurrent", PackID: id, UnitID: "reading-1", Mode: "practice", Status: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for _, answer := range []string{"Monday", "Tuesday"} {
		go func(value string) {
			candidate := a
			candidate.Answers = map[string][]string{"q1": {value}}
			_, err := s.saveAttempt(candidate)
			results <- err
		}(answer)
	}
	success, conflict := 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			success++
		} else if errors.Is(err, errAttemptConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
}

func TestLibraryRejectsInvalidHighlightsAndReviewScope(t *testing.T) {
	s := libraryTestStore(t)
	id, err := s.importPack(sampleLibraryPack())
	if err != nil {
		t.Fatal(err)
	}
	a := libraryAttempt{ID: "highlight", PackID: id, UnitID: "reading-1", Mode: "practice", Status: "draft", Answers: map[string][]string{}}
	for _, ranges := range [][]libraryHighlight{{{ParagraphID: "A", Start: 0, End: 999}}, {{ParagraphID: "missing", Start: 0, End: 1}}, {{ParagraphID: "A", Start: 0, End: 5}, {ParagraphID: "A", Start: 4, End: 8}}} {
		a.Highlights = ranges
		if _, err := s.saveAttempt(a); err == nil {
			t.Fatal("invalid highlight accepted")
		}
	}
	a.Highlights = []libraryHighlight{{ParagraphID: "A", Start: 0, End: 3}}
	a.Status = "submitted"
	a.Answers = map[string][]string{"q1": {"Monday"}}
	if _, err := s.saveAttempt(a); err != nil {
		t.Fatal(err)
	}
	a.ID = "review-highlight"
	a.Status = "draft"
	a.ReviewOf = "highlight"
	a.ReviewQuestions = []string{"q1"}
	a.Answers = map[string][]string{}
	if _, err := s.saveAttempt(a); err == nil {
		t.Fatal("correct answer accepted as wrong-only review")
	}
}
