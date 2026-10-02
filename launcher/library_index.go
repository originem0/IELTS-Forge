package main

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type fileStamp struct {
	Size     int64
	Modified int64
}

func stamp(info os.FileInfo) fileStamp { return fileStamp{info.Size(), info.ModTime().UnixNano()} }

type packIndexEntry struct {
	Stamp fileStamp
	View  map[string]any
	Units map[string]string
	Exams map[string]string
}
type attemptIndexEntry struct {
	Stamp    fileStamp
	PackID   string
	Counts   map[string]int
	Dates    []string
	Deadline time.Time
}

// Derived metadata only. Content remains authoritative on disk and a restart
// rebuilds the index. Expensive reads never hold the primary archive lock.
type libraryReadIndex struct {
	sync.Mutex
	directory               string
	packs                   map[string]packIndexEntry
	attempts                map[string]attemptIndexEntry
	counts                  map[string]int
	dates                   map[string]int
	packReads, attemptReads int
}

func (s *diskStore) readIndex(expected string) (*libraryReadIndex, error) {
	s.Lock()
	defer s.Unlock()
	if err := s.checkDirectoryLocked(expected); expected != "" && err != nil {
		return nil, err
	}
	if s.directory == "" {
		return nil, errors.New("请先绑定永久数据目录")
	}
	if s.libraryIndex == nil || s.libraryIndex.directory != s.directory {
		s.libraryIndex = &libraryReadIndex{directory: s.directory, packs: map[string]packIndexEntry{}, attempts: map[string]attemptIndexEntry{}, counts: map[string]int{"reading": 0, "listening": 0, "simulations": 0, "completed": 0}, dates: map[string]int{}}
	}
	return s.libraryIndex, nil
}

func (index *libraryReadIndex) scanPacks() ([]any, error) {
	reader := &diskStore{directory: index.directory}
	path, err := reader.libraryPathLocked("packs", "")
	if err != nil {
		return nil, err
	}
	files, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	items := []any{}
	for _, file := range files {
		id := strings.TrimSuffix(file.Name(), ".json")
		if file.IsDir() || !contentID.MatchString(id) {
			continue
		}
		info, err := file.Info()
		if err != nil {
			return nil, err
		}
		entry, ok := index.packs[id]
		if !ok || entry.Stamp != stamp(info) {
			pack, err := reader.loadPackLocked(id)
			if err != nil {
				return nil, errors.New("题库存在损坏文件，请从备份恢复")
			}
			index.packReads++
			entry = packIndexEntry{Stamp: stamp(info), Units: map[string]string{}, Exams: map[string]string{}}
			skills := map[string]int{}
			unitIDs := make([]string, 0, len(pack.Units))
			visibleIDs := make([]string, 0, len(pack.Units))
			for _, unit := range pack.Units {
				unitIDs = append(unitIDs, unit.ID)
				if !unit.Hidden {
					visibleIDs = append(visibleIDs, unit.ID)
				}
				skills[unit.Skill]++
				entry.Units[unit.ID] = unit.Skill
			}
			for _, exam := range pack.Exams {
				entry.Exams[exam.ID] = exam.Skill
			}
			entry.View = map[string]any{"id": id, "title": pack.Title, "source": pack.Source, "count": len(pack.Units), "unitIds": unitIDs, "visibleUnitIds": visibleIDs, "skills": skills, "importedAt": info.ModTime().UTC().Format(time.RFC3339Nano)}
			index.packs[id] = entry
		}
		seen[id] = true
		items = append(items, entry.View)
	}
	for id := range index.packs {
		if !seen[id] {
			delete(index.packs, id)
		}
	}
	return items, nil
}

func (index *libraryReadIndex) adjust(entry attemptIndexEntry, sign int) {
	for key, value := range entry.Counts {
		index.counts[key] += sign * value
	}
	for _, date := range entry.Dates {
		index.dates[date] += sign
		if index.dates[date] == 0 {
			delete(index.dates, date)
		}
	}
}

