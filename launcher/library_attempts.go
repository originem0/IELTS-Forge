package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"
	"unicode/utf16"
)

var errAttemptConflict = errors.New("练习已在其他页面更新，请重新载入后继续")

type libraryAttempt struct {
	ID              string              `json:"id"`
	Revision        int                 `json:"revision"`
	PackID          string              `json:"packId"`
	UnitID          string              `json:"unitId"`
	ExamID          string              `json:"examId,omitempty"`
	StartedAt       string              `json:"startedAt,omitempty"`
	DeadlineAt      string              `json:"deadlineAt,omitempty"`
	Mode            string              `json:"mode"`
	Status          string              `json:"status"`
	ReviewOf        string              `json:"reviewOf,omitempty"`
	ReviewQuestions []string            `json:"reviewQuestions,omitempty"`
	Highlights      []libraryHighlight  `json:"highlights,omitempty"`
	Answers         map[string][]string `json:"answers"`
	Marked          []string            `json:"marked,omitempty"`
	Notes           map[string]string   `json:"notes,omitempty"`
	ElapsedSeconds  int                 `json:"elapsedSeconds"`
	AudioSeconds    float64             `json:"audioSeconds,omitempty"`
	AudioRate       float64             `json:"audioRate,omitempty"`
	AudioTrack      int                 `json:"audioTrack,omitempty"`
	AudioEnded      bool                `json:"audioEnded,omitempty"`
	ActivityDates   []string            `json:"activityDates,omitempty"`
	CreatedAt       string              `json:"createdAt"`
	UpdatedAt       string              `json:"updatedAt"`
}

type libraryHighlight struct {
	ParagraphID string `json:"paragraphId"`
	Start       int    `json:"start"`
	End         int    `json:"end"`
}

func safeAttemptID(id string) bool {
	if !libraryID.MatchString(id) {
		return false
	}
	name := strings.ToUpper(id)
	if name == "CON" || name == "PRN" || name == "AUX" || name == "NUL" {
		return false
	}
	if len(name) == 4 && (strings.HasPrefix(name, "COM") || strings.HasPrefix(name, "LPT")) && name[3] >= '1' && name[3] <= '9' {
		return false
	}
	return true
}

func (s *diskStore) loadAttemptLocked(id string) (libraryAttempt, error) {
	var attempt libraryAttempt
	if !safeAttemptID(id) {
		return attempt, errors.New("练习编号无效")
	}
	path, err := s.libraryPathLocked("attempts", id+".json")
	if err != nil {
		return attempt, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return attempt, err
	}
	if err := json.Unmarshal(raw, &attempt); err != nil {
		return attempt, err
	}
	if attempt.ID != id || attempt.Revision < 1 {
		return attempt, errors.New("练习数据损坏")
	}
	return attempt, nil
}

