package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var studyMediaID = regexp.MustCompile(`^[a-f0-9]{64}\.(mp3|wav|ogg|webm|m4a|png|jpg|webp|gif)$`)

var studyCollections = []string{"writings", "speaking", "mistakes"}

type studyRecordRef struct {
	ID   string `json:"id"`
	Hash string `json:"hash"`
}
type studyManifest struct {
	Metadata    map[string]json.RawMessage  `json:"metadata"`
	Collections map[string][]studyRecordRef `json:"collections"`
}
type studyCollectionPatch struct {
	Upsert []json.RawMessage `json:"upsert"`
	Order  []string          `json:"order"`
}
type studyPatch struct {
	Metadata    map[string]json.RawMessage      `json:"metadata"`
	Collections map[string]studyCollectionPatch `json:"collections"`
	Base        *studyPatchBase                 `json:"base,omitempty"`
}

func isStudyCollection(name string) bool {
	for _, value := range studyCollections {
		if name == value {
			return true
		}
	}
	return false
}

func (s *diskStore) externalizeStudyMedia(value any) (any, error) {
	return s.externalizeStudyMediaAt(value, false)
}

func (s *diskStore) externalizeStudyMediaAt(value any, media bool) (any, error) {
	switch typed := value.(type) {
	case string:
		if !media {
			return value, nil
		}
		if !strings.HasPrefix(typed, "data:audio/") && !strings.HasPrefix(typed, "data:image/") {
			return value, nil
		}
		header, encoded, ok := strings.Cut(typed, ",")
		if !ok || !strings.HasSuffix(header, ";base64") {
			return nil, errors.New("媒体编码无效")
		}
		mime := strings.Split(strings.TrimPrefix(header, "data:"), ";")[0]
		extension := map[string]string{"audio/webm": "webm", "audio/mp4": "m4a", "audio/x-m4a": "m4a", "audio/x-wav": "wav", "image/gif": "gif", "audio/mpeg": "mp3", "audio/wav": "wav", "audio/ogg": "ogg", "image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}[mime]
		if extension == "" {
			return nil, errors.New("学习记录包含不支持的媒体类型")
		}
		directory, err := s.libraryPathLocked("study-media", "")
		if err != nil {
			return nil, err
		}
		name, digest, _, err := streamImportFile(base64.NewDecoder(base64.StdEncoding, strings.NewReader(encoded)), directory, 32<<20)
		if err != nil {
			return nil, err
		}
		defer os.Remove(name)
		id := digest + "." + extension
		destination, err := s.libraryPathLocked("study-media", id)
		if err != nil {
			return nil, err
		}
		if err := os.Rename(name, destination); err != nil {
			return nil, err
		}
		return "/api/study/media/" + id, nil
	case []any:
		for index, item := range typed {
			value, err := s.externalizeStudyMediaAt(item, media)
			if err != nil {
				return nil, err
			}
			typed[index] = value
		}
	case map[string]any:
		for key, item := range typed {
			value, err := s.externalizeStudyMediaAt(item, key == "audio" || key == "images" || key == "promptImages")
			if err != nil {
				return nil, err
			}
			typed[key] = value
		}
	}
	return value, nil
}

func (s *diskStore) storeStudyRecord(raw json.RawMessage) (studyRecordRef, json.RawMessage, error) {
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return studyRecordRef{}, nil, errors.New("练习记录必须是对象")
	}
	id, ok := value["id"].(string)
	if !ok || strings.TrimSpace(id) == "" {
		return studyRecordRef{}, nil, errors.New("练习记录缺少编号")
	}
	normalized, err := s.externalizeStudyMedia(value)
	if err != nil {
		return studyRecordRef{}, nil, err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return studyRecordRef{}, nil, err
	}
	if len(encoded) > 32<<20 {
		return studyRecordRef{}, nil, errors.New("单条练习记录过大")
	}
	refs := map[string]bool{}
	if err := studyMediaRefs(normalized, refs); err != nil {
		return studyRecordRef{}, nil, err
	}
	for id := range refs {
		path, err := s.libraryPathLocked("study-media", id)
		if err != nil {
			return studyRecordRef{}, nil, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return studyRecordRef{}, nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > 32<<20 {
			return studyRecordRef{}, nil, errors.New("学习媒体引用无效")
		}
	}
	ref := studyRecordRef{ID: id, Hash: hashContent(encoded)}
	path, err := s.libraryPathLocked("study-records", ref.Hash+".json")
	if err != nil {
		return ref, nil, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := atomicLibraryWrite(path, encoded); err != nil {
			return ref, nil, err
		}
	} else if err != nil {
		return ref, nil, err
	} else {
		digest, err := hashFile(path, 32<<20)
		if err != nil || digest != ref.Hash {
			return ref, nil, errors.New("已存在的练习记录校验失败")
		}
	}
	return ref, encoded, nil
}

func (s *diskStore) manifestFromLegacy(data json.RawMessage) (studyManifest, error) {
	manifest := studyManifest{Metadata: map[string]json.RawMessage{}, Collections: map[string][]studyRecordRef{}}
	if json.Unmarshal(data, &manifest.Metadata) != nil || manifest.Metadata == nil {
		return manifest, errors.New("学习档案无效")
	}
	for _, collection := range studyCollections {
		var records []json.RawMessage
		if raw, exists := manifest.Metadata[collection]; exists {
			if err := json.Unmarshal(raw, &records); err != nil {
				return manifest, err
			}
		}
		refs := []studyRecordRef{}
		seen := map[string]bool{}
		for _, raw := range records {
			ref, _, err := s.storeStudyRecord(raw)
			if err != nil {
				return manifest, err
			}
			if seen[ref.ID] {
				return manifest, errors.New("学习档案包含重复编号")
			}
			seen[ref.ID] = true
			refs = append(refs, ref)
		}
		manifest.Collections[collection] = refs
		delete(manifest.Metadata, collection)
	}
	return manifest, nil
}

var errUnsupportedDataVersion = errors.New("档案版本高于当前程序支持的版本，请使用更新的程序打开；未恢复旧备份或修改文件")

func readDiskEnvelope(primary, backup string) (diskDataEnvelope, error) {
	var lastErr error
	for _, path := range recoveryCandidates(primary, backup) {
		if path == "" {
			continue
		}
		raw, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return diskDataEnvelope{}, err
		}
		envelope, err := validateDiskSnapshot(raw, filepath.Dir(primary))
		if errors.Is(err, errUnsupportedDataVersion) {
			return diskDataEnvelope{}, err
		}
		if err == nil {
			if path != primary {
				envelope.RecoverySource = path
			}
			return envelope, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return diskDataEnvelope{}, lastErr
	}
	return diskDataEnvelope{Version: 1, Data: json.RawMessage(`{}`)}, nil
}

func recoveryCandidates(primary, backup string) []string {
	paths := []string{primary, backup}
	if primary != "" && backup != "" {
		daily, _ := filepath.Glob(filepath.Join(filepath.Dir(primary), "backups", "EnglishLearnPath-data-????-??-??.json"))
		sort.Sort(sort.Reverse(sort.StringSlice(daily)))
		paths = append(paths, daily...)
	}
	return paths
}

// A parseable manifest is not a recovery point until all records and media resolve.
func validateDiskSnapshot(raw []byte, directory string) (diskDataEnvelope, error) {
	var envelope diskDataEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return envelope, err
	}
	if envelope.Version > 2 {
		return envelope, errUnsupportedDataVersion
	}
	if envelope.Version != 1 && envelope.Version != 2 {
		return envelope, errors.New("学习档案版本无效")
	}
	store := &diskStore{directory: directory}
	data := envelope.Data
	if envelope.Version == 2 {
		var err error
		data, err = store.resolveStudyManifest(data)
		if err != nil {
			return envelope, err
		}
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) != nil || object == nil {
		return envelope, errors.New("学习档案不是有效对象")
	}
	if err := store.validateStudyMedia(data); err != nil {
		return envelope, err
	}
	envelope.ResolvedData = data
	return envelope, nil
}

