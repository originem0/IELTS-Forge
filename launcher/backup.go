package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxBackupBytes int64 = 1 << 30
const backupManifestName = "backup-manifest.json"

var errDirectoryChanged = errors.New("数据目录已在其他页面更换，请重新载入后继续")

func (s *diskStore) directoryIDLocked() string {
	if s.directory == "" {
		return ""
	}
	return hashContent([]byte(filepath.Clean(s.directory)))
}

func (s *diskStore) checkDirectoryLocked(expected ...string) error {
	// Internal store operations omit the argument. HTTP writes always pass one,
	// so pre-restore tabs cannot overwrite the newly restored learning archive.
	if len(expected) == 0 {
		return nil
	}
	if expected[0] == "" && !s.strictDirectory {
		return nil
	}
	if expected[0] != s.directoryIDLocked() {
		return errDirectoryChanged
	}
	return nil
}

type backupEntry struct {
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
	ModifiedAt string `json:"modifiedAt"`
}
type backupManifest struct {
	Version   int           `json:"version"`
	CreatedAt string        `json:"createdAt"`
	Files     []backupEntry `json:"files"`
}

func backupFileLimit(name string) int64 {
	if name == dataFilename {
		return maxDataFile
	}
	parts := strings.Split(name, "/")
	if len(parts) != 3 || parts[0] != "library" {
		return 0
	}
	switch parts[1] {
	case "study-records":
		if strings.HasSuffix(parts[2], ".json") && contentID.MatchString(strings.TrimSuffix(parts[2], ".json")) {
			return 32 << 20
		}
	case "packs":
		if strings.HasSuffix(parts[2], ".json") && contentID.MatchString(strings.TrimSuffix(parts[2], ".json")) {
			return maxPackSize
		}
	case "study-media":
		if studyMediaID.MatchString(parts[2]) {
			return 32 << 20
		}
	case "media":
		if mediaID.MatchString(parts[2]) {
			return 128 << 20
		}
	case "attempts":
		id := strings.TrimSuffix(parts[2], ".json")
		id = strings.TrimSuffix(id, ".backup")
		if strings.HasSuffix(parts[2], ".json") && safeAttemptID(id) {
			return 1 << 20
		}
	}
	return 0
}

func validateArchiveDirectory(store *diskStore, files []backupEntry) error {
	data, err := loadDataFromPath(filepath.Join(store.directory, dataFilename), "")
	if err != nil {
		return err
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) != nil || object == nil {
		return errors.New("学习档案不是有效对象")
	}
	if err := store.validateStudyMedia(data); err != nil {
		return err
	}
	for _, entry := range files {
		parts := strings.Split(entry.Path, "/")
		if len(parts) != 3 {
			continue
		}
		switch parts[1] {
		case "packs":
			id := strings.TrimSuffix(parts[2], ".json")
			pack, err := store.loadPackLocked(id)
			if err != nil {
				return err
			}
			if _, err := store.importPack(pack); err != nil {
				return err
			}
		case "media", "study-media":
			if strings.Split(parts[2], ".")[0] != entry.SHA256 {
				return errors.New("媒体文件编号与内容不一致")
			}
		case "attempts":
			if strings.HasSuffix(parts[2], ".backup.json") {
				continue
			}
			attempt, err := store.loadAttemptLocked(strings.TrimSuffix(parts[2], ".json"))
			if err != nil {
				return err
			}
			if err := store.validateAttemptLocked(attempt); err != nil {
				return err
			}
			seen := map[string]bool{attempt.ID: true}
			for parent := attempt.ReviewOf; parent != ""; {
				if seen[parent] {
					return errors.New("练习复习来源构成循环")
				}
				seen[parent] = true
				original, err := store.loadAttemptLocked(parent)
				// Saved reviews retain their immutable scope after source deletion,
				// just as validateAttemptLocked permits when continuing the review.
				if errors.Is(err, os.ErrNotExist) {
					break
				}
				if err != nil {
					return err
				}
				parent = original.ReviewOf
			}
		}
	}
	return nil
}

func (s *diskStore) exportArchive(directoryID ...string) (*os.File, error) {
	stage, err := s.pinBackupSnapshot(directoryID...)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)
	return (&diskStore{directory: stage}).compressArchive()
}

