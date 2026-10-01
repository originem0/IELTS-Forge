package main

import (
	"net/http"
	"regexp"
	"strings"
)

type libraryScoreItem struct {
	ID      string `json:"id"`
	Correct bool   `json:"correct"`
	Points  int    `json:"points"`
	Total   int    `json:"total"`
	Reason  string `json:"reason"`
}

type libraryScore struct {
	Points int                `json:"points"`
	Total  int                `json:"total"`
	Items  []libraryScoreItem `json:"items"`
}

var answerNumber = regexp.MustCompile(`^[£$€]?[0-9]+(?:[.,:/-][0-9]+)*(?:st|nd|rd|th|%)?$`)

func normalizedAnswer(value string) string {
	value = strings.NewReplacer("’", "'", "‘", "'", "‐", "-", "‑", "-").Replace(value)
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func withinAnswerLimit(value string, group libraryGroup) bool {
	words, numbers := 0, 0
	for _, token := range strings.Fields(value) {
		if answerNumber.MatchString(token) {
			numbers++
		} else {
			words++
		}
	}
	if group.NumberOnly {
		return words == 0 && numbers == 1
	}
	if group.MaxWords == 0 {
		return true
	}
	if group.AllowNumber {
		return words <= group.MaxWords && numbers <= 1
	}
	return words+numbers <= group.MaxWords
}

// Objective marking is local and deterministic. Never use semantic similarity:
// plural/spelling changes are wrong unless explicitly listed by the source key.
func gradeLibraryUnit(unit libraryUnit, answers map[string][]string) libraryScore {
	score := libraryScore{Items: []libraryScoreItem{}}
	for _, group := range unit.Groups {
		for _, q := range group.Questions {
			item := libraryScoreItem{ID: q.ID, Total: 1, Reason: "incorrect"}
			given := answers[q.ID]
			if group.Kind == "multiple" {
				item.Total = len(q.Answers)
				seen := map[string]bool{}
				if len(given) <= item.Total {
					for _, selected := range given {
						if seen[selected] {
							continue
						}
						seen[selected] = true
						for _, expected := range q.Answers {
							if selected == expected {
								item.Points++
								break
							}
						}
					}
				}
			} else if len(given) == 1 && strings.TrimSpace(given[0]) != "" {
				if group.Kind == "text" && !withinAnswerLimit(normalizedAnswer(given[0]), group) {
					item.Reason = "word-limit"
				} else {
					for _, expected := range q.Answers {
						if normalizedAnswer(given[0]) == normalizedAnswer(expected) {
							item.Points = 1
							break
						}
					}
				}
			}
			if len(given) == 0 || (len(given) == 1 && strings.TrimSpace(given[0]) == "") {
				item.Reason = "unanswered"
			}
			item.Correct = item.Points == item.Total
			if item.Correct {
				item.Reason = "correct"
			}
			score.Total += item.Total
			score.Points += item.Points
			score.Items = append(score.Items, item)
		}
	}
	return score
}

func registerLibraryResultsAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/library/attempts/{id}/result", func(w http.ResponseWriter, r *http.Request) {
		disk.Lock()
		defer disk.Unlock()
		attempt, err := disk.loadAttemptLocked(r.PathValue("id"))
		if err != nil {
			writeError(w, 404, "练习不存在")
			return
		}
		if attempt.Status != "submitted" {
			var expireErr error
			attempt, _, expireErr = disk.expireAttemptLocked(attempt)
			if expireErr != nil {
				writeError(w, 500, expireErr.Error())
				return
			}
		}
		if attempt.Status != "submitted" {
			writeError(w, 409, "提交后才能查看结果")
			return
		}
		pack, err := disk.loadPackLocked(attempt.PackID)
		if err != nil {
			writeError(w, 500, "原题版本不存在或已损坏")
			return
		}
		unit, err := unitForAttempt(pack, attempt)
		if err != nil {
			writeError(w, 404, "原题不存在")
			return
		}
		writeJSON(w, 200, gradeLibraryUnit(scopedLibraryUnit(unit, attempt), attempt.Answers))
	})
}

func scopedLibraryUnit(unit libraryUnit, attempt libraryAttempt) libraryUnit {
	if len(attempt.ReviewQuestions) == 0 {
		return unit
	}
	selected := map[string]bool{}
	for _, id := range attempt.ReviewQuestions {
		selected[id] = true
	}
	groups := []libraryGroup{}
	for _, group := range unit.Groups {
		questions := []libraryQuestion{}
		for _, q := range group.Questions {
			if selected[q.ID] {
				questions = append(questions, q)
			}
		}
		if len(questions) > 0 {
			group.Questions = questions
			groups = append(groups, group)
		}
	}
	unit.Groups = groups
	return unit
}
