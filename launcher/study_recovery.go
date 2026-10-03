package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type studyRecovery struct {
	Token  string                     `json:"token"`
	Data   map[string]json.RawMessage `json:"data"`
	Issues []string                   `json:"issues"`
	Counts map[string]int             `json:"counts"`
}

// Recovery is explicit and uses the newest readable index as the active set:
// older snapshots may repair a listed record, but must not resurrect deletions.
func (s *diskStore) previewStudyRecoveryLocked() (studyRecovery, error) {
	result := studyRecovery{Issues: []string{}, Counts: map[string]int{}}
	var snapshots []map[string]json.RawMessage
	var evidence []string
	for _, path := range recoveryCandidates(s.dataPathLocked(), s.backupPathLocked()) {
		raw, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return result, err
		}
		evidence = append(evidence, hashContent(raw))
		var envelope diskDataEnvelope
		if json.Unmarshal(raw, &envelope) != nil {
			result.Issues = append(result.Issues, filepath.Base(path)+" 无法解析")
			continue
		}
		if envelope.Version > 2 {
			return result, errUnsupportedDataVersion
		}
		values := map[string]json.RawMessage{}
		if envelope.Version == 1 {
			if json.Unmarshal(envelope.Data, &values) != nil || values == nil {
				continue
			}
		} else if envelope.Version == 2 {
			var manifest studyManifest
			if json.Unmarshal(envelope.Data, &manifest) != nil || manifest.Metadata == nil || manifest.Collections == nil {
				continue
			}
			values = manifest.Metadata
			for _, collection := range studyCollections {
				records := []json.RawMessage{}
				for _, ref := range manifest.Collections[collection] {
					record := json.RawMessage(`null`)
					if contentID.MatchString(ref.Hash) {
						path, err := s.libraryPathLocked("study-records", ref.Hash+".json")
						if err == nil {
							candidate, err := os.ReadFile(path)
							if err == nil && hashContent(candidate) == ref.Hash {
								record = candidate
							}
						}
					}
					// Preserve even unreadable IDs in the active set for an older-version lookup.
					records = append(records, json.RawMessage(fmt.Sprintf(`{"id":%q,"recoveryRecord":%s}`, ref.ID, record)))
				}
				values[collection], _ = json.Marshal(records)
			}
		} else {
			continue
		}
		snapshots = append(snapshots, values)
	}
	if len(snapshots) == 0 {
		return result, errors.New("没有可读取的档案索引，请保留目录并使用完整备份恢复")
	}
	result.Data = snapshots[0]
	for _, collection := range studyCollections {
		versions := []map[string]json.RawMessage{}
		var active []string
		for index, snapshot := range snapshots {
			var entries []json.RawMessage
			if json.Unmarshal(snapshot[collection], &entries) != nil {
				entries = nil
			}
			values := map[string]json.RawMessage{}
			for _, entry := range entries {
				var wrapper struct {
					ID             string          `json:"id"`
					RecoveryRecord json.RawMessage `json:"recoveryRecord"`
				}
				if json.Unmarshal(entry, &wrapper) != nil || wrapper.ID == "" {
					continue
				}
				if index == 0 {
					active = append(active, wrapper.ID)
				}
				if wrapper.RecoveryRecord != nil {
					entry = wrapper.RecoveryRecord
				}
				var identity struct {
					ID string `json:"id"`
				}
				if json.Unmarshal(entry, &identity) == nil && identity.ID == wrapper.ID && s.validateStudyMedia(entry) == nil {
					values[wrapper.ID] = entry
				}
			}
			versions = append(versions, values)
		}
		records := []json.RawMessage{}
		seen := map[string]bool{}
		for _, id := range active {
			if seen[id] {
				continue
			}
			seen[id] = true
			found := false
			for index, values := range versions {
				if raw, ok := values[id]; ok {
					records = append(records, raw)
					found = true
					if index > 0 {
						result.Issues = append(result.Issues, collection+"/"+id+" 使用较旧的健康版本")
					}
					break
				}
			}
			if !found {
				result.Issues = append(result.Issues, collection+"/"+id+" 无健康版本，原件保留在目录中")
			}
		}
		result.Counts[collection] = len(records)
		result.Data[collection], _ = json.Marshal(records)
	}
	raw, _ := json.Marshal(result.Data)
	if err := s.validateStudyMedia(raw); err != nil {
		return result, err
	}
	token, _ := json.Marshal([]any{evidence, result.Data, result.Issues})
	result.Token = hashContent(token)
	return result, nil
}

func registerStudyRecoveryAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/data/recovery", func(w http.ResponseWriter, r *http.Request) {
		disk.RLock()
		defer disk.RUnlock()
		if err := disk.checkDirectoryLocked(r.Header.Get("X-ELP-Directory")); err != nil {
			writeError(w, 409, err.Error())
			return
		}
		result, err := disk.previewStudyRecoveryLocked()
		if err != nil {
			writeError(w, 422, err.Error())
			return
		}
		writeJSON(w, 200, result)
	})
	mux.HandleFunc("POST /api/data/recovery", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Token string `json:"token"`
		}
		if err := decodeJSONLimit(w, r, &input, 4096); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		disk.Lock()
		defer disk.Unlock()
		if err := disk.checkDirectoryLocked(r.Header.Get("X-ELP-Directory")); err != nil {
			writeError(w, 409, err.Error())
			return
		}
		preview, err := disk.previewStudyRecoveryLocked()
		if err != nil {
			writeError(w, 422, err.Error())
			return
		}
		if preview.Token != input.Token {
			writeError(w, 409, "档案已改变，请重新检查抢救结果")
			return
		}
		// Keep both original snapshots and the recovery report before writing anything.
		for _, path := range []string{disk.dataPathLocked(), disk.backupPathLocked()} {
			if raw, err := os.ReadFile(path); err == nil {
				if err = preserveDamagedFile(path, raw); err != nil {
					writeError(w, 500, err.Error())
					return
				}
			}
		}
		report, _ := json.MarshalIndent(preview, "", "  ")
		if err := atomicLibraryWrite(filepath.Join(disk.directory, "recovery-"+preview.Token+".json"), report); err != nil {
			writeError(w, 500, err.Error())
			return
		}
		data, _ := json.Marshal(preview.Data)
		if err := disk.writeEnvelopeLocked(diskDataEnvelope{Version: 1, UpdatedAt: time.Now().Format(time.RFC3339), Data: data}); err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"data": preview.Data, "revision": dataRevision(data), "storage": disk.statusLocked()})
	})
}
