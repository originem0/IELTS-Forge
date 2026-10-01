//go:build !windows

package main

import (
	"errors"
	"os"
)

func lockDataDirectory(string) (*os.File, error) {
	return nil, errors.New("当前发行版仅支持 Windows 数据目录锁")
}
