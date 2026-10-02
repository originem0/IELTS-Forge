package main

import (
	"archive/zip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// Optional acceptance uses the delivered archive, never a developer's retired
// staging paths or copyrighted fixtures committed to the source repository.
func TestLocalQuestionBankArchive(t *testing.T) {
	archivePath := os.Getenv("ELP_QUESTION_BANK_ZIP")
	if archivePath == "" {
		t.Skip("set ELP_QUESTION_BANK_ZIP to validate a local archive")
	}
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	manifestFile, err := archive.Open("archive-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Packs, Attachments, AnswerPoints, Exams int
		Counts                                  map[string]int
	}
	err = json.NewDecoder(manifestFile).Decode(&manifest)
	manifestFile.Close()
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	preview := &bankImportPreview{Stage: t.TempDir(), Media: map[string]bool{}}
	if err = preview.readZIP(file, info.Size()); err != nil {
		t.Fatal(err)
	}
	store := libraryTestStore(t)
	if err = store.validateImport(preview, store.directoryIDLocked()); err != nil {
		t.Fatal(err)
	}
	counts, points, exams := map[string]int{}, 0, 0
	for _, item := range preview.Packs {
		exams += len(item.Pack.Exams)
		for _, unit := range item.Pack.Units {
			counts[unit.Skill]++
			if unit.Provenance == nil {
				t.Fatal("missing source metadata", unit.ID)
			}
			answers := map[string][]string{}
			for _, group := range unit.Groups {
				for _, question := range group.Questions {
					if group.Kind == "multiple" {
						answers[question.ID] = question.Answers
						continue
					}
					answers[question.ID] = question.Answers[:1]
					for _, answer := range question.Answers {
						one := group
						one.Questions = []libraryQuestion{question}
						result := gradeLibraryUnit(libraryUnit{Groups: []libraryGroup{one}}, map[string][]string{question.ID: {answer}})
						if result.Points != 1 {
							t.Errorf("%s/%s rejects source answer %q", unit.ID, question.ID, answer)
						}
					}
				}
			}
			result := gradeLibraryUnit(unit, answers)
			if result.Points != result.Total {
				t.Fatal("source key failed", unit.ID)
			}
			points += result.Total
		}
	}
	if !reflect.DeepEqual(counts, manifest.Counts) || len(preview.Packs) != manifest.Packs || len(preview.Media) != manifest.Attachments || points != manifest.AnswerPoints || exams != manifest.Exams {
		t.Fatal("archive differs from manifest", counts, points, exams)
	}
	t.Log(counts, points, "answer points", exams, "exams")
	if os.Getenv("ELP_BROWSER_TEST") != "1" {
		return
	}
	oldDisk, oldSettings := disk, settings
	disk, settings = store, &configStore{}
	disk.configPath = filepath.Join(t.TempDir(), "config.json")
	defer func() { disk, settings = oldDisk, oldSettings }()
	mux := http.NewServeMux()
	registerAPI(mux)
	appDir, _ := filepath.Abs("../app")
	mux.Handle("/", secureStaticServer(appDir))
	server := httptest.NewServer(securityHeaders(mux))
	defer server.Close()
	cmd := exec.Command("node", "../tests/library-archive-ui.cjs")
	cmd.Env = append(os.Environ(), "ELP_TEST_URL="+server.URL)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatal(err)
	}
}