func (s *diskStore) indexedSummary(expected string) (map[string]any, error) {
	index, err := s.readIndex(expected)
	if err != nil {
		return nil, err
	}
	index.Lock()
	defer index.Unlock()
	if _, err := index.scanPacks(); err != nil {
		return nil, err
	}
	reader := &diskStore{directory: index.directory}
	path, err := reader.libraryPathLocked("attempts", "")
	if err != nil {
		return nil, err
	}
	files, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, file := range files {
		id := strings.TrimSuffix(file.Name(), ".json")
		if file.IsDir() || !safeAttemptID(id) {
			continue
		}
		info, err := file.Info()
		if err != nil {
			return nil, err
		}
		old, ok := index.attempts[id]
		expired := ok && !old.Deadline.IsZero() && !time.Now().Before(old.Deadline.Add(2*time.Second))
		if !ok || old.Stamp != stamp(info) || expired {
			attempt, err := reader.loadAttemptLocked(id)
			if err != nil {
				return nil, errors.New("统计包含损坏记录")
			}
			index.attemptReads++
			deadline, _ := time.Parse(time.RFC3339Nano, attempt.DeadlineAt)
			if attempt.Mode == "simulation" && attempt.Status == "draft" && !time.Now().Before(deadline.Add(2*time.Second)) {
				// Reload under the write lock; another tab may have saved newer answers
				// since the metadata scan. Only this small expiry commit blocks writers.
				s.Lock()
				if s.directory != index.directory {
					s.Unlock()
					return nil, errDirectoryChanged
				}
				attempt, err = s.loadAttemptLocked(id)
				if err == nil {
					attempt, _, err = s.expireAttemptLocked(attempt)
				}
				s.Unlock()
				if err != nil {
					return nil, err
				}
				filename, _ := reader.libraryPathLocked("attempts", id+".json")
				info, err = os.Stat(filename)
				if err != nil {
					return nil, err
				}
			}
			pack, exists := index.packs[attempt.PackID]
			if !exists {
				return nil, errors.New("统计缺少原题版本")
			}
			skill := pack.Units[attempt.UnitID]
			if attempt.ExamID != "" {
				skill = pack.Exams[attempt.ExamID]
			}
			if skill == "" {
				return nil, errors.New("统计记录缺少原题")
			}
			entry := attemptIndexEntry{Stamp: stamp(info), PackID: attempt.PackID, Counts: map[string]int{skill: 1}}
			if attempt.Mode == "simulation" {
				entry.Counts["simulations"] = 1
				if attempt.Status == "draft" {
					entry.Deadline = deadline
				}
			}
			if attempt.Status == "submitted" {
				entry.Counts["completed"] = 1
			}
			dates := map[string]bool{}
			for _, date := range attempt.ActivityDates {
				dates[date] = true
			}
			if len(dates) == 0 {
				for _, date := range []string{attempt.CreatedAt, attempt.UpdatedAt} {
					if parsed, err := time.Parse(time.RFC3339Nano, date); err == nil {
						dates[parsed.Local().Format("2006-01-02")] = true
					}
				}
			}
			for date := range dates {
				entry.Dates = append(entry.Dates, date)
			}
			if ok {
				index.adjust(old, -1)
			}
			index.adjust(entry, 1)
			index.attempts[id] = entry
		} else if _, exists := index.packs[old.PackID]; !exists {
			return nil, errors.New("统计缺少原题版本")
		}
		seen[id] = true
	}
	for id, old := range index.attempts {
		if !seen[id] {
			index.adjust(old, -1)
			delete(index.attempts, id)
		}
	}
	counts := map[string]int{}
	for key, value := range index.counts {
		counts[key] = value
	}
	dates := []string{}
	for date := range index.dates {
		dates = append(dates, date)
	}
	return map[string]any{"counts": counts, "activityDates": dates}, nil
}

func handleLibraryCatalogue(w http.ResponseWriter, r *http.Request) {
	index, err := disk.readIndex(r.Header.Get("X-ELP-Directory"))
	if err != nil {
		writeError(w, 409, err.Error())
		return
	}
	index.Lock()
	items, err := index.scanPacks()
	index.Unlock()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"packs": items})
}
