//go:build windows

package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var messageBoxW = syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")
var queryImage = syscall.NewLazyDLL("kernel32.dll").NewProc("QueryFullProcessImageNameW")

func sameExecutable(pid int, expected string) bool {
	handle, err := syscall.OpenProcess(0x1000, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	ok, _, _ := queryImage.Call(uintptr(handle), 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)))
	return ok != 0 && strings.EqualFold(filepath.Clean(syscall.UTF16ToString(buffer[:size])), filepath.Clean(expected))
}

func main() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	root := os.Getenv("ENGLISH_LEARN_PATH_CONFIG_DIR")
	if root == "" {
		root = filepath.Join(filepath.Dir(exe), "runtime-data")
	}
	paths, _ := filepath.Glob(filepath.Join(root, "instance-*.json"))
	client := &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	stopped := 0
	for _, path := range paths {
		var instance struct {
			ID  string `json:"id"`
			PID int    `json:"pid"`
			URL string `json:"url"`
		}
		raw, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(raw, &instance) != nil || instance.PID < 1 || instance.ID == "" {
			continue
		}
		u, err := url.Parse(instance.URL)
		if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" {
			continue
		}
		response, err := client.Get(instance.URL + "/api/app/info")
		if err != nil {
			continue
		}
		var info struct {
			ID string `json:"instanceId"`
		}
		err = json.NewDecoder(response.Body).Decode(&info)
		response.Body.Close()
		if err != nil || info.ID != instance.ID {
			continue
		}
		response, err = client.Post(instance.URL+"/api/app/shutdown", "application/json", strings.NewReader("{}"))
		if err != nil {
			continue
		}
		response.Body.Close()
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if _, err := os.Stat(path); err == nil {
			if !sameExecutable(instance.PID, filepath.Join(filepath.Dir(exe), "启动学习中心.exe")) {
				continue
			}
			if showMessage("服务仍在等待保存或转写完成。强制结束可能丢失未保存内容。是否应急终止此实例？", "English Learning Path", 0x34) == 6 {
				_ = exec.Command("taskkill.exe", "/F", "/PID", strconv.Itoa(instance.PID)).Run()
			} else {
				continue
			}
		}
		stopped++
	}
	message := "未发现本目录中可识别的运行实例。旧版本请在页面中退出。"
	if stopped > 0 {
		message = "本目录的学习中心服务已结束。浏览器中未保存的草稿请先复制保留。"
	}
	showMessage(message, "English Learning Path", 0x40)
}

func showMessage(text, title string, icon uintptr) uintptr {
	for _, arg := range os.Args[1:] {
		if arg == "--quiet" {
			return 7
		}
	}
	textPointer, _ := syscall.UTF16PtrFromString(text)
	titlePointer, _ := syscall.UTF16PtrFromString(title)
	result, _, _ := messageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(textPointer)),
		uintptr(unsafe.Pointer(titlePointer)),
		icon,
	)
	return result
}