func (s *diskStore) resolveStudyManifest(raw json.RawMessage) (json.RawMessage, error) {
	var manifest studyManifest
	if json.Unmarshal(raw, &manifest) != nil || manifest.Metadata == nil || manifest.Collections == nil {
		return nil, errors.New("练习索引无效")
	}
	result := manifest.Metadata
	recordDirectory, err := s.libraryPathLocked("study-records", "")
	if err != nil {
		return nil, err
	}
	for _, collection := range studyCollections {
		records := []json.RawMessage{}
		seen := map[string]bool{}
		for _, ref := range manifest.Collections[collection] {
			if ref.ID == "" || seen[ref.ID] || !contentID.MatchString(ref.Hash) {
				return nil, errors.New("练习索引引用无效")
			}
			seen[ref.ID] = true
			path, err := collectionFilePath(recordDirectory, ref.Hash+".json")
			if err != nil {
				return nil, err
			}
			record, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			if hashContent(record) != ref.Hash {
				return nil, errors.New("练习记录校验失败")
			}
			var identity struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(record, &identity) != nil || identity.ID != ref.ID {
				return nil, errors.New("练习索引编号不匹配")
			}
			records = append(records, record)
		}
		encoded, _ := json.Marshal(records)
		result[collection] = encoded
	}
	return json.Marshal(result)
}