// Pin immutable files with hard links under the archive lock. Compression and
// validation then run on an independent snapshot while practice saves continue.
// Atomic replacement of a live attempt cannot change the pinned old version.
func (s *diskStore) pinBackupSnapshot(directoryID ...string) (string, error) {
	s.Lock()
	defer s.Unlock()
	if err := s.checkDirectoryLocked(directoryID...); err != nil {
		return "", err
	}
	if s.directory == "" {
		return "", errors.New("请先绑定数据目录")
	}
	data, err := s.loadLocked()
	if err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(s.directory, ".backup-snapshot-")
	if err != nil {
		return "", err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(stage)
		}
	}()
	target := &diskStore{directory: stage}
	raw, _ := json.Marshal(diskDataEnvelope{Version: 1, UpdatedAt: s.lastWrite, Data: data})
	if err := atomicLibraryWrite(target.dataPathLocked(), raw); err != nil {
		return "", err
	}
	for _, collection := range []string{"packs", "media", "attempts", "study-records", "study-media"} {
		from, err := s.libraryPathLocked(collection, "")
		if err != nil {
			return "", err
		}
		to, err := target.libraryPathLocked(collection, "")
		if err != nil {
			return "", err
		}
		files, err := os.ReadDir(from)
		if err != nil {
			return "", err
		}
		for _, file := range files {
			if strings.HasPrefix(file.Name(), ".library-") || strings.HasPrefix(file.Name(), ".import-") || strings.Contains(file.Name(), ".damaged-") {
				continue
			}
			limit := backupFileLimit("library/" + collection + "/" + file.Name())
			if file.IsDir() || limit == 0 {
				return "", fmt.Errorf("资料目录含未知文件：%s", file.Name())
			}
			original, err := collectionFilePath(from, file.Name())
			if err != nil {
				return "", err
			}
			destination := filepath.Join(to, file.Name())
			if err := os.Link(original, destination); err != nil {
				// Filesystems without hard-link support retain the same snapshot rule.
				input, err := os.Open(original)
				if err != nil {
					return "", err
				}
				name, _, _, copyErr := streamImportFile(input, to, limit)
				input.Close()
				if copyErr != nil {
					return "", copyErr
				}
				if err := os.Rename(name, destination); err != nil {
					return "", err
				}
			}
		}
	}
	keep = true
	return stage, nil
}

