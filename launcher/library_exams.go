package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type libraryExam struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Skill   string   `json:"skill"`
	UnitIDs []string `json:"unitIds"`
	Minutes int      `json:"minutes"`
}

type libraryTrack struct {
	Audio string `json:"audio"`
	Title string `json:"title"`
	Part  string `json:"part"`
}

func validateLibraryExams(pack libraryPack) error {
	units := map[string]libraryUnit{}
	for _, unit := range pack.Units {
		units[unit.ID] = unit
		if len(unit.Tracks) > 0 {
			return errors.New("录音序列必须由试卷关联生成")
		}
	}
	seen := map[string]bool{}
	for _, exam := range pack.Exams {
		if pack.Source.Status == "unverified" {
			return errors.New("未核验来源不能声明整套试卷；生成材料须明确标为 generated")
		}
		if !libraryID.MatchString(exam.ID) || seen[exam.ID] || strings.TrimSpace(exam.Title) == "" {
			return errors.New("试卷编号或标题无效")
		}
		seen[exam.ID] = true
		if _, exists := units[exam.ID]; exists {
			return errors.New("试卷和单篇题目不能使用同一编号")
		}
		if (exam.Skill == "reading" && (len(exam.UnitIDs) != 3 || exam.Minutes != 60)) || (exam.Skill == "listening" && (len(exam.UnitIDs) != 4 || exam.Minutes < 20 || exam.Minutes > 60)) || (exam.Skill != "reading" && exam.Skill != "listening") {
			return errors.New("阅读模拟须三篇、60 分钟；听力须四段、20–60 分钟")
		}
		used := map[string]bool{}
		audios := map[string]bool{}
		next := 1
		part := ""
		for index, id := range exam.UnitIDs {
			unit, ok := units[id]
			if !ok || used[id] || unit.Skill != exam.Skill {
				return errors.New("试卷题目缺失、重复或科目不符")
			}
			used[id] = true
			if exam.Skill == "reading" {
				if part != "" && part != unit.Part {
					return errors.New("不能混合学术类和培训类阅读")
				}
				part = unit.Part
			}
			if exam.Skill == "listening" {
				if unit.Part != fmt.Sprint(index+1) || audios[unit.Audio] {
					return errors.New("听力必须按 Section 1–4 排列，且不能重复录音")
				}
				audios[unit.Audio] = true
			}
			for _, group := range unit.Groups {
				for _, q := range group.Questions {
					points := 1
					if group.Kind == "multiple" {
						points = len(q.Answers)
					}
					expected := fmt.Sprint(next)
					if points > 1 {
						expected = fmt.Sprintf("%d-%d", next, next+points-1)
					}
					label := strings.NewReplacer("–", "-", "—", "-", " ", "").Replace(q.Label)
					if label != expected {
						return errors.New("整套试卷题号必须连续覆盖 1–40，不能重复单篇题号")
					}
					next += points
				}
			}
		}
		if next != 41 {
			return errors.New("整套听读试卷必须有 40 个计分点")
		}
	}
	return nil
}

func unitForAttempt(pack libraryPack, attempt libraryAttempt) (libraryUnit, error) {
	if attempt.ExamID == "" {
		for _, unit := range pack.Units {
			if unit.ID == attempt.UnitID {
				return unit, nil
			}
		}
		return libraryUnit{}, errors.New("题目不存在")
	}
	for _, exam := range pack.Exams {
		if exam.ID != attempt.ExamID || attempt.UnitID != exam.ID {
			continue
		}
		combined := libraryUnit{ID: exam.ID, Skill: exam.Skill, Title: exam.Title, Part: "full", Minutes: exam.Minutes, Prompt: "完成整套试卷。计时持续进行，听力按原速顺序播放一次；中断后可恢复已保存进度。"}
		for index, id := range exam.UnitIDs {
			for _, unit := range pack.Units {
				if unit.ID != id {
					continue
				}
				prefix := fmt.Sprintf("s%d-", index+1)
				if exam.Skill == "reading" {
					combined.Part = unit.Part
				}
				for _, paragraph := range unit.Passages {
					paragraph.Label = fmt.Sprintf("%d · %s · %s", index+1, unit.Title, paragraph.ID)
					paragraph.ID = prefix + paragraph.ID
					combined.Passages = append(combined.Passages, paragraph)
				}
				for _, group := range unit.Groups {
					group.ID = prefix + group.ID
					group.Instruction = fmt.Sprintf("%s %d · %s\n%s", map[string]string{"reading": "Passage", "listening": "Section"}[exam.Skill], index+1, unit.Title, group.Instruction)
					questions := []libraryQuestion{}
					for _, q := range group.Questions {
						q.ID = prefix + q.ID
						if q.Evidence != "" {
							q.Evidence = prefix + q.Evidence
						}
						questions = append(questions, q)
					}
					group.Questions = questions
					combined.Groups = append(combined.Groups, group)
				}
				combined.Images = append(combined.Images, unit.Images...)
				if unit.Audio != "" {
					combined.Tracks = append(combined.Tracks, libraryTrack{Audio: unit.Audio, Title: unit.Title, Part: unit.Part})
					if combined.Audio == "" {
						combined.Audio = unit.Audio
					}
					combined.Transcript += fmt.Sprintf("Section %s · %s\n%s\n\n", unit.Part, unit.Title, unit.Transcript)
				}
			}
		}
		return combined, nil
	}
	return libraryUnit{}, errors.New("试卷不存在")
}

func (s *diskStore) expireAttemptLocked(attempt libraryAttempt) (libraryAttempt, bool, error) {
	if attempt.Mode != "simulation" || attempt.Status != "draft" {
		return attempt, false, nil
	}
	deadline, err := time.Parse(time.RFC3339Nano, attempt.DeadlineAt)
	if err != nil {
		return attempt, false, errors.New("模拟截止时间无效")
	}
	// Allow a brief local write grace so the last debounce at zero can finish.
	if time.Now().Before(deadline.Add(2 * time.Second)) {
		return attempt, false, nil
	}
	path, err := s.libraryPathLocked("attempts", attempt.ID+".json")
	if err != nil {
		return attempt, false, err
	}
	backup, err := s.libraryPathLocked("attempts", attempt.ID+".backup.json")
	if err != nil {
		return attempt, false, err
	}
	previous, _ := json.Marshal(attempt)
	if err := atomicLibraryWrite(backup, previous); err != nil {
		return attempt, false, err
	}
	start, err := time.Parse(time.RFC3339Nano, attempt.StartedAt)
	if err != nil {
		return attempt, false, err
	}
	attempt.Status = "submitted"
	attempt.Revision++
	attempt.ElapsedSeconds = int(deadline.Sub(start).Seconds())
	attempt.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	raw, _ := json.Marshal(attempt)
	if err := atomicLibraryWrite(path, raw); err != nil {
		return attempt, false, err
	}
	return attempt, true, nil
}

func registerLibraryExamAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/library/attempts/{id}/question", func(w http.ResponseWriter, r *http.Request) {
		disk.Lock()
		defer disk.Unlock()
		attempt, err := disk.loadAttemptLocked(r.PathValue("id"))
		if err != nil {
			writeError(w, 404, "练习不存在")
			return
		}
		pack, err := disk.loadPackLocked(attempt.PackID)
		if err != nil {
			writeError(w, 404, "题包不存在")
			return
		}
		unit, err := unitForAttempt(pack, attempt)
		if err != nil {
			writeError(w, 404, err.Error())
			return
		}
		writeJSON(w, 200, unit)
	})
}
