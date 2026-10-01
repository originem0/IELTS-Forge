package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStudyVersionCleanupKeepsRecoveryPoints(t *testing.T) {
	s := libraryTestStore(t)
	snapshot, _ := s.snapshot()
	revision := snapshot["revision"].(string)
	for i := 0; i < 40; i++ {
		raw := json.RawMessage(fmt.Sprintf(`{"id":"draft","essay":"version %d"}`, i))
		result, err := s.patchStudy(studyPatch{Collections: map[string]studyCollectionPatch{"writings": {Upsert: []json.RawMessage{raw}}}}, s.directoryIDLocked(), revision)
		if err != nil {
			t.Fatal(err)
		}
		revision = result["revision"].(string)
	}
	files, err := os.ReadDir(filepath.Join(s.directory, "library", "study-records"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) > 20 {
		t.Fatalf("unused edit versions accumulated: %d", len(files))
	}
	for i := 0; i < 5; i++ {
		s.studyWrites = 0 // a new launcher process resets its maintenance counter
		raw := json.RawMessage(fmt.Sprintf(`{"id":"draft","essay":"short session %d"}`, i))
		result, err := s.patchStudy(studyPatch{Collections: map[string]studyCollectionPatch{"writings": {Upsert: []json.RawMessage{raw}}}}, s.directoryIDLocked(), revision)
		if err != nil {
			t.Fatal(err)
		}
		revision = result["revision"].(string)
	}
	files, err = os.ReadDir(filepath.Join(s.directory, "library", "study-records"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) > 4 {
		t.Fatal("repeated short sessions bypassed version cleanup")
	}
	for _, path := range []string{s.dataPathLocked(), s.backupPathLocked()} {
		if _, err := loadDataFromPath(path, ""); err != nil {
			t.Fatal("cleanup broke recovery point", err)
		}
	}
}

func TestDirectorySwitchCopiesExternalMedia(t *testing.T) {
	s := libraryTestStore(t)
	s.configPath = filepath.Join(t.TempDir(), "config.json")
	snapshot, _ := s.snapshot()
	_, err := s.patchStudy(studyPatch{Collections: map[string]studyCollectionPatch{"speaking": {Upsert: []json.RawMessage{json.RawMessage(`{"id":"voice","audio":"data:audio/webm;base64,dGVzdA==","transcript":"test"}`)}}}}, s.directoryIDLocked(), snapshot["revision"].(string))
	if err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	locked, err := lockDataDirectory(target)
	if err != nil {
		t.Fatal(err)
	}
	previous := s.directory
	if _, err := s.switchDirectory(target); err == nil {
		t.Fatal("switch accepted an occupied target")
	}
	if s.directory != previous {
		t.Fatal("failed switch changed current directory")
	}
	locked.Close()
	if _, err := s.switchDirectory(target); err != nil {
		t.Fatal(err)
	}
	data, err := s.load()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.validateStudyMedia(data); err != nil {
		t.Fatal("switch lost recording", err)
	}
}

func TestStudyPatchMigratesMediaAndWritesOnlyChangedRecords(t *testing.T) {
	s := libraryTestStore(t)
	audio := "data:audio/webm;base64," + base64.StdEncoding.EncodeToString([]byte("recorded fixture bytes"))
	legacy, _ := json.Marshal(map[string]any{"writings": []any{map[string]any{"id": "one", "essay": "first"}, map[string]any{"id": "two", "essay": "unchanged"}}, "speaking": []any{map[string]any{"id": "voice", "audio": audio, "transcript": "original"}}, "opaque": map[string]any{"preserved": true}})
	if err := s.save(legacy); err != nil {
		t.Fatal(err)
	}
	snap, _ := s.snapshot()
	result, err := s.patchStudy(studyPatch{Collections: map[string]studyCollectionPatch{"writings": {Upsert: []json.RawMessage{json.RawMessage(`{"id":"one","essay":"edited"}`)}}}}, s.directoryIDLocked(), snap["revision"].(string))
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := readDiskEnvelope(s.dataPathLocked(), s.backupPathLocked())
	if err != nil || envelope.Version != 2 {
		t.Fatalf("migration failed: %v", err)
	}
	if strings.Contains(string(envelope.Data), "base64") || strings.Contains(string(envelope.Data), "edited") {
		t.Fatal("manifest embeds record content")
	}
	loaded, err := s.load()
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Speaking []struct {
			Audio string `json:"audio"`
		} `json:"speaking"`
		Opaque map[string]bool `json:"opaque"`
	}
	json.Unmarshal(loaded, &state)
	if !state.Opaque["preserved"] || !strings.HasPrefix(state.Speaking[0].Audio, "/api/study/media/") {
		t.Fatal(string(loaded))
	}
	mediaPath := filepath.Join(s.directory, "library", "study-media", strings.TrimPrefix(state.Speaking[0].Audio, "/api/study/media/"))
	media, err := os.ReadFile(mediaPath)
	if err != nil || string(media) != "recorded fixture bytes" {
		t.Fatal("media changed during migration")
	}
	backup, err := os.ReadFile(s.backupPathLocked())
	if err != nil || !strings.Contains(string(backup), audio) {
		t.Fatal("original embedded-media backup not retained")
	}
	directory := filepath.Join(s.directory, "library", "study-records")
	before, _ := os.ReadDir(directory)
	_, err = s.patchStudy(studyPatch{Collections: map[string]studyCollectionPatch{"writings": {Upsert: []json.RawMessage{json.RawMessage(`{"id":"one","essay":"edited again"}`)}}}}, s.directoryIDLocked(), result["revision"].(string))
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadDir(directory)
	if len(after) != len(before)+1 {
		t.Fatal("editing one record must create exactly one record version")
	}
	archive, err := s.exportArchive()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { archive.Close(); os.Remove(archive.Name()) }()
	info, _ := archive.Stat()
	target := libraryTestStore(t)
	target.configPath = filepath.Join(t.TempDir(), "config.json")
	if _, err := target.restoreArchive(archive, info.Size(), target.directoryIDLocked()); err != nil {
		t.Fatal(err)
	}
	restored, err := target.load()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(restored), "edited again") {
		t.Fatal("restored archive lost incremental records")
	}
}
