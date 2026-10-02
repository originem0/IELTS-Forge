package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var errLibraryPackInUse = errors.New("该题包仍被练习记录引用；请隐藏题目，保留历史原题")

// Pack versions are immutable. Deletion is permitted only after checking current
// learning data and recoverable record versions under the same write lock.
func (s *diskStore) deleteLibraryPack(id, directoryID string) error {
	s.Lock()
	defer s.Unlock()
	if err := s.checkDirectoryLocked(directoryID); err != nil {
		return err
	}
	if _, err := s.loadPackLocked(id); err != nil {
		return err
	}
	hasRef := func(raw []byte) (bool, error) {
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return false, err
		}
		var visit func(any) bool
		visit = func(v any) bool {
			switch x := v.(type) {
			case map[string]any:
				for key, item := range x {
					if key == "packId" && item == id {
						return true
					}
					if visit(item) {
						return true
					}
				}
			case []any:
				for _, item := range x {
					if visit(item) {
						return true
					}
				}
			}
			return false
		}
		return visit(value), nil
	}
	data, err := s.loadLocked()
	if err != nil {
		return err
	}
	if used, err := hasRef(data); err != nil {
		return err
	} else if used {
		return errLibraryPackInUse
	}
	for _, collection := range []string{"attempts", "study-records"} {
		dir, err := s.libraryPathLocked(collection, "")
		if err != nil {
			return err
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				return err
			}
			if used, err := hasRef(raw); err != nil {
				return err
			} else if used {
				return errLibraryPackInUse
			}
		}
	}
	snapshots, err := filepath.Glob(filepath.Join(s.directory, "backups", "EnglishLearnPath-data-*.json"))
	if err != nil {
		return err
	}
	for _, snapshot := range append(snapshots, s.backupPathLocked()) {
		if raw, err := os.ReadFile(snapshot); err == nil {
			if used, err := hasRef(raw); err != nil {
				return err
			} else if used {
				return errLibraryPackInUse
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	path, err := s.libraryPathLocked("packs", id+".json")
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func registerLibraryDeleteAPI(mux *http.ServeMux) {
	mux.HandleFunc("DELETE /api/library/packs/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !contentID.MatchString(r.PathValue("id")) {
			writeError(w, 400, "题包编号无效")
			return
		}
		if err := disk.deleteLibraryPack(r.PathValue("id"), r.Header.Get("X-ELP-Directory")); err != nil {
			status := 400
			if errors.Is(err, errLibraryPackInUse) || errors.Is(err, errDirectoryChanged) {
				status = 409
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"deleted": true})
	})
}
