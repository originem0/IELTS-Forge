package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Opt-in real-browser acceptance against the actual Go storage handlers. This
// always uses a temporary data directory, never the user's launcher config.
func TestLibraryBrowser(t *testing.T) {
	if os.Getenv("ELP_BROWSER_TEST") != "1" {
		t.Skip("set ELP_BROWSER_TEST=1 with Playwright available")
	}
	previousDisk, previousSettings := disk, settings
	disk = libraryTestStore(t)
	disk.configPath = filepath.Join(t.TempDir(), "config.json")
	settings = &configStore{}
	defer func() { disk, settings = previousDisk, previousSettings }()
	appDir, err := filepath.Abs("../app")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerAPI(mux)
	mux.Handle("/", secureStaticServer(appDir))
	server := httptest.NewServer(securityHeaders(mux))
	defer server.Close()
	cmd := exec.Command("node", "../tests/library-ui.cjs")
	cmd.Env = append(os.Environ(), "ELP_TEST_URL="+server.URL)
	output, err := cmd.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatal(err)
	}
}
