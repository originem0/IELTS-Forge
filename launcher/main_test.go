package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDiskStorePersistsAndKeepsBackup(t *testing.T) {
	root := t.TempDir()
	store := &diskStore{directory: root, configPath: filepath.Join(root, "config.json")}
	first := json.RawMessage(`{"writings":[{"id":"one"}],"speaking":[],"activityDates":[]}`)
	second := json.RawMessage(`{"writings":[],"speaking":[],"activityDates":[]}`)

	if err := store.save(first); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := store.save(second); err != nil {
		t.Fatalf("second save: %v", err)
	}
	loaded, err := store.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	var loadedObject, expectedObject map[string]any
	if err := json.Unmarshal(loaded, &loadedObject); err != nil {
		t.Fatalf("decode loaded data: %v", err)
	}
	if err := json.Unmarshal(second, &expectedObject); err != nil {
		t.Fatalf("decode expected data: %v", err)
	}
	if !reflect.DeepEqual(loadedObject, expectedObject) {
		t.Fatalf("loaded data mismatch: %s", loaded)
	}
	if _, err := os.Stat(filepath.Join(root, backupName)); err != nil {
		t.Fatalf("rolling backup missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "backups", "EnglishLearnPath-data-"+todayForTest()+".json")); err != nil {
		t.Fatalf("daily backup missing: %v", err)
	}
}

func TestUnboundDiskStoreDoesNotWrite(t *testing.T) {
	store := &diskStore{configPath: filepath.Join(t.TempDir(), "config.json")}
	if err := store.save(json.RawMessage(`{"writings":[]}`)); err == nil {
		t.Fatal("unbound store should reject writes")
	}
	loaded, err := store.load()
	if err != nil || string(loaded) != "{}" {
		t.Fatalf("unbound load should be empty: %s, %v", loaded, err)
	}
}

func TestNewDiskStoreUsesIsolatedConfigAndStartsUnbound(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("ENGLISH_LEARN_PATH_CONFIG_DIR", configRoot)

	store, err := newDiskStore()
	if err != nil {
		t.Fatal(err)
	}
	if store.configPath != filepath.Join(configRoot, "config.json") {
		t.Fatalf("unexpected config path: %s", store.configPath)
	}
	if store.directoryPath() != "" {
		t.Fatalf("fresh portable config should be unbound: %s", store.directoryPath())
	}
}

func TestNewDiskStoreDoesNotRecreateStaleDirectory(t *testing.T) {
	configRoot := t.TempDir()
	missingDirectory := filepath.Join(t.TempDir(), "old-computer", "userdata")
	payload, err := json.Marshal(launcherConfig{DataDirectory: missingDirectory})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configRoot, "config.json"), payload, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENGLISH_LEARN_PATH_CONFIG_DIR", configRoot)

	store, err := newDiskStore()
	if err != nil {
		t.Fatal(err)
	}
	if store.directoryPath() != "" {
		t.Fatalf("stale directory should not be loaded: %s", store.directoryPath())
	}
	if _, err := os.Stat(missingDirectory); !os.IsNotExist(err) {
		t.Fatalf("stale directory must not be recreated: %v", err)
	}
}

