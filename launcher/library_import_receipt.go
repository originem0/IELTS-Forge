package main

import (
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"time"
)

var importToken = regexp.MustCompile(`^[a-f0-9]{32}$`)

type importReceipt struct {
	Token       string `json:"token"`
	DirectoryID string `json:"directoryId"`
	Added       int    `json:"added"`
	Existing    int    `json:"existing"`
	CreatedAt   string `json:"createdAt"`
}

func (s *diskStore) saveImportReceipt(token string, preview *bankImportPreview) error {
	s.RLock()
	defer s.RUnlock()
	if s.directoryIDLocked() != preview.DirectoryID {
		return errDirectoryChanged
	}
	if !importToken.MatchString(token) {
		return errors.New("导入回执编号无效")
	}
	path, err := s.libraryPathLocked("import-receipts", token+".json")
	if err != nil {
		return err
	}
	raw, err := json.Marshal(importReceipt{Token: token, DirectoryID: preview.DirectoryID, Added: preview.Added, Existing: len(preview.Packs) - preview.Added, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return err
	}
	return atomicLibraryWrite(path, raw)
}

func (s *diskStore) loadImportReceipt(token, expected string) (importReceipt, error) {
	s.RLock()
	defer s.RUnlock()
	var receipt importReceipt
	if !importToken.MatchString(token) {
		return receipt, errors.New("导入回执不存在")
	}
	if err := s.checkDirectoryLocked(expected); err != nil {
		return receipt, err
	}
	path, err := s.libraryPathLocked("import-receipts", token+".json")
	if err != nil {
		return receipt, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return receipt, err
	}
	if json.Unmarshal(raw, &receipt) != nil || receipt.Token != token || receipt.DirectoryID != s.directoryIDLocked() || receipt.Added < 0 || receipt.Existing < 0 {
		return receipt, errors.New("导入回执无效")
	}
	return receipt, nil
}
