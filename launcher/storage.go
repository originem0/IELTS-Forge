package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func newDiskStore() (*diskStore, error) {
	cleanupImportStages(time.Now())
	var configRoot string
	if override := strings.TrimSpace(os.Getenv("ENGLISH_LEARN_PATH_CONFIG_DIR")); override != "" {
		configRoot = override
	} else {
		executable, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("无法定位启动器目录：%w", err)
		}
		configRoot = filepath.Join(filepath.Dir(executable), configDirname)
	}
	if err := os.MkdirAll(configRoot, 0700); err != nil {
		return nil, fmt.Errorf("无法创建便携配置目录 %s：%w", configRoot, err)
	}

	store := &diskStore{
		configPath: filepath.Join(configRoot, "config.json"),
	}
	if raw, readErr := os.ReadFile(store.configPath); readErr == nil {
		var saved launcherConfig
		if json.Unmarshal(raw, &saved) == nil && filepath.IsAbs(saved.DataDirectory) {
			candidate := filepath.Clean(saved.DataDirectory)
			// Never recreate a stale absolute path copied from another computer.
			// A missing directory means this portable copy starts unbound and asks
			// the current user to choose a local folder again.
			if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
				store.directory = candidate
			}
		}
	}
	if store.directory != "" {
		var err error
		store.directoryLock, err = lockDataDirectory(store.directory)
		if err != nil {
			return nil, err
		}
		if info, statErr := os.Stat(store.dataPathLocked()); statErr == nil {
			store.lastWrite = info.ModTime().Format(time.RFC3339)
		}
	}
	return store, nil
}

func (s *diskStore) directoryPath() string {
	s.RLock()
	defer s.RUnlock()
	return s.directory
}

func (s *diskStore) dataPathLocked() string {
	if s.directory == "" {
		return ""
	}
	return filepath.Join(s.directory, dataFilename)
}

func (s *diskStore) backupPathLocked() string {
	if s.directory == "" {
		return ""
	}
	return filepath.Join(s.directory, backupName)
}

func (s *diskStore) status() map[string]any {
	s.RLock()
	defer s.RUnlock()
	return s.statusLocked()
}

func (s *diskStore) statusLocked() map[string]any {
	dataPath := s.dataPathLocked()
	bound := s.directory != ""
	fileExists := false
	if bound {
		_, statErr := os.Stat(dataPath)
		fileExists = statErr == nil
	}
	return map[string]any{
		"ready":          bound,
		"bound":          bound,
		"directory":      s.directory,
		"dataFile":       dataPath,
		"fileExists":     fileExists,
		"lastWriteAt":    s.lastWrite,
		"browserStorage": false,
		"directoryId":    s.directoryIDLocked(),
	}
}

func (s *diskStore) load() (json.RawMessage, error) {
	s.RLock()
	defer s.RUnlock()
	return s.loadLocked()
}

func (s *diskStore) loadLocked() (json.RawMessage, error) {
	if s.directory == "" {
		return json.RawMessage(`{}`), nil
	}
	return loadDataFromPath(s.dataPathLocked(), s.backupPathLocked())
}

func loadDataFromPath(dataPath, fallbackPath string) (json.RawMessage, error) {
	envelope, err := readDiskEnvelope(dataPath, fallbackPath)
	if err != nil {
		return nil, err
	}
	if envelope.ResolvedData != nil {
		return envelope.ResolvedData, nil
	}
	return envelope.Data, nil
}

func decodeDiskEnvelope(raw []byte) (json.RawMessage, error) {
	var envelope diskDataEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	if envelope.Version > 2 {
		return nil, errUnsupportedDataVersion
	}
	if len(envelope.Data) == 0 || !json.Valid(envelope.Data) {
		return nil, errors.New("数据文件缺少有效的 data 字段")
	}
	return envelope.Data, nil
}

func (s *diskStore) save(data json.RawMessage, directoryID ...string) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil || object == nil {
		return errors.New("学习数据必须是 JSON 对象")
	}
	s.Lock()
	defer s.Unlock()
	if err := s.checkDirectoryLocked(directoryID...); err != nil {
		return err
	}
	if s.directory == "" {
		return errors.New("请先选择或创建永久数据文件夹")
	}
	return s.writeLocked(data)
}