func (s *diskStore) validateAttemptLocked(attempt libraryAttempt) error {
	if len(attempt.ActivityDates) > 10000 {
		return errors.New("学习日期记录过多")
	}
	for _, date := range attempt.ActivityDates {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return errors.New("学习日期无效")
		}
	}
	if !safeAttemptID(attempt.ID) || attempt.Revision < 0 || attempt.ElapsedSeconds < 0 || attempt.ElapsedSeconds > 7*24*3600 || attempt.AudioSeconds < 0 || attempt.AudioSeconds > 7*24*3600 || (attempt.AudioRate != 0 && (attempt.AudioRate < .5 || attempt.AudioRate > 2)) {
		return errors.New("练习编号、版本或时间无效")
	}
	if attempt.Mode != "practice" && attempt.Mode != "simulation" {
		return errors.New("练习模式无效")
	}
	if attempt.Status != "draft" && attempt.Status != "submitted" {
		return errors.New("练习状态无效")
	}
	pack, err := s.loadPackLocked(attempt.PackID)
	if err != nil {
		return err
	}
	resolved, resolveErr := unitForAttempt(pack, attempt)
	unit := &resolved
	if resolveErr != nil || (unit.Skill != "reading" && unit.Skill != "listening") {
		return errors.New("听读练习题目不存在")
	}
	if attempt.Mode == "simulation" && attempt.ExamID == "" {
		return errors.New("模拟必须引用完整试卷")
	}
	if attempt.AudioTrack < 0 || (len(unit.Tracks) > 0 && attempt.AudioTrack >= len(unit.Tracks)) {
		return errors.New("音频段落编号无效")
	}
	if attempt.Mode == "simulation" && attempt.AudioRate != 0 && attempt.AudioRate != 1 {
		return errors.New("模拟使用原速录音")
	}
	if attempt.Mode == "simulation" && attempt.Revision > 0 {
		start, startErr := time.Parse(time.RFC3339Nano, attempt.StartedAt)
		deadline, deadlineErr := time.Parse(time.RFC3339Nano, attempt.DeadlineAt)
		if startErr != nil || deadlineErr != nil || deadline.Sub(start) != time.Duration(unit.Minutes)*time.Minute {
			return errors.New("模拟计时记录无效")
		}
	}
	questions := map[string]libraryGroup{}
	questionDetails := map[string]libraryQuestion{}
	for _, group := range unit.Groups {
		for _, q := range group.Questions {
			questions[q.ID] = group
			questionDetails[q.ID] = q
		}
	}
	for id, answers := range attempt.Answers {
		group, ok := questions[id]
		if !ok {
			return errors.New("答案引用未知题目")
		}
		if len(answers) > 100 || (group.Kind != "multiple" && len(answers) > 1) {
			return errors.New("答案数量无效")
		}
		if group.Kind == "multiple" && len(answers) > len(questionDetails[id].Answers) {
			return errors.New("多选答案超过题目要求数量")
		}
		seen := map[string]bool{}
		for _, value := range answers {
			if len(value) > 2000 || seen[value] {
				return errors.New("答案过长或重复")
			}
			seen[value] = true
			if group.Kind != "text" && value != "" {
				found := false
				options := group.Options
				if len(questionDetails[id].Options) > 0 {
					options = questionDetails[id].Options
				}
				for _, opt := range options {
					if opt.ID == value {
						found = true
					}
				}
				if !found {
					return errors.New("答案引用未知选项")
				}
			}
		}
	}
	for _, id := range attempt.Marked {
		if _, ok := questions[id]; !ok {
			return errors.New("标记引用未知题目")
		}
	}
	paragraphs := map[string]bool{}
	paragraphLengths := map[string]int{}
	for _, p := range unit.Passages {
		paragraphs[p.ID] = true
		paragraphLengths[p.ID] = len(utf16.Encode([]rune(p.Text)))
	}
	for id, note := range attempt.Notes {
		if !paragraphs[id] || len(note) > 10000 {
			return errors.New("笔记段落或长度无效")
		}
	}
	if len(attempt.Highlights) > 1000 {
		return errors.New("划线数量过多")
	}
	for i, h := range attempt.Highlights {
		if !paragraphs[h.ParagraphID] || h.Start < 0 || h.End <= h.Start || h.End > paragraphLengths[h.ParagraphID] {
			return errors.New("原文划线位置无效")
		}
		for _, other := range attempt.Highlights[:i] {
			if h.ParagraphID == other.ParagraphID && h.Start < other.End && other.Start < h.End {
				return errors.New("原文划线重叠")
			}
		}
	}
	if len(attempt.ReviewQuestions) > 0 && attempt.ReviewOf == "" {
		return errors.New("错题重练必须有原练习")
	}
	if attempt.ReviewOf != "" {
		original, err := s.loadAttemptLocked(attempt.ReviewOf)
		if err != nil || original.PackID != attempt.PackID || original.UnitID != attempt.UnitID || original.ExamID != attempt.ExamID || original.Status != "submitted" || original.ID == attempt.ID {
			return errors.New("复习来源无效")
		}
		wrong := map[string]bool{}
		for _, item := range gradeLibraryUnit(scopedLibraryUnit(*unit, original), original.Answers).Items {
			if !item.Correct {
				wrong[item.ID] = true
			}
		}
		seen := map[string]bool{}
		for _, id := range attempt.ReviewQuestions {
			if !wrong[id] || seen[id] {
				return errors.New("错题重练范围无效")
			}
			seen[id] = true
		}
		if len(seen) > 0 {
			for id := range attempt.Answers {
				if !seen[id] {
					return errors.New("答案超出本次重练范围")
				}
			}
		}
	}
	return nil
}

