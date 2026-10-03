package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestAuditStorageLatency(t *testing.T) {
	if os.Getenv("ELP_STORAGE_BENCH") != "1" {
		t.Skip("storage benchmark opt-in")
	}
	s := libraryTestStore(t)
	records := []map[string]any{}
	for i := 0; i < 1000; i++ {
		records = append(records, map[string]any{"id": fmt.Sprint(i), "essay": strings.Repeat("A saved practice answer. ", 100)})
	}
	raw, _ := json.Marshal(map[string]any{"writings": records, "speaking": []any{map[string]any{"id": "audio", "audio": "data:audio/webm;base64," + base64.StdEncoding.EncodeToString([]byte(strings.Repeat("audio", 100000)))}}, "mistakes": []any{}})
	if err := s.save(raw); err != nil {
		t.Fatal(err)
	}
	initial, _ := s.snapshot()
	migrated, err := s.patchStudy(studyPatch{Metadata: map[string]json.RawMessage{"benchmark": json.RawMessage(`true`)}}, "", initial["revision"].(string))
	if err != nil {
		t.Fatal(err)
	}
	revision := migrated["revision"].(string)
	results := map[string]any{"scope": "Real disk snapshot resolution and fsync saves; excludes UI debounce. Synthetic data, 20 samples per operation.", "writingRecords": 1000, "audioBytes": 500000}
	measure := func(name string, operation func(int) error) {
		samples := []float64{}
		for i := 0; i < 20; i++ {
			start := time.Now()
			if err := operation(i); err != nil {
				t.Fatal(err)
			}
			samples = append(samples, float64(time.Since(start).Microseconds())/1000)
		}
		sort.Float64s(samples)
		results[name] = map[string]any{"p50Ms": samples[9], "p95Ms": samples[18], "maxMs": samples[19]}
	}
	measure("startup", func(int) error { _, err := s.snapshot(); return err })
	save := func(i int, metadata bool) error {
		patch := studyPatch{Collections: map[string]studyCollectionPatch{"writings": {Upsert: []json.RawMessage{json.RawMessage(fmt.Sprintf(`{"id":"0","essay":"Edited answer %d"}`, i))}}}}
		if metadata {
			patch = studyPatch{Metadata: map[string]json.RawMessage{"languagePractice": json.RawMessage(fmt.Sprintf(`{"item":{"attempts":%d}}`, i))}}
		}
		result, err := s.patchStudy(patch, "", revision)
		if err == nil {
			revision = result["revision"].(string)
		}
		return err
	}
	measure("answerCommit", func(i int) error { return save(i, false) })
	measure("reviewCommit", func(i int) error { return save(i, true) })
	backupSaveSamples := []float64{}
	for i := 0; i < 20; i++ {
		done := make(chan error, 1)
		go func() {
			file, err := s.exportArchive()
			if err == nil {
				file.Close()
				os.Remove(file.Name())
			}
			done <- err
		}()
		time.Sleep(5 * time.Millisecond)
		start := time.Now()
		err := save(i+100, false)
		backupSaveSamples = append(backupSaveSamples, float64(time.Since(start).Microseconds())/1000)
		backupErr := <-done
		if err != nil {
			t.Fatal(err)
		}
		if backupErr != nil {
			t.Fatal(backupErr)
		}
	}
	sort.Float64s(backupSaveSamples)
	results["saveDuringBackup"] = map[string]any{"p50Ms": backupSaveSamples[9], "p95Ms": backupSaveSamples[18], "maxMs": backupSaveSamples[19]}

	encoded, _ := json.MarshalIndent(results, "", "  ")
	dir := filepath.Join("..", "dist-test", "audit-20261003")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "storage-latency.json"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log(string(encoded))
}