func (s *diskStore) writeLocked(data json.RawMessage) error {
	return s.writeEnvelopeLocked(diskDataEnvelope{Version: 1, UpdatedAt: time.Now().Format(time.RFC3339), Data: data})
}

func (s *diskStore) writeEnvelopeLocked(envelope diskDataEnvelope) error {
	if err := os.MkdirAll(s.directory, 0700); err != nil {
		return err
	}
	dataPath := s.dataPathLocked()
	backupPath := s.backupPathLocked()
	previous, previousErr := os.ReadFile(dataPath)
	if previousErr != nil && !errors.Is(previousErr, os.ErrNotExist) {
		return previousErr
	}
	// Protect every write path, including internal saves that bypass revision checks.
	if _, err := decodeDiskEnvelope(previous); errors.Is(err, errUnsupportedDataVersion) {
		return err
	}
	_, validErr := validateDiskSnapshot(previous, s.directory)
	if previousErr == nil && validErr != nil {
		if err := preserveDamagedFile(dataPath, previous); err != nil {
			return err
		}
	}
	if previousErr == nil && validErr == nil {
		if err := atomicLibraryWrite(backupPath, previous); err != nil {
			return fmt.Errorf("创建滚动备份失败：%w", err)
		}
		backupsDir := filepath.Join(s.directory, "backups")
		if err := os.MkdirAll(backupsDir, 0700); err != nil {
			return fmt.Errorf("创建每日备份目录失败：%w", err)
		} else {
			dailyPath := filepath.Join(backupsDir, "EnglishLearnPath-data-"+time.Now().Format("2006-01-02")+".json")
			if _, err := os.Stat(dailyPath); errors.Is(err, os.ErrNotExist) {
				if err := atomicLibraryWrite(dailyPath, previous); err != nil {
					return fmt.Errorf("创建每日备份失败：%w", err)
				}
			} else if err != nil {
				return err
			}
			pruneDailyBackups(backupsDir, dailyBackupKeep)
		}
	}

	encoded, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicLibraryWrite(dataPath, encoded); err != nil {
		return err
	}
	s.lastWrite = envelope.UpdatedAt
	return nil
}

// pruneDailyBackups keeps only the newest keep daily snapshots so the backups
// directory cannot grow without bound over months of use. The yyyy-mm-dd file
// names sort chronologically, so the oldest names are simply the smallest.
func pruneDailyBackups(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var daily []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && strings.HasPrefix(name, "EnglishLearnPath-data-") && strings.HasSuffix(name, ".json") {
			daily = append(daily, name)
		}
	}
	if len(daily) <= keep {
		return
	}
	sort.Strings(daily)
	for _, name := range daily[:len(daily)-keep] {
		_ = os.Remove(filepath.Join(dir, name))
	}
}

func (s *diskStore) switchDirectory(selected string) (bool, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(selected))
	if err != nil || !filepath.IsAbs(absolute) {
		return false, errors.New("选择的目录无效")
	}
	if err := os.MkdirAll(absolute, 0700); err != nil {
		return false, err
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return false, err
	}
	s.Lock()
	defer s.Unlock()
	if strings.EqualFold(absolute, s.directory) {
		return true, nil
	}
	nextLock, err := lockDataDirectory(absolute)
	if err != nil {
		return false, err
	}
	committed := false
	defer func() {
		if !committed {
			nextLock.Close()
		}
	}()
	current, err := s.loadLocked()
	if err != nil {
		return false, err
	}
	targetPath := filepath.Join(absolute, dataFilename)
	_, statErr := os.Stat(targetPath)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return false, statErr
	}
	loadedExisting := statErr == nil
	if loadedExisting {
		if _, err := loadDataFromPath(targetPath, filepath.Join(absolute, backupName)); err != nil {
			return false, fmt.Errorf("所选目录中的数据文件无效：%w", err)
		}
	}
	previousDirectory, previousWrite := s.directory, s.lastWrite
	s.directory = absolute
	rollback := func() { s.directory = previousDirectory; s.lastWrite = previousWrite }
	if !loadedExisting {
		if err := copyLearningFiles(previousDirectory, absolute); err != nil {
			rollback()
			return false, err
		}
		if err := s.writeLocked(current); err != nil {
			rollback()
			return false, err
		}
	} else if info, err := os.Stat(targetPath); err == nil {
		s.lastWrite = info.ModTime().Format(time.RFC3339)
	}
	if err := s.persistConfigLocked(); err != nil {
		rollback()
		return false, err
	}
	if s.directoryLock != nil {
		s.directoryLock.Close()
	}
	s.directoryLock = nextLock
	s.strictDirectory = true
	committed = true
	return loadedExisting, nil
}

