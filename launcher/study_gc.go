package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func studyMediaRefs(value any, refs map[string]bool) error {
	return studyMediaRefsAt(value, refs, false)
}

func studyMediaRefsAt(value any, refs map[string]bool, media bool) error {
	switch typed := value.(type) {
	case string:
		if !media {
			return nil
		}
		if strings.HasPrefix(typed, "/api/study/media/") {
			id := strings.TrimPrefix(typed, "/api/study/media/")
			if !studyMediaID.MatchString(id) {
				return errors.New("学习媒体引用无效")
			}
			refs[id] = true
		}
	case []any:
		for _, item := range typed {
			if err := studyMediaRefsAt(item, refs, media); err != nil {
				return err
			}
		}
	case map[string]any:
		for key, item := range typed {
			if err := studyMediaRefsAt(item, refs, key == "audio" || key == "images" || key == "promptImages"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *diskStore) validateStudyMedia(data json.RawMessage) error {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	refs := map[string]bool{}
	if err := studyMediaRefs(value, refs); err != nil {
		return err
	}
	for id := range refs {
		path, err := s.libraryPathLocked("study-media", id)
		if err != nil {
			return err
		}
		digest, err := hashFile(path, 32<<20)
		if err != nil {
			return err
		}
		if digest != strings.Split(id, ".")[0] {
			return errors.New("学习媒体校验失败")
		}
	}
	return nil
}

// Caller owns the archive lock. Every managed recovery point participates in
// reachability; no file is removed if any snapshot or referenced record is bad.
func (s *diskStore) collectStudyGarbageLocked() error {
	snapshots := []string{s.dataPathLocked(), s.backupPathLocked()}
	daily, err := filepath.Glob(filepath.Join(s.directory, "backups", "EnglishLearnPath-data-*.json"))
	if err != nil {
		return err
	}
	snapshots = append(snapshots, daily...)
	records, media := map[string]bool{}, map[string]bool{}
	collectMedia := func(raw []byte) error {
		if !bytes.Contains(raw, []byte("/api/study/media/")) {
			return nil
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		return studyMediaRefs(value, media)
	}
	for _, path := range snapshots {
		raw, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		var envelope diskDataEnvelope
		if json.Unmarshal(raw, &envelope) != nil || !json.Valid(envelope.Data) {
			return errors.New("备份损坏，跳过版本清理")
		}
		if envelope.Version == 2 {
			var manifest studyManifest
			if json.Unmarshal(envelope.Data, &manifest) != nil || manifest.Collections == nil {
				return errors.New("索引损坏，跳过版本清理")
			}
			for _, entries := range manifest.Collections {
				for _, ref := range entries {
					if !contentID.MatchString(ref.Hash) {
						return errors.New("索引引用损坏")
					}
					records[ref.Hash+".json"] = true
				}
			}
		}
		if err := collectMedia(envelope.Data); err != nil {
			return err
		}
	}
	for name := range records {
		path, err := s.libraryPathLocked("study-records", name)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if hashContent(raw) != strings.TrimSuffix(name, ".json") {
			return errors.New("练习记录损坏，跳过版本清理")
		}
		if err := collectMedia(raw); err != nil {
			return err
		}
	}
	for collection, keep := range map[string]map[string]bool{"study-records": records, "study-media": media} {
		dir, err := s.libraryPathLocked(collection, "")
		if err != nil {
			return err
		}
		files, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, file := range files {
			if file.IsDir() || keep[file.Name()] || backupFileLimit("library/"+collection+"/"+file.Name()) == 0 {
				continue
			}
			if err := os.Remove(filepath.Join(dir, file.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}
