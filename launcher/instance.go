package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func registerInstance(url string) (func(), error) {
	var token [24]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, err
	}
	instanceID = hex.EncodeToString(token[:])
	path := filepath.Join(filepath.Dir(disk.configPath), fmt.Sprintf("instance-%d.json", os.Getpid()))
	raw, _ := json.Marshal(map[string]any{"id": instanceID, "pid": os.Getpid(), "url": url})
	if err := atomicLibraryWrite(path, raw); err != nil {
		return nil, err
	}
	return func() { _ = os.Remove(path) }, nil
}
