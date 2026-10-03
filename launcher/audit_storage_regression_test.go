package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupRestoreSlowUploadAndInterruptedRetry(t *testing.T) {
	source, _, _ := fullBackupFixture(t)
	raw := backupBytes(t, source)
	target := libraryTestStore(t)
	target.configPath = filepath.Join(t.TempDir(), "config.json")
	previous := disk
	disk = target
	t.Cleanup(func() { disk = previous })
	mux := http.NewServeMux()
	registerBackupAPI(mux)
	server := httptest.NewUnstartedServer(mux)
	server.Config.ReadTimeout = 50 * time.Millisecond
	server.Start()
	defer server.Close()
	originalDirectory := target.directory
	for _, interrupted := range []bool{true, false} {
		reader, writer := io.Pipe()
		req, err := http.NewRequest("POST", server.URL+"/api/backup/restore", reader)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-ELP-Directory", target.directoryIDLocked())
		go func() {
			defer writer.Close()
			if _, err := writer.Write(raw[:len(raw)/2]); err != nil {
				return
			}
			time.Sleep(150 * time.Millisecond)
			if !interrupted {
				_, _ = writer.Write(raw[len(raw)/2:])
			}
		}()
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if interrupted {
			if response.StatusCode != 400 || target.directory != originalDirectory {
				t.Fatalf("interrupted upload changed archive: %d %s", response.StatusCode, body)
			}
		} else if response.StatusCode != 200 {
			t.Fatalf("slow retry failed: %d %s", response.StatusCode, body)
		}
	}
	if _, err := target.loadAttemptLocked("one"); err != nil {
		t.Fatal(err)
	}
}

func TestDeletedReviewSourceBackupRestoreContinue(t *testing.T) {
	s := libraryTestStore(t)
	if err := s.save(json.RawMessage(`{"writings":[],"speaking":[]}`)); err != nil {
		t.Fatal(err)
	}
	pack, err := s.importPack(sampleLibraryPack())
	if err != nil {
		t.Fatal(err)
	}
	original, err := s.saveAttempt(libraryAttempt{ID: "original", PackID: pack, UnitID: "reading-1", Mode: "practice", Status: "submitted", Answers: map[string][]string{"q1": {"Tuesday"}}})
	if err != nil {
		t.Fatal(err)
	}
	review, err := s.saveAttempt(libraryAttempt{ID: "review", PackID: pack, UnitID: "reading-1", Mode: "practice", Status: "draft", ReviewOf: original.ID, ReviewQuestions: []string{"q1"}})
	if err != nil {
		t.Fatal(err)
	}
	path, err := s.libraryPathLocked("attempts", "original.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	review.Answers = map[string][]string{"q1": {"Monday"}}
	if _, err := s.saveAttempt(review); err != nil {
		t.Fatal(err)
	}
	raw := backupBytes(t, s)
	target := libraryTestStore(t)
	target.configPath = filepath.Join(t.TempDir(), "config.json")
	if _, err := target.restoreArchive(bytes.NewReader(raw), int64(len(raw)), target.directoryIDLocked()); err != nil {
		t.Fatal(err)
	}
	restored, err := target.loadAttemptLocked("review")
	if err != nil {
		t.Fatal(err)
	}
	if restored.ReviewOf != original.ID || len(restored.ReviewQuestions) != 1 || restored.ReviewQuestions[0] != "q1" {
		t.Fatal("lost immutable review scope")
	}
	restored.Status = "submitted"
	if _, err := target.saveAttempt(restored); err != nil {
		t.Fatal(err)
	}
	backupBytes(t, target)
	// Missing parents may not be used to create a new, unverified review.
	restored.ID = "invented"
	restored.Revision = 0
	if _, err := target.saveAttempt(restored); err == nil {
		t.Fatal("accepted unverified orphan review")
	}
}

func TestFutureArchiveRefusesFallbackAndEveryWrite(t *testing.T) {
	s := libraryTestStore(t)
	old := json.RawMessage(`{"marker":"old"}`)
	if err := s.save(old); err != nil {
		t.Fatal(err)
	}
	if err := s.save(json.RawMessage(`{"marker":"new"}`)); err != nil {
		t.Fatal(err)
	}
	future := []byte(`{"version":99,"data":{"marker":"future"}}`)
	if err := os.WriteFile(s.dataPathLocked(), future, 0600); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(s.backupPathLocked())
	if err != nil {
		t.Fatal(err)
	}
	for name, operation := range map[string]func() error{
		"load":        func() error { _, err := s.load(); return err },
		"snapshot":    func() error { _, err := s.snapshot(); return err },
		"save":        func() error { return s.save(old) },
		"conditional": func() error { _, err := s.saveConditional(old, "", dataRevision(old)); return err },
		"patch": func() error {
			_, err := s.patchStudy(studyPatch{Metadata: map[string]json.RawMessage{"marker": json.RawMessage(`"changed"`)}}, "", dataRevision(old))
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := operation(); !errors.Is(err, errUnsupportedDataVersion) {
				t.Fatalf("expected version error, got %v", err)
			}
			current, err := os.ReadFile(s.dataPathLocked())
			if err != nil || !bytes.Equal(current, future) {
				t.Fatal("future archive changed", err)
			}
			current, err = os.ReadFile(s.backupPathLocked())
			if err != nil || !bytes.Equal(current, backup) {
				t.Fatal("backup changed", err)
			}
		})
	}
}