func (s *diskStore) compressArchive() (*os.File, error) {
	file, err := os.CreateTemp("", "elp-backup-*.zip")
	if err != nil {
		return nil, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			file.Close()
			os.Remove(file.Name())
		}
	}()
	archive := zip.NewWriter(file)
	manifest := backupManifest{Version: 2, CreatedAt: time.Now().UTC().Format(time.RFC3339), Files: []backupEntry{}}
	var total int64
	add := func(name string, source io.Reader, size int64, modified time.Time) error {
		if len(manifest.Files) >= 9999 {
			return errors.New("备份文件数量超过 9999")
		}
		limit := backupFileLimit(name)
		if limit == 0 || size > limit || size < 0 || total+size > maxBackupBytes {
			return errors.New("备份文件超出大小限制")
		}
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetModTime(modified)
		header.SetMode(0600)
		if strings.HasPrefix(name, "library/media/") {
			header.Method = zip.Store
		}
		entry, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		hash := sha256.New()
		n, err := io.Copy(io.MultiWriter(entry, hash), io.LimitReader(source, limit+1))
		if err != nil {
			return err
		}
		if n != size {
			return errors.New("备份期间文件长度变化")
		}
		total += n
		manifest.Files = append(manifest.Files, backupEntry{Path: name, Size: n, SHA256: hex.EncodeToString(hash.Sum(nil)), ModifiedAt: modified.UTC().Format(time.RFC3339Nano)})
		return nil
	}
	data, err := loadDataFromPath(s.dataPathLocked(), s.backupPathLocked())
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(diskDataEnvelope{Version: 1, UpdatedAt: s.lastWrite, Data: data})
	if err != nil {
		return nil, err
	}
	if err = add(dataFilename, bytes.NewReader(raw), int64(len(raw)), time.Now()); err != nil {
		return nil, err
	}
	for _, collection := range []string{"packs", "media", "attempts", "study-records", "study-media"} {
		directory, err := s.libraryPathLocked(collection, "")
		if err != nil {
			return nil, err
		}
		entries, err := os.ReadDir(directory)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".library-") || strings.HasPrefix(entry.Name(), ".import-") || strings.Contains(entry.Name(), ".damaged-") {
				continue
			}
			name := "library/" + collection + "/" + entry.Name()
			if backupFileLimit(name) == 0 || entry.IsDir() {
				return nil, fmt.Errorf("资料目录含未知文件，未创建不完整备份：%s", name)
			}
			path, err := collectionFilePath(directory, entry.Name())
			if err != nil {
				return nil, err
			}
			input, err := os.Open(path)
			if err != nil {
				return nil, err
			}
			info, err := input.Stat()
			if err == nil {
				err = add(name, input, info.Size(), info.ModTime())
			}
			input.Close()
			if err != nil {
				return nil, err
			}
		}
	}
	// Validate using an independent mutex while the real store stays locked.
	if err := validateArchiveDirectory(&diskStore{directory: s.directory}, manifest.Files); err != nil {
		return nil, err
	}
	encoded, _ := json.Marshal(manifest)
	entry, err := archive.Create(backupManifestName)
	if err != nil {
		return nil, err
	}
	if _, err = entry.Write(encoded); err != nil {
		return nil, err
	}
	if err = archive.Close(); err != nil {
		return nil, err
	}
	if info, err := file.Stat(); err != nil || info.Size() > maxBackupBytes {
		return nil, errors.New("完整 ZIP 超过 1 GB，请使用整个数据目录进行迁移")
	}
	if err = file.Sync(); err != nil {
		return nil, err
	}
	if _, err = file.Seek(0, 0); err != nil {
		return nil, err
	}
	cleanup = false
	return file, nil
}

