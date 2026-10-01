package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fullBackupFixture(t *testing.T) (*diskStore, string, string) {
	t.Helper()
	s := libraryTestStore(t)
	s.configPath = filepath.Join(t.TempDir(), "config.json")
	if err := s.save(json.RawMessage(`{"writings":[{"id":"old","essay":"preserved"}],"speaking":[],"legacy":{"unknown":true}}`)); err != nil {
		t.Fatal(err)
	}
	media := []byte("RIFFxxxxWAVEtest-media")
	mediaID := hashContent(media) + ".wav"
	path, err := s.libraryPathLocked("media", mediaID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, media, 0600); err != nil {
		t.Fatal(err)
	}
	pack := sampleLibraryPack()
	pack.Units[0].Audio = mediaID
	id, err := s.importPack(pack)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(s.directory, "library", "packs", id+".json"), time.Unix(1000000000, 123456700), time.Unix(1000000000, 123456700)); err != nil {
		t.Fatal(err)
	}
	a, err := s.saveAttempt(libraryAttempt{ID: "one", PackID: id, UnitID: "reading-1", Mode: "practice", Status: "draft", Answers: map[string][]string{"q1": {"Monday"}}})
	if err != nil {
		t.Fatal(err)
	}
	a.Status = "submitted"
	if _, err := s.saveAttempt(a); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.directory, "ai-credentials.dpapi"), []byte("synthetic-excluded"), 0600); err != nil {
		t.Fatal(err)
	}
	return s, id, mediaID
}

