package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAuditBrowser(t *testing.T) {
	if os.Getenv("ELP_BROWSER_TEST") != "1" {
		t.Skip("browser opt-in")
	}
	oldDisk, oldSettings := disk, settings
	disk = libraryTestStore(t)
	disk.configPath = filepath.Join(t.TempDir(), "config.json")
	settings = &configStore{}
	defer func() { disk, settings = oldDisk, oldSettings }()
	dir, err := filepath.Abs("../app")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerAPI(mux)
	mux.Handle("/", secureStaticServer(dir))
	server := httptest.NewServer(securityHeaders(mux))
	defer server.Close()
	cmd := exec.Command("node", "../tests/audit-storage-browser.cjs")
	cmd.Env = append(os.Environ(), "ELP_TEST_URL="+server.URL)
	output, err := cmd.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := disk.load(); err != nil {
		t.Fatal("browser result failed real disk validation", err)
	}
}