func (s *diskStore) restoreArchive(source io.ReaderAt, size int64, directoryID string) (string, error) {
	if size <= 0 || size > maxBackupBytes {
		return "", errors.New("备份大小无效，最大 1 GB")
	}
	archive, err := zip.NewReader(source, size)
	if err != nil {
		return "", errors.New("不是有效的 ZIP 备份")
	}
	if len(archive.File) > 10000 {
		return "", errors.New("备份文件数量过多")
	}
	files := map[string]*zip.File{}
	for _, file := range archive.File {
		limit := backupFileLimit(file.Name)
		if file.Name == backupManifestName {
			limit = 4 << 20
		}
		if limit == 0 || file.UncompressedSize64 > uint64(limit) || file.Mode()&os.ModeType != 0 || files[file.Name] != nil {
			return "", errors.New("备份含非法路径、重复文件或超大文件")
		}
		files[file.Name] = file
	}
	manifestFile := files[backupManifestName]
	if manifestFile == nil {
		return "", errors.New("备份缺少校验清单")
	}
	input, err := manifestFile.Open()
	if err != nil {
		return "", err
	}
	raw, err := io.ReadAll(io.LimitReader(input, (4<<20)+1))
	input.Close()
	if err != nil {
		return "", err
	}
	var manifest backupManifest
	if json.Unmarshal(raw, &manifest) != nil || manifest.Version != 2 || len(manifest.Files)+1 != len(files) {
		return "", errors.New("备份清单版本或文件数量无效")
	}
	seen := map[string]bool{}
	var total int64
	for _, entry := range manifest.Files {
		file := files[entry.Path]
		if _, err := time.Parse(time.RFC3339Nano, entry.ModifiedAt); err != nil {
			return "", errors.New("备份文件时间无效")
		}
		limit := backupFileLimit(entry.Path)
		if limit == 0 || file == nil || seen[entry.Path] || entry.Size < 0 || entry.Size > limit || uint64(entry.Size) != file.UncompressedSize64 || !contentID.MatchString(entry.SHA256) {
			return "", errors.New("备份清单与文件不一致")
		}
		seen[entry.Path] = true
		total += entry.Size
		if total > maxBackupBytes {
			return "", errors.New("解压后的备份超过 1 GB")
		}
	}
	if !seen[dataFilename] {
		return "", errors.New("备份缺少学习档案")
	}
	s.Lock()
	defer s.Unlock()
	if err := s.checkDirectoryLocked(directoryID); err != nil {
		return "", err
	}
	if s.directory == "" {
		return "", errors.New("请先绑定数据目录")
	}
	root, err := filepath.EvalSymlinks(s.directory)
	if err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(root, ".restore-")
	if err != nil {
		return "", err
	}
	// This fresh staging directory contains only allowlisted regular files. Never
	// clean the user's data directory or an existing distribution directory.
	defer func() {
		if filepath.Dir(stage) == root && strings.HasPrefix(filepath.Base(stage), ".restore-") {
			os.RemoveAll(stage)
		}
	}()
	for _, entry := range manifest.Files {
		path := filepath.Join(stage, filepath.FromSlash(entry.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return "", err
		}
		input, err := files[entry.Path].Open()
		if err != nil {
			return "", err
		}
		output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			input.Close()
			return "", err
		}
		hash := sha256.New()
		n, copyErr := io.Copy(io.MultiWriter(output, hash), io.LimitReader(input, entry.Size+1))
		input.Close()
		syncErr := output.Sync()
		closeErr := output.Close()
		if copyErr != nil || syncErr != nil || closeErr != nil || n != entry.Size || hex.EncodeToString(hash.Sum(nil)) != entry.SHA256 {
			return "", errors.New("备份校验失败，原数据未更改")
		}
		modified, _ := time.Parse(time.RFC3339Nano, entry.ModifiedAt)
		if err := os.Chtimes(path, modified, modified); err != nil {
			return "", err
		}
	}
	if err := validateArchiveDirectory(&diskStore{directory: stage}, manifest.Files); err != nil {
		return "", err
	}
	name := "restored-" + time.Now().Format("20060102-150405") + "-" + strings.TrimPrefix(filepath.Base(stage), ".restore-")
	destination := filepath.Join(root, name)
	if err := os.Rename(stage, destination); err != nil {
		return "", err
	}
	previous, previousWrite := s.directory, s.lastWrite
	nextLock, err := lockDataDirectory(destination)
	if err != nil {
		return "", err
	}
	s.directory = destination
	if err := s.persistConfigLocked(); err != nil {
		s.directory = previous
		s.lastWrite = previousWrite
		nextLock.Close()
		return "", fmt.Errorf("备份已恢复到 %s，但绑定失败：%w", destination, err)
	}
	if s.directoryLock != nil {
		s.directoryLock.Close()
	}
	s.directoryLock = nextLock
	s.strictDirectory = true
	s.lastWrite = time.Now().Format(time.RFC3339)
	return destination, nil
}

func registerBackupAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/backup", func(w http.ResponseWriter, r *http.Request) {
		file, err := disk.exportArchive(r.Header.Get("X-ELP-Directory"))
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		defer func() { file.Close(); os.Remove(file.Name()) }()
		info, err := file.Stat()
		if err != nil {
			writeError(w, 500, "无法读取备份")
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="EnglishLearnPath-full-backup.zip"`)
		http.ServeContent(w, r, "EnglishLearnPath-full-backup.zip", info.ModTime(), file)
	})
	mux.HandleFunc("POST /api/backup/restore", func(w http.ResponseWriter, r *http.Request) {
		// A bounded, route-local deadline matches the 1 GiB import allowance.
		_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(10 * time.Minute))
		file, err := os.CreateTemp("", "elp-restore-upload-*.zip")
		if err != nil {
			writeError(w, 500, "无法暂存备份")
			return
		}
		defer func() { file.Close(); os.Remove(file.Name()) }()
		n, err := io.Copy(file, http.MaxBytesReader(w, r.Body, maxBackupBytes))
		if err != nil {
			writeError(w, 400, "备份超过大小限制或上传中断")
			return
		}
		directory, err := disk.restoreArchive(file, n, r.Header.Get("X-ELP-Directory"))
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		result, err := disk.snapshot()
		if err != nil {
			writeError(w, 500, "恢复后读取失败")
			return
		}
		result["directory"] = directory
		writeJSON(w, 200, result)
	})
}