func (s *diskStore) saveAttempt(attempt libraryAttempt, directoryID ...string) (libraryAttempt, error) {
	s.Lock()
	defer s.Unlock()
	if err := s.checkDirectoryLocked(directoryID...); err != nil {
		return attempt, err
	}
	if err := s.validateAttemptLocked(attempt); err != nil {
		return attempt, err
	}
	previous, err := s.loadAttemptLocked(attempt.ID)
	if err == nil {
		if previous, expired, expireErr := s.expireAttemptLocked(previous); expireErr != nil {
			return attempt, expireErr
		} else if expired {
			return previous, nil
		}
		if previous.Revision != attempt.Revision {
			return attempt, errAttemptConflict
		}
		if previous.Status == "submitted" {
			return attempt, errors.New("已提交的练习不可覆盖，请创建重练记录")
		}
		if previous.PackID != attempt.PackID || previous.UnitID != attempt.UnitID || previous.ExamID != attempt.ExamID || previous.Mode != attempt.Mode || previous.ReviewOf != attempt.ReviewOf || !slices.Equal(previous.ReviewQuestions, attempt.ReviewQuestions) {
			return attempt, errors.New("不能更换已有练习的题目或模式")
		}
		attempt.CreatedAt = previous.CreatedAt
		attempt.ActivityDates = append(attempt.ActivityDates, previous.ActivityDates...)
		attempt.StartedAt, attempt.DeadlineAt = previous.StartedAt, previous.DeadlineAt
		if attempt.Mode == "simulation" && (attempt.AudioTrack < previous.AudioTrack || attempt.AudioTrack > previous.AudioTrack+1 || (previous.AudioEnded && !attempt.AudioEnded)) {
			return attempt, errors.New("模拟录音不能倒回或跳段")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return attempt, err
	} else if attempt.Revision != 0 {
		return attempt, errAttemptConflict
	} else {
		attempt.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		attempt.StartedAt, attempt.DeadlineAt = "", ""
		if attempt.Mode == "simulation" {
			pack, err := s.loadPackLocked(attempt.PackID)
			if err != nil {
				return attempt, err
			}
			for _, exam := range pack.Exams {
				if exam.ID == attempt.ExamID {
					attempt.StartedAt = attempt.CreatedAt
					anchor, _ := time.Parse(time.RFC3339Nano, attempt.StartedAt)
					attempt.DeadlineAt = anchor.Add(time.Duration(exam.Minutes) * time.Minute).Format(time.RFC3339Nano)
				}
			}
			attempt.AudioTrack, attempt.AudioSeconds, attempt.AudioEnded = 0, 0, false
		}
	}
	if attempt.Mode == "simulation" {
		start, _ := time.Parse(time.RFC3339Nano, attempt.StartedAt)
		deadline, _ := time.Parse(time.RFC3339Nano, attempt.DeadlineAt)
		attempt.ElapsedSeconds = int(time.Since(start).Seconds())
		if attempt.ElapsedSeconds < 0 {
			attempt.ElapsedSeconds = 0
		}
		if maximum := int(deadline.Sub(start).Seconds()); attempt.ElapsedSeconds > maximum {
			attempt.ElapsedSeconds = maximum
		}
	}
	dates := map[string]bool{}
	uniqueDates := []string{}
	for _, date := range append(attempt.ActivityDates, time.Now().Format("2006-01-02")) {
		if !dates[date] {
			dates[date] = true
			uniqueDates = append(uniqueDates, date)
		}
	}
	attempt.ActivityDates = uniqueDates
	attempt.Revision++
	attempt.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	path, err := s.libraryPathLocked("attempts", attempt.ID+".json")
	if err != nil {
		return attempt, err
	}
	if previous.Revision > 0 {
		backupPath, err := s.libraryPathLocked("attempts", attempt.ID+".backup.json")
		if err != nil {
			return attempt, err
		}
		raw, _ := json.Marshal(previous)
		if err := atomicLibraryWrite(backupPath, raw); err != nil {
			return attempt, err
		}
	}
	raw, err := json.Marshal(attempt)
	if err != nil {
		return attempt, err
	}
	return attempt, atomicLibraryWrite(path, raw)
}

func registerLibraryAttemptsAPI(mux *http.ServeMux) {
	mux.HandleFunc("PUT /api/library/attempts/{id}", func(w http.ResponseWriter, r *http.Request) {
		var attempt libraryAttempt
		if err := decodeLibraryJSON(w, r, 1<<20, &attempt); err != nil || attempt.ID != r.PathValue("id") {
			writeError(w, 400, "练习格式无效")
			return
		}
		saved, err := disk.saveAttempt(attempt, r.Header.Get("X-ELP-Directory"))
		if err != nil {
			status := 400
			if errors.Is(err, errAttemptConflict) || errors.Is(err, errDirectoryChanged) {
				status = 409
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, 200, saved)
	})
	mux.HandleFunc("GET /api/library/attempts/{id}", func(w http.ResponseWriter, r *http.Request) {
		disk.Lock()
		defer disk.Unlock()
		attempt, err := disk.loadAttemptLocked(r.PathValue("id"))
		if err != nil {
			writeError(w, 404, "练习不存在或已损坏")
			return
		}
		attempt, _, err = disk.expireAttemptLocked(attempt)
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, attempt)
	})
	mux.HandleFunc("GET /api/library/attempts", func(w http.ResponseWriter, r *http.Request) {
		disk.Lock()
		defer disk.Unlock()
		path, err := disk.libraryPathLocked("attempts", "")
		if err != nil {
			writeError(w, 409, err.Error())
			return
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			writeError(w, 500, "无法读取练习记录")
			return
		}
		items := []libraryAttempt{}
		for _, entry := range entries {
			id := strings.TrimSuffix(entry.Name(), ".json")
			if !libraryID.MatchString(id) || entry.IsDir() {
				continue
			}
			attempt, err := disk.loadAttemptLocked(id)
			if err != nil {
				writeError(w, 500, "练习记录损坏，请从备份恢复")
				return
			}
			attempt, _, err = disk.expireAttemptLocked(attempt)
			if err != nil {
				writeError(w, 500, err.Error())
				return
			}
			items = append(items, attempt)
		}
		writeJSON(w, 200, map[string]any{"attempts": items})
	})
}
