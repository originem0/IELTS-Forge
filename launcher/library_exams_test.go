package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sampleExamPack() libraryPack {
	pack := sampleLibraryPack()
	pack.Units = nil
	number := 1
	for index, count := range []int{13, 13, 14} {
		unit := sampleLibraryPack().Units[0]
		unit.ID = fmt.Sprintf("passage-%d", index+1)
		unit.Groups[0].Questions = nil
		for i := 0; i < count; i++ {
			unit.Groups[0].Questions = append(unit.Groups[0].Questions, libraryQuestion{ID: fmt.Sprintf("q%d", i+1), Label: fmt.Sprint(number), Text: "Opening day?", Answers: []string{"Monday"}, Evidence: "A"})
			number++
		}
		pack.Units = append(pack.Units, unit)
	}
	pack.Exams = []libraryExam{{ID: "full-reading", Title: "Synthetic complete reading", Skill: "reading", UnitIDs: []string{"passage-1", "passage-2", "passage-3"}, Minutes: 60}}
	return pack
}

func TestExamStructureAndNamespacedQuestions(t *testing.T) {
	pack := sampleExamPack()
	if err := validatePack(pack); err != nil {
		t.Fatal(err)
	}
	unit, err := unitForAttempt(pack, libraryAttempt{UnitID: "full-reading", ExamID: "full-reading"})
	if err != nil {
		t.Fatal(err)
	}
	if score := gradeLibraryUnit(unit, nil); score.Total != 40 {
		t.Fatal("lost questions", score.Total)
	}
	if unit.Groups[0].Questions[0].ID == unit.Groups[1].Questions[0].ID {
		t.Fatal("unit-local question ids collided")
	}
	if unit.Groups[1].Questions[0].Evidence != "s2-A" {
		t.Fatal("evidence did not retain passage namespace")
	}
	for name, edit := range map[string]func(*libraryPack){
		"partial":         func(p *libraryPack) { p.Exams[0].UnitIDs = p.Exams[0].UnitIDs[:2] },
		"repeated":        func(p *libraryPack) { p.Exams[0].UnitIDs[1] = p.Exams[0].UnitIDs[0] },
		"wrong time":      func(p *libraryPack) { p.Exams[0].Minutes = 20 },
		"missing answer":  func(p *libraryPack) { p.Units[1].Groups[0].Questions[0].Answers = nil },
		"local numbering": func(p *libraryPack) { p.Units[1].Groups[0].Questions[0].Label = "1" },
		"unverified":      func(p *libraryPack) { p.Source.Status = "unverified" },
	} {
		t.Run(name, func(t *testing.T) {
			p := sampleExamPack()
			edit(&p)
			if err := validatePack(p); err == nil {
				t.Fatal("bad exam accepted")
			}
		})
	}
}

func TestMockDeadlineCannotResetAndLateAnswersDoNotReplaceSavedOnes(t *testing.T) {
	s := libraryTestStore(t)
	id, err := s.importPack(sampleExamPack())
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.saveAttempt(libraryAttempt{ID: "mock", PackID: id, UnitID: "full-reading", ExamID: "full-reading", Mode: "simulation", Status: "draft", Answers: map[string][]string{"s1-q1": {"Monday"}}})
	if err != nil {
		t.Fatal(err)
	}
	deadline := a.DeadlineAt
	start, _ := time.Parse(time.RFC3339Nano, a.StartedAt)
	a.StartedAt = start.Add(time.Hour).Format(time.RFC3339Nano)
	a.DeadlineAt = start.Add(2 * time.Hour).Format(time.RFC3339Nano)
	a, err = s.saveAttempt(a)
	if err != nil {
		t.Fatal(err)
	}
	if a.DeadlineAt != deadline {
		t.Fatal("client reset deadline")
	}
	stored := a
	stored.StartedAt = time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339Nano)
	anchor, _ := time.Parse(time.RFC3339Nano, stored.StartedAt)
	stored.DeadlineAt = anchor.Add(time.Hour).Format(time.RFC3339Nano)
	raw, _ := json.Marshal(stored)
	if err := os.WriteFile(filepath.Join(s.directory, "library", "attempts", "mock.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	a.Answers = map[string][]string{"s1-q1": {"Tuesday"}}
	expired, err := s.saveAttempt(a)
	if err != nil {
		t.Fatal(err)
	}
	if expired.Status != "submitted" || expired.Answers["s1-q1"][0] != "Monday" || expired.ElapsedSeconds != 3600 {
		t.Fatalf("late answers altered expired mock: %+v", expired)
	}
	if _, err := s.saveAttempt(expired); err == nil {
		t.Fatal("expired mock reopened")
	}
	backupBytes(t, s)
}