func (s *diskStore) close() {
	s.Lock()
	defer s.Unlock()
	if s.directoryLock != nil {
		s.directoryLock.Close()
		s.directoryLock = nil
	}
}

// The revision is independent of indentation introduced by disk envelopes.
func dataRevision(data json.RawMessage) string {
	var compact bytes.Buffer
	if json.Compact(&compact, data) != nil {
		return ""
	}
	return hashContent(compact.Bytes())
}

var errDataConflict = errors.New("档案已在其他页面更新。当前修改已保留，请先导出或复制草稿，再重新加载档案")
var errRevisionRequired = errors.New("缺少档案版本，请重新加载页面后再保存")

func (s *diskStore) snapshot() (map[string]any, error) {
	s.RLock()
	defer s.RUnlock()
	data := json.RawMessage(`{}`)
	revision := dataRevision(json.RawMessage(`{}`))
	status := s.statusLocked()
	if s.directory != "" {
		envelope, err := readDiskEnvelope(s.dataPathLocked(), s.backupPathLocked())
		if err != nil {
			return nil, err
		}
		revision = dataRevision(envelope.Data)
		data = envelope.ResolvedData
		if data == nil {
			data = envelope.Data
		}
		if envelope.RecoverySource != "" {
			status["recoverySource"] = envelope.RecoverySource
		}
	}
	return map[string]any{"data": data, "revision": revision, "storage": status}, nil
}

func preserveDamagedFile(path string, data []byte) error {
	// Content-address the original beside the file; retries never overwrite evidence.
	return atomicLibraryWrite(path+".damaged-"+hashContent(data), data)
}

func (s *diskStore) saveConditional(data json.RawMessage, directoryID, expected string, commitIDs ...string) (map[string]any, error) {
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) != nil || object == nil {
		return nil, errors.New("学习数据必须是 JSON 对象")
	}
	s.Lock()
	defer s.Unlock()
	if err := s.checkDirectoryLocked(directoryID); err != nil {
		return nil, err
	}
	if s.directory == "" {
		return nil, errors.New("请先选择永久数据目录")
	}
	if expected == "" {
		return nil, errRevisionRequired
	}
	previous, err := readDiskEnvelope(s.dataPathLocked(), s.backupPathLocked())
	if err != nil {
		return nil, err
	}
	commit, err := identifyCommit(commitIDs, "PUT", expected, data)
	if err != nil {
		return nil, err
	}
	if replay, err := s.replayCommit(previous.Commits, commit); replay != nil || err != nil {
		return replay, err
	}
	if dataRevision(previous.Data) != expected {
		return nil, errDataConflict
	}
	if err := s.validateStudyMedia(data); err != nil {
		return nil, err
	}
	if err := s.writeEnvelopeLocked(diskDataEnvelope{Version: 1, UpdatedAt: time.Now().Format(time.RFC3339), Data: data, Commits: appendCommit(previous.Commits, commit, dataRevision(data))}); err != nil {
		return nil, err
	}
	return map[string]any{"saved": true, "revision": dataRevision(data), "storage": s.statusLocked()}, nil
}

func (s *diskStore) persistConfigLocked() error {
	payload, err := json.MarshalIndent(launcherConfig{DataDirectory: s.directory}, "", "  ")
	if err != nil {
		return err
	}
	return atomicLibraryWrite(s.configPath, payload)
}