func TestSelectedDirectoryPersistsAcrossRestarts(t *testing.T) {
	configRoot := t.TempDir()
	dataRoot := t.TempDir()
	t.Setenv("ENGLISH_LEARN_PATH_CONFIG_DIR", configRoot)

	first, err := newDiskStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(first.close)
	if loadedExisting, err := first.switchDirectory(dataRoot); err != nil {
		t.Fatal(err)
	} else if loadedExisting {
		t.Fatal("fresh directory should not be reported as existing data")
	}

	first.close()
	second, err := newDiskStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(second.close)
	selected, err := os.Stat(dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := os.Stat(second.directoryPath())
	if err != nil {
		t.Fatal(err)
	}
	// Windows may expand RUNNER~1 in the selected path. Compare directory
	// identity so equivalent short/long paths still prove binding persistence.
	if !os.SameFile(selected, restored) {
		t.Fatalf("selected directory %s was not restored: %s", dataRoot, second.directoryPath())
	}
	if _, err := os.Stat(filepath.Join(dataRoot, dataFilename)); err != nil {
		t.Fatalf("data file was not created: %v", err)
	}
}

func TestSecurityHeadersAllowLocalImagePreviewBlobs(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	policy := response.Header().Get("Content-Security-Policy")
	if !strings.Contains(policy, "img-src 'self' data: blob:") {
		t.Fatalf("image previews created from local files need blob CSP support: %s", policy)
	}
}

func todayForTest() string {
	return timeNow().Format("2006-01-02")
}

func TestBindingDoesNotCreateResourceDirectories(t *testing.T) {
	root := t.TempDir()
	store := &diskStore{configPath: filepath.Join(t.TempDir(), "config.json")}
	t.Cleanup(store.close)
	if _, err := store.switchDirectory(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "resources")); !os.IsNotExist(err) {
		t.Fatalf("unexpected resources directory: %v", err)
	}
}

func TestRetiredResourceAPIsAreUnavailable(t *testing.T) {
	mux := http.NewServeMux()
	registerAPI(mux)
	for _, path := range []string{"/api/resources", "/api/resources/rescan", "/api/resources/import", "/api/resources/open", "/api/resources/open-directory"} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(method, path, nil))
			if response.Code != http.StatusNotFound {
				t.Fatalf("retired API %s %s returned %d", method, path, response.Code)
			}
		}
	}
}

func TestAppInfoUsesInjectedBuildVersion(t *testing.T) {
	previous := appVersion
	appVersion = "v-test-build"
	t.Cleanup(func() { appVersion = previous })
	mux := http.NewServeMux()
	registerAPI(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/app/info", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"version":"v-test-build"`) {
		t.Fatalf("build version missing from app info: %d %s", response.Code, response.Body.String())
	}
}

func TestProviderNeutralOutputContracts(t *testing.T) {
	writing := "主题：城市交通\n\n### 评分与小分\n6.5\n\n### 总体评价\n清晰。\n\n### 确定语法错误\n没有。\n\n### 原文优化建议\n更具体。\n\n### 目标水平范文\nExample.\n\n### 最终值得记忆的语料\npublic transport"
	if err := validateOutputContract(writing, "review-markdown-v1-writing"); err != nil {
		t.Fatalf("valid writing contract rejected: %v", err)
	}
	if err := validateOutputContract(strings.Replace(writing, "### 评分与小分", "### 评分", 1), "review-markdown-v1-writing"); err == nil {
		t.Fatal("review without the fixed score heading must be rejected")
	}
	wrongOrder := strings.Replace(writing, "### 评分与小分\n6.5\n\n### 总体评价\n清晰。", "### 总体评价\n清晰。\n\n### 评分与小分\n6.5", 1)
	if err := validateOutputContract(wrongOrder, "review-markdown-v1-writing"); err == nil {
		t.Fatal("review headings in the wrong order must be rejected")
	}
	bank := `{"summary":"updated","speaking":[],"writing":[],"noContentSources":[{"sourceKey":"fixture","reason":"Only repeats the prompt."}]}`
	if err := validateOutputContract(bank, "personal-language-bank-json-v1"); err != nil {
		t.Fatalf("valid language bank rejected: %v", err)
	}
	if err := validateOutputContract(`{"summary":"missing arrays"}`, "personal-language-bank-json-v1"); err == nil {
		t.Fatal("language bank without fixed arrays must be rejected")
	}
	if err := validateOutputContract(`{"summary":"wrong type","speaking":{},"writing":[]}`, "personal-language-bank-json-v1"); err == nil {
		t.Fatal("language bank fields with the wrong type must be rejected")
	}
	plan := `{"summary":"plan","priorities":[],"phases":[]}`
	if err := validateOutputContract(plan, "study-plan-json-v1"); err == nil {
		t.Fatal("empty plan accepted")
	}
	for contract, content := range map[string]string{
		"study-plan-json-v1":             `{"summary":"plan","priorities":[],"phases":[null,42]}`,
		"personal-language-bank-json-v1": `{"summary":"","speaking":[],"writing":[]}`,
		"review-markdown-v1-writing":     "主题：x\n### 评分与小分\n### 总体评价\n### 确定语法错误\n### 原文优化建议\n### 目标水平范文\n### 最终值得记忆的语料",
	} {
		if err := validateOutputContract(content, contract); err == nil {
			t.Fatalf("accepted unusable %s", contract)
		}
	}
}

