package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLibraryDeleteProtectsSavedAndRecoverableReferences(t *testing.T) {
	for _, kind := range []string{"attempt", "attempt-backup", "study-version", "current-study", "archive-backup", "daily-backup"} {
		t.Run(kind, func(t *testing.T) {
			s := libraryTestStore(t)
			if err := s.save(json.RawMessage(`{}`)); err != nil {
				t.Fatal(err)
			}
			id, err := s.importPack(sampleLibraryPack())
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(map[string]any{"questionRef": map[string]string{"packId": id}})
			var path string
			switch kind {
			case "attempt":
				path, _ = s.libraryPathLocked("attempts", "saved.json")
			case "attempt-backup":
				path, _ = s.libraryPathLocked("attempts", "saved.backup.json")
			case "study-version":
				path, _ = s.libraryPathLocked("study-records", hashContent(raw)+".json")
			case "current-study":
				if err = s.save(raw); err != nil {
					t.Fatal(err)
				}
			case "archive-backup":
				path = s.backupPathLocked()
			case "daily-backup":
				dir := filepath.Join(s.directory, "backups")
				if err = os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				path = filepath.Join(dir, "EnglishLearnPath-data-2026-10-01.json")
			}
			if path != "" {
				if err = os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err = s.deleteLibraryPack(id, s.directoryIDLocked()); !errors.Is(err, errLibraryPackInUse) {
				t.Fatal("referenced pack deletion not blocked", err)
			}
			if _, err = s.loadPackLocked(id); err != nil {
				t.Fatal("history lost original pack", err)
			}
		})
	}
}

func TestLibraryDeleteRemovesOnlyUnreferencedPack(t *testing.T) {
	s := libraryTestStore(t)
	if err := s.save(json.RawMessage(`{"note":"keep"}`)); err != nil {
		t.Fatal(err)
	}
	id, err := s.importPack(sampleLibraryPack())
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.dataPathLocked())
	if err = s.deleteLibraryPack(id, "wrong-directory"); !errors.Is(err, errDirectoryChanged) {
		t.Fatal("wrong-directory deletion accepted", err)
	}
	if err = s.deleteLibraryPack(id, s.directoryIDLocked()); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(s.directory, "library", "packs", id+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("pack not removed", err)
	}
	after, _ := os.ReadFile(s.dataPathLocked())
	if string(before) != string(after) {
		t.Fatal("learning archive changed")
	}
}
