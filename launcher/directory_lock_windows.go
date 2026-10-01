//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// A zero-share kernel handle is released even after a crash. A leftover file
// alone never means the directory is locked; no stale PID heuristics are needed.
func lockDataDirectory(directory string) (*os.File, error) {
	name := filepath.Join(directory, ".elp-directory.lock")
	if info, err := os.Lstat(name); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("数据目录锁不能是符号链接")
	}
	path, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	handle, err := syscall.CreateFile(path, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, fmt.Errorf("数据目录已被另一个学习中心占用，或没有写入权限：%w", err)
	}
	return os.NewFile(uintptr(handle), name), nil
}