func TestGuardLocalRequests(t *testing.T) {
	const addr = "127.0.0.1:52345"
	guard := guardLocalRequests(addr, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	run := func(path string, mutate func(*http.Request)) int {
		r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+path, nil)
		r.Host = addr
		mutate(r)
		w := httptest.NewRecorder()
		guard.ServeHTTP(w, r)
		return w.Code
	}
	if code := run("/api/data", func(r *http.Request) {
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		r.Header.Set("Origin", "http://"+addr)
	}); code != http.StatusNoContent {
		t.Fatalf("same-origin request blocked: %d", code)
	}
	if code := run("/api/data", func(*http.Request) {}); code != http.StatusNoContent {
		t.Fatalf("headerless local request blocked: %d", code)
	}
	if code := run("/api/data", func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }); code != http.StatusForbidden {
		t.Fatalf("cross-site request allowed: %d", code)
	}
	if code := run("/api/data", func(r *http.Request) { r.Header.Set("Origin", "https://example.com") }); code != http.StatusForbidden {
		t.Fatalf("foreign origin allowed: %d", code)
	}
	if code := run("/api/data", func(r *http.Request) { r.Host = "evil.example.com" }); code != http.StatusForbidden {
		t.Fatalf("rebinding host allowed: %d", code)
	}
	// Static assets are governed by CSP, not the API guard.
	if code := run("/index.html", func(r *http.Request) {
		r.Host = "evil.example.com"
		r.Header.Set("Sec-Fetch-Site", "cross-site")
	}); code != http.StatusNoContent {
		t.Fatalf("static path should bypass api guard: %d", code)
	}
}

func TestDailyBackupsArePruned(t *testing.T) {
	backupsDir := filepath.Join(t.TempDir(), "backups")
	if err := os.MkdirAll(backupsDir, 0700); err != nil {
		t.Fatal(err)
	}
	for month := 1; month <= 12; month++ {
		for day := 1; day <= 4; day++ {
			name := fmt.Sprintf("EnglishLearnPath-data-2024-%02d-%02d.json", month, day)
			if err := os.WriteFile(filepath.Join(backupsDir, name), []byte("{}"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	// A non-daily file must never be touched.
	if err := os.WriteFile(filepath.Join(backupsDir, "notes.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	pruneDailyBackups(backupsDir, dailyBackupKeep)
	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != dailyBackupKeep+1 {
		t.Fatalf("expected %d files after prune, got %d", dailyBackupKeep+1, len(entries))
	}
	if _, err := os.Stat(filepath.Join(backupsDir, "EnglishLearnPath-data-2024-01-01.json")); !os.IsNotExist(err) {
		t.Fatal("oldest daily backup should have been pruned")
	}
	if _, err := os.Stat(filepath.Join(backupsDir, "EnglishLearnPath-data-2024-12-04.json")); err != nil {
		t.Fatal("newest daily backup should be kept")
	}
	if _, err := os.Stat(filepath.Join(backupsDir, "notes.txt")); err != nil {
		t.Fatal("unrelated file must not be pruned")
	}
}

var timeNow = func() time.Time { return time.Now() }