func TestCorruptStudyRecordUsesCompleteFallbackAndPreservesEvidence(t *testing.T) {
	s := libraryTestStore(t)
	if err := s.save(json.RawMessage(`{"writings":[{"id":"essay","essay":"first"}],"speaking":[]}`)); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, answer := range []string{"second", "third"} {
		result, err := s.patchStudy(studyPatch{Collections: map[string]studyCollectionPatch{"writings": {Upsert: []json.RawMessage{json.RawMessage(`{"id":"essay","essay":"` + answer + `"}`)}}}}, "", snapshot["revision"].(string))
		if err != nil {
			t.Fatal(err)
		}
		snapshot = result
	}
	primary, err := os.ReadFile(s.dataPathLocked())
	if err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(s.backupPathLocked())
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := readDiskEnvelope(s.dataPathLocked(), "")
	if err != nil {
		t.Fatal(err)
	}
	var manifest studyManifest
	if err := json.Unmarshal(envelope.Data, &manifest); err != nil {
		t.Fatal(err)
	}
	path, err := s.libraryPathLocked("study-records", manifest.Collections["writings"][0].Hash+".json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err = s.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(snapshot["data"].(json.RawMessage), []byte("second")) {
		t.Fatal("healthy backup not selected")
	}
	if snapshot["storage"].(map[string]any)["recoverySource"] != s.backupPathLocked() {
		t.Fatal("recovery was silent")
	}
	current, _ := os.ReadFile(s.dataPathLocked())
	if !bytes.Equal(current, primary) {
		t.Fatal("read changed damaged original")
	}
	// Explicit empty save confirms recovery without changing the loaded data.
	if _, err := s.patchStudy(studyPatch{}, "", snapshot["revision"].(string)); err != nil {
		t.Fatal(err)
	}
	current, _ = os.ReadFile(s.backupPathLocked())
	if !bytes.Equal(current, backup) {
		t.Fatal("damaged primary overwrote healthy backup")
	}
	current, err = os.ReadFile(s.dataPathLocked() + ".damaged-" + hashContent(primary))
	if err != nil || !bytes.Equal(current, primary) {
		t.Fatal("damaged original not preserved", err)
	}
	current, err = os.ReadFile(path)
	if err != nil || string(current) != "broken" {
		t.Fatal("damaged record evidence removed", err)
	}
	if _, err := loadDataFromPath(s.dataPathLocked(), ""); err != nil {
		t.Fatal("recovered primary invalid", err)
	}
	// Daily snapshots are another validated recovery point.
	if err := os.WriteFile(s.dataPathLocked(), []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.backupPathLocked(), []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	if recovered, err := s.snapshot(); err != nil || recovered["storage"].(map[string]any)["recoverySource"] == "" {
		t.Fatal("daily backup not recovered", err)
	}
	daily, _ := filepath.Glob(filepath.Join(s.directory, "backups", "*.json"))
	for _, path := range daily {
		if err := os.WriteFile(path, []byte("bad"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.load(); err == nil {
		t.Fatal("all broken snapshots became empty")
	}

}

func TestCorruptAttemptReadsBackupAndPreservesOriginalOnSave(t *testing.T) {
	s, _, _ := fullBackupFixture(t)
	path, _ := s.libraryPathLocked("attempts", "one.json")
	broken := []byte("broken attempt")
	if err := os.WriteFile(path, broken, 0600); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.loadAttemptLocked("one")
	if err != nil || recovered.Revision != 1 {
		t.Fatal("attempt backup not selected", err)
	}
	recovered.Answers = map[string][]string{"q1": {"Tuesday"}}
	if _, err := s.saveAttempt(recovered); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(path + ".damaged-" + hashContent(broken))
	if err != nil || !bytes.Equal(current, broken) {
		t.Fatal("attempt original not preserved", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.loadAttemptLocked("one"); err != nil {
		t.Fatal("missing primary ignored backup", err)
	}
}
