package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func hashFile(filename string, limit int64) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(file, limit+1))
	if err != nil {
		return "", err
	}
	if n > limit {
		return "", errors.New("文件超过大小限制")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func streamImportFile(reader io.Reader, directory string, limit int64) (name, digest string, size int64, err error) {
	file, err := os.CreateTemp(directory, ".import-*")
	if err != nil {
		return "", "", 0, err
	}
	name = file.Name()
	complete := false
	defer func() {
		file.Close()
		if !complete {
			os.Remove(name)
		}
	}()
	hash := sha256.New()
	size, err = io.Copy(io.MultiWriter(file, hash), io.LimitReader(reader, limit+1))
	if err != nil {
		return
	}
	if size > limit {
		err = errors.New("文件超过导入限制")
		return
	}
	if err = file.Sync(); err != nil {
		return
	}
	if err = file.Close(); err != nil {
		return
	}
	digest = hex.EncodeToString(hash.Sum(nil))
	complete = true
	return
}

func (preview *bankImportPreview) readMedia(reader io.Reader, size int64) error {
	buffered := bufio.NewReader(reader)
	prefix, err := buffered.Peek(512)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	extension := libraryMediaExtension(prefix)
	if extension == "" {
		preview.Ignored++
		return nil
	}
	stage := &diskStore{directory: preview.Stage}
	dir, err := stage.libraryPathLocked("media", "")
	if err != nil {
		return err
	}
	name, digest, written, err := streamImportFile(buffered, dir, 128<<20)
	if err != nil {
		return err
	}
	defer os.Remove(name)
	if written != size {
		return errors.New("附件上传不完整")
	}
	id := digest + "." + extension
	target, err := stage.libraryPathLocked("media", id)
	if err != nil {
		return err
	}
	if err := os.Rename(name, target); err != nil {
		return err
	}
	preview.Media[id] = true
	return nil
}

func (s *diskStore) importReader(expected string) (*diskStore, error) {
	s.RLock()
	defer s.RUnlock()
	if err := s.checkDirectoryLocked(expected); err != nil {
		return nil, err
	}
	if s.directory == "" {
		return nil, errors.New("请先选择永久数据目录")
	}
	return &diskStore{directory: s.directory}, nil
}

type preparedImportFile struct {
	target, temporary string
	existing          *fileStamp
	pack              bool
}

func (s *diskStore) commitImport(preview *bankImportPreview, directoryID string) (int, error) {
	reader, err := s.importReader(directoryID)
	if err != nil {
		return 0, err
	}
	if reader.directoryIDLocked() != preview.DirectoryID {
		return 0, errDirectoryChanged
	}
	prepared := []preparedImportFile{}
	defer func() {
		for _, file := range prepared {
			if file.temporary != "" {
				os.Remove(file.temporary)
			}
		}
	}()
	prepare := func(collection, name string) error {
		target, err := reader.libraryPathLocked(collection, name)
		if err != nil {
			return err
		}
		item := preparedImportFile{target: target, pack: collection == "packs"}
		expected := strings.Split(name, ".")[0]
		limit := int64(128 << 20)
		if item.pack {
			limit = maxPackSize
		}
		if info, err := os.Stat(target); err == nil {
			digest, err := hashFile(target, limit)
			if err != nil {
				return err
			}
			if digest != expected {
				return errors.New("本地题库文件已损坏，请先从备份恢复")
			}
			version := stamp(info)
			item.existing = &version
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		} else {
			source, err := os.Open(filepath.Join(preview.Stage, "library", collection, name))
			if err != nil {
				return err
			}
			temporary, digest, _, err := streamImportFile(source, filepath.Dir(target), limit)
			source.Close()
			if err != nil {
				return err
			}
			item.temporary = temporary
			if digest != expected {
				os.Remove(temporary)
				return errors.New("导入暂存文件校验失败")
			}
		}
		prepared = append(prepared, item)
		return nil
	}
	for id := range preview.Media {
		if err := prepare("media", id); err != nil {
			return 0, err
		}
	}
	for _, pack := range preview.Packs {
		if err := prepare("packs", pack.ID+".json"); err != nil {
			return 0, err
		}
	}
	// Only publication holds the archive mutex; uploads, hashing, copying and
	// fsync above cannot delay a writing/speaking autosave.
	s.Lock()
	defer s.Unlock()
	if s.directory != reader.directory {
		return 0, errDirectoryChanged
	}
	if err := s.checkDirectoryLocked(directoryID); err != nil {
		return 0, err
	}
	for _, item := range prepared {
		info, err := os.Stat(item.target)
		if item.existing != nil {
			if err != nil || stamp(info) != *item.existing {
				return 0, errors.New("题库已变化，请重试导入")
			}
		}
		if item.existing == nil && !errors.Is(err, os.ErrNotExist) {
			return 0, errors.New("题库已变化，请重试导入")
		}
	}
	written := []string{}
	complete := false
	defer func() {
		if !complete {
			for _, name := range written {
				os.Remove(name)
			}
		}
	}()
	added := 0
	for _, item := range prepared {
		if item.existing == nil {
			if err := os.Rename(item.temporary, item.target); err != nil {
				return 0, err
			}
			written = append(written, item.target)
			if item.pack {
				added++
			}
		}
	}
	complete = true
	return added, nil
}

func (preview *bankImportPreview) cleanup() {
	if preview.owner != nil {
		preview.owner.Close()
		preview.owner = nil
	}
	removeImportStage(preview.Stage)
}

func cleanupImportStages(now time.Time) { cleanupImportStagesAt(filepath.Clean(os.TempDir()), now) }

func cleanupImportStagesAt(root string, now time.Time) {
	files, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, file := range files {
		if !file.IsDir() || file.Type()&os.ModeSymlink != 0 || !strings.HasPrefix(file.Name(), "elp-bank-import-") {
			continue
		}
		info, err := file.Info()
		if err != nil || now.Sub(info.ModTime()) < 24*time.Hour {
			continue
		}
		path := filepath.Join(root, file.Name())
		// Never remove another running process's preview, even if it is old.
		lock, err := lockDataDirectory(path)
		if err != nil {
			continue
		}
		lock.Close()
		if filepath.Dir(path) == root && strings.HasPrefix(filepath.Base(path), "elp-bank-import-") {
			_ = os.RemoveAll(path)
		}
	}
}

func copyPreviewMedia(preview *bankImportPreview, reader *diskStore, ref string) error {
	filename, err := reader.libraryPathLocked("media", ref)
	if err != nil {
		return err
	}
	input, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if err := preview.readMedia(input, info.Size()); err != nil {
		return err
	}
	if !preview.Media[ref] {
		return fmt.Errorf("附件校验失败：%s", ref)
	}
	return nil
}