func backupBytes(t *testing.T, s *diskStore) []byte {
	t.Helper()
	file, err := s.exportArchive()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { file.Close(); os.Remove(file.Name()) }()
	raw, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestFullBackupRestorePreservesOldDirectoryAndAllLearningFiles(t *testing.T) {
	s, id, mediaID := fullBackupFixture(t)
	raw := backupBytes(t, s)
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range archive.File {
		if strings.Contains(file.Name, "credential") || strings.Contains(file.Name, "config.json") {
			t.Fatal("credentials/config entered backup")
		}
	}
	target := libraryTestStore(t)
	target.configPath = filepath.Join(t.TempDir(), "config.json")
	if err := target.save(json.RawMessage(`{"writings":[{"id":"keep-original"}]}`)); err != nil {
		t.Fatal(err)
	}
	oldDirectory, oldContext := target.directory, target.directoryIDLocked()
	before, _ := os.ReadFile(filepath.Join(oldDirectory, dataFilename))
	restored, err := target.restoreArchive(bytes.NewReader(raw), int64(len(raw)), oldContext)
	if err != nil {
		t.Fatal(err)
	}
	if restored == oldDirectory || target.directory != restored {
		t.Fatal("restore should use a separate archive")
	}
	after, _ := os.ReadFile(filepath.Join(oldDirectory, dataFilename))
	if !bytes.Equal(before, after) {
		t.Fatal("restore overwrote original directory")
	}
	data, err := target.load()
	if err != nil || !bytes.Contains(data, []byte(`"unknown":true`)) {
		t.Fatal("legacy fields missing", err)
	}
	if _, err := target.loadPackLocked(id); err != nil {
		t.Fatal(err)
	}
	media, err := os.ReadFile(filepath.Join(restored, "library", "media", mediaID))
	if err != nil || !bytes.Contains(media, []byte("test-media")) {
		t.Fatal("media missing", err)
	}
	attempt, err := target.loadAttemptLocked("one")
	if err != nil || attempt.Status != "submitted" || attempt.Revision != 2 {
		t.Fatal("attempt missing", err)
	}
	if _, err := os.Stat(filepath.Join(restored, "library", "attempts", "one.backup.json")); err != nil {
		t.Fatal("rolling attempt backup missing", err)
	}
	info, _ := os.Stat(filepath.Join(restored, "library", "packs", id+".json"))
	if !info.ModTime().Equal(time.Unix(1000000000, 123456700)) {
		t.Fatal("import ordering timestamp changed")
	}
	if err := target.save(json.RawMessage(`{}`), oldContext); !errors.Is(err, errDirectoryChanged) {
		t.Fatal("old tab could overwrite restored data", err)
	}
	if err := target.save(json.RawMessage(`{}`), ""); !errors.Is(err, errDirectoryChanged) {
		t.Fatal("missing directory context accepted", err)
	}
	config, _ := os.ReadFile(target.configPath)
	var parsed launcherConfig
	if json.Unmarshal(config, &parsed) != nil || parsed.DataDirectory != restored {
		t.Fatal("restored directory binding not durable")
	}
}

func rewriteBackup(t *testing.T, raw []byte, change func(map[string][]byte)) []byte {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, file := range archive.File {
		input, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		files[file.Name], err = io.ReadAll(input)
		input.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	change(files)
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for name, data := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestFullBackupRejectsTamperingTraversalAndBrokenReferences(t *testing.T) {
	s, _, _ := fullBackupFixture(t)
	original := backupBytes(t, s)
	cases := map[string]func(map[string][]byte){
		"traversal":        func(files map[string][]byte) { files["../escape.json"] = []byte(`{}`) },
		"credentials":      func(files map[string][]byte) { files["runtime-data/ai-credentials.dpapi"] = []byte("bad") },
		"checksum":         func(files map[string][]byte) { files[dataFilename] = []byte(`{"version":1,"data":{}}`) },
		"missing manifest": func(files map[string][]byte) { delete(files, backupManifestName) },
		"broken reference with valid checksum": func(files map[string][]byte) {
			name := "library/attempts/one.json"
			var attempt libraryAttempt
			json.Unmarshal(files[name], &attempt)
			attempt.UnitID = "absent"
			files[name], _ = json.Marshal(attempt)
			var manifest backupManifest
			json.Unmarshal(files[backupManifestName], &manifest)
			for i := range manifest.Files {
				if manifest.Files[i].Path == name {
					manifest.Files[i].SHA256 = hashContent(files[name])
					manifest.Files[i].Size = int64(len(files[name]))
				}
			}
			files[backupManifestName], _ = json.Marshal(manifest)
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			raw := rewriteBackup(t, original, change)
			target := libraryTestStore(t)
			target.configPath = filepath.Join(t.TempDir(), "config.json")
			before := target.directory
			if _, err := target.restoreArchive(bytes.NewReader(raw), int64(len(raw)), target.directoryIDLocked()); err == nil {
				t.Fatal("bad archive accepted")
			}
			if target.directory != before {
				t.Fatal("failed restore changed binding")
			}
			entries, _ := os.ReadDir(before)
			if len(entries) != 0 {
				t.Fatalf("failed restore left partial data: %v", entries)
			}
		})
	}
}

func TestFullBackupRefusesDamagedMediaAndSupportsLegacyOnly(t *testing.T) {
	s, _, mediaID := fullBackupFixture(t)
	if err := os.WriteFile(filepath.Join(s.directory, "library", "media", mediaID), []byte("damaged"), 0600); err != nil {
		t.Fatal(err)
	}
	if file, err := s.exportArchive(); err == nil {
		file.Close()
		os.Remove(file.Name())
		t.Fatal("damaged media exported")
	}
	legacy := libraryTestStore(t)
	if err := legacy.save(json.RawMessage(`{"writings":[],"speaking":[],"oldReading":[1]}`)); err != nil {
		t.Fatal(err)
	}
	raw := backupBytes(t, legacy)
	target := libraryTestStore(t)
	target.configPath = filepath.Join(t.TempDir(), "config.json")
	if _, err := target.restoreArchive(bytes.NewReader(raw), int64(len(raw)), target.directoryIDLocked()); err != nil {
		t.Fatal("legacy-only restore failed", err)
	}
}

func TestBackupDeviceNamesAndMissingPrimary(t *testing.T) {
	for _, name := range []string{"CON", "nul", "COM1", "lpt9"} {
		if safeAttemptID(name) || backupFileLimit("library/attempts/"+name+".json") != 0 {
			t.Fatalf("reserved device name accepted: %s", name)
		}
	}
	s := libraryTestStore(t)
	if err := s.save(json.RawMessage(`{"writings":[{"id":"recover"}]}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.save(json.RawMessage(`{"writings":[]}`)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(s.directory, dataFilename)); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.load()
	if err != nil || !bytes.Contains(loaded, []byte("recover")) {
		t.Fatal("missing primary did not recover rolling backup", err)
	}
	raw := backupBytes(t, s)
	target := libraryTestStore(t)
	target.configPath = filepath.Join(t.TempDir(), "config.json")
	if _, err := target.restoreArchive(bytes.NewReader(raw), int64(len(raw)), target.directoryIDLocked()); err != nil {
		t.Fatal(err)
	}
	loaded, err = target.load()
	if err != nil || !bytes.Contains(loaded, []byte("recover")) {
		t.Fatal("backup exported an empty archive after primary loss", err)
	}
}
