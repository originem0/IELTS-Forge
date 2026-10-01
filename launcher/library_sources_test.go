package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConvertedLocalQuestionPacks(t *testing.T) {
	root := os.Getenv("ELP_CONVERTED_PACKS")
	if root == "" {
		t.Skip("set ELP_CONVERTED_PACKS to validate private local conversions")
	}
	paths, err := filepath.Glob(filepath.Join(root, "*-starter.json"))
	if err != nil || len(paths) == 0 {
		t.Fatal("no converted packs", err)
	}
	store := libraryTestStore(t)
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var pack libraryPack
			if err := json.Unmarshal(raw, &pack); err != nil {
				t.Fatal(err)
			}
			if _, err := store.importPack(pack); err != nil {
				t.Fatal(err)
			}
			for _, unit := range pack.Units {
				if unit.Skill != "reading" {
					continue
				}
				answers := map[string][]string{}
				for _, group := range unit.Groups {
					for _, q := range group.Questions {
						if group.Kind == "multiple" {
							answers[q.ID] = q.Answers
						} else {
							answers[q.ID] = q.Answers[:1]
						}
					}
				}
				score := gradeLibraryUnit(unit, answers)
				if score.Points != score.Total {
					t.Fatalf("%s source key conflicts with its own answer rules: %+v", unit.ID, score)
				}
			}
			t.Logf("validated %d units", len(pack.Units))
		})
	}
}