func (s *diskStore) patchStudy(patch studyPatch, directoryID, revision string, commitIDs ...string) (map[string]any, error) {
	s.Lock()
	defer s.Unlock()
	if err := s.checkDirectoryLocked(directoryID); err != nil {
		return nil, err
	}
	if s.directory == "" {
		return nil, errors.New("请先绑定数据目录")
	}
	if revision == "" {
		return nil, errRevisionRequired
	}
	envelope, err := readDiskEnvelope(s.dataPathLocked(), s.backupPathLocked())
	if err != nil {
		return nil, err
	}
	commit, err := identifyCommit(commitIDs, "PATCH", revision, patch)
	if err != nil {
		return nil, err
	}
	if replay, err := s.replayCommit(envelope.Commits, commit); replay != nil || err != nil {
		return replay, err
	}
	if patch.Base != nil {
		data := envelope.ResolvedData
		if data == nil {
			data = envelope.Data
		}
		if err := validateStudyPatchBase(patch, data); err != nil {
			return nil, err
		}
	} else if dataRevision(envelope.Data) != revision {
		return nil, errDataConflict
	}
	if len(patch.Metadata) == 0 && len(patch.Collections) == 0 && envelope.RecoverySource == "" {
		return map[string]any{"saved": true, "revision": revision, "storage": s.statusLocked()}, nil
	}
	var manifest studyManifest
	if envelope.Version == 2 {
		err = json.Unmarshal(envelope.Data, &manifest)
	} else {
		manifest, err = s.manifestFromLegacy(envelope.Data)
	}
	if err != nil {
		return nil, err
	}
	if manifest.Metadata == nil || manifest.Collections == nil {
		return nil, errors.New("学习索引无效")
	}
	for key, value := range patch.Metadata {
		if isStudyCollection(key) {
			return nil, errors.New("集合不能作为元数据写入")
		}
		manifest.Metadata[key] = value
	}
	normalized := map[string][]json.RawMessage{}
	for collection, change := range patch.Collections {
		if !isStudyCollection(collection) {
			return nil, errors.New("未知练习集合")
		}
		refs := map[string]studyRecordRef{}
		order := []string{}
		for _, ref := range manifest.Collections[collection] {
			refs[ref.ID] = ref
			order = append(order, ref.ID)
		}
		updated := map[string]bool{}
		for _, raw := range change.Upsert {
			ref, encoded, err := s.storeStudyRecord(raw)
			if err != nil {
				return nil, err
			}
			if updated[ref.ID] {
				return nil, errors.New("请求包含重复练习")
			}
			updated[ref.ID] = true
			if _, exists := refs[ref.ID]; !exists {
				order = append(order, ref.ID)
			}
			refs[ref.ID] = ref
			normalized[collection] = append(normalized[collection], encoded)
		}
		if change.Order != nil {
			order = change.Order
		}
		entries := []studyRecordRef{}
		seen := map[string]bool{}
		for _, id := range order {
			ref, exists := refs[id]
			if !exists || seen[id] {
				return nil, errors.New("练习顺序包含重复或未知编号")
			}
			seen[id] = true
			entries = append(entries, ref)
		}
		manifest.Collections[collection] = entries
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	commit.Records = normalized
	if err := s.writeEnvelopeLocked(diskDataEnvelope{Version: 2, UpdatedAt: time.Now().Format(time.RFC3339), Data: encoded, Commits: appendCommit(envelope.Commits, commit, dataRevision(encoded))}); err != nil {
		return nil, err
	}
	s.studyWrites++
	if s.studyWrites == 1 || s.studyWrites%32 == 0 {
		if err := s.collectStudyGarbageLocked(); err != nil {
			log.Printf("学习版本清理已跳过：%v", err)
		}
	}
	return map[string]any{"saved": true, "revision": dataRevision(encoded), "storage": s.statusLocked(), "records": normalized}, nil
}

func registerStudyAPI(mux *http.ServeMux) {
	mux.HandleFunc("PATCH /api/data", func(w http.ResponseWriter, r *http.Request) {
		var patch studyPatch
		if err := decodeJSONLimit(w, r, &patch, maxDataFile); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		result, err := disk.patchStudy(patch, r.Header.Get("X-ELP-Directory"), r.Header.Get("If-Match"), r.Header.Get("X-ELP-Commit"))
		if err != nil {
			code := 500
			if errors.Is(err, errDirectoryChanged) || errors.Is(err, errDataConflict) {
				code = 409
			}
			if errors.Is(err, errRevisionRequired) {
				code = 428
			}
			writeError(w, code, err.Error())
			return
		}
		writeJSON(w, 200, result)
	})
	mux.HandleFunc("GET /api/study/media/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !studyMediaID.MatchString(id) {
			http.NotFound(w, r)
			return
		}
		disk.RLock()
		reader := &diskStore{directory: disk.directory}
		disk.RUnlock()
		path, err := reader.libraryPathLocked("study-media", id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", map[string]string{".webm": "audio/webm", ".m4a": "audio/mp4", ".gif": "image/gif", ".mp3": "audio/mpeg", ".wav": "audio/wav", ".ogg": "audio/ogg", ".png": "image/png", ".jpg": "image/jpeg", ".webp": "image/webp"}[filepath.Ext(id)])
		http.ServeFile(w, r, path)
	})
}
