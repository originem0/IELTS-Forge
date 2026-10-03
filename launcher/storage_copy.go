package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Directory switching is rare and intentionally serialized with saves. Copy
// referenced libraries before publishing the main archive, never overwrite an
// existing destination file with different content.
func copyLearningFiles(source, destination string) error {
	if source == "" {
		return nil
	}
	from, to := &diskStore{directory: source}, &diskStore{directory: destination}
	for _, collection := range []string{"packs", "media", "attempts", "study-records", "study-media"} {
		dir, err := from.libraryPathLocked(collection, "")
		if err != nil {
			return err
		}
		files, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, file := range files {
			if strings.HasPrefix(file.Name(), ".library-") || strings.HasPrefix(file.Name(), ".import-") || strings.Contains(file.Name(), ".damaged-") {
				continue
			}
			limit := backupFileLimit("library/" + collection + "/" + file.Name())
			if file.IsDir() || limit == 0 {
				return fmt.Errorf("资料目录含未知文件：%s", file.Name())
			}
			original, err := from.libraryPathLocked(collection, file.Name())
			if err != nil {
				return err
			}
			target, err := to.libraryPathLocked(collection, file.Name())
			if err != nil {
				return err
			}
			digest, err := hashFile(original, limit)
			if err != nil {
				return err
			}
			if _, err := os.Stat(target); err == nil {
				existing, err := hashFile(target, limit)
				if err != nil || existing != digest {
					return errors.New("目标目录存在不同的学习资料，未覆盖")
				}
				continue
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			input, err := os.Open(original)
			if err != nil {
				return err
			}
			info, err := input.Stat()
			if err != nil {
				input.Close()
				return err
			}
			name, copied, _, err := streamImportFile(input, filepath.Dir(target), limit)
			input.Close()
			if err != nil {
				return err
			}
			if copied != digest {
				os.Remove(name)
				return errors.New("复制期间来源文件发生变化")
			}
			if err := os.Rename(name, target); err != nil {
				os.Remove(name)
				return err
			}
			if err := os.Chtimes(target, info.ModTime(), info.ModTime()); err != nil {
				return err
			}
		}
	}
	return nil
}
