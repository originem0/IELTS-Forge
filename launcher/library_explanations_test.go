package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestObjectiveExplanationValidationAndPersistence(t *testing.T) {
	oldDisk, oldSettings, oldClient := disk, settings, client
	disk = libraryTestStore(t)
	settings = &configStore{value: aiConfig{BaseURL: "https://text.test", Model: "text-model", Connected: true, Vision: &aiEndpoint{BaseURL: "https://vision.test", Model: "vision-model", Connected: true}}}
	defer func() { disk, settings, client = oldDisk, oldSettings, oldClient }()
	pack := sampleLibraryPack()
	pack.Units[0].Groups[0].Questions = append(pack.Units[0].Groups[0].Questions, libraryQuestion{ID: "q2", Label: "2", Text: "Correct question", Answers: []string{"Monday"}})
	packID, err := disk.importPack(pack)
	if err != nil {
		t.Fatal(err)
	}
	record, err := disk.saveAttempt(libraryAttempt{ID: "explain", PackID: packID, UnitID: "reading-1", Mode: "practice", Status: "submitted", Answers: map[string][]string{"q1": {"Tuesday"}, "q2": {"Monday"}}})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerLibraryAttemptsAPI(mux)
	calls := 0
	bad := false
	deleteDuringCall := false
	client = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		var request struct {
			Model    string        `json:"model"`
			Messages []chatMessage `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&request)
		if request.Model != "text-model" {
			t.Error("wrong endpoint model", request.Model)
		}
		context := request.Messages[1].Content.(string)
		if strings.Contains(context, "q2") || strings.Contains(context, "image_url") {
			t.Error("unselected question or image sent")
		}
		quote := "The library opens on Monday."
		if bad || calls == 1 {
			quote = "An invented quote."
		}
		content, _ := json.Marshal(map[string]any{"items": []any{map[string]any{"id": "q1", "explanation": "原文明确给出 Monday。", "trap": "Tuesday 与原文不符。", "evidence": []objectiveEvidence{{Source: "A", Quote: quote}}, "score": 99}}})
		if deleteDuringCall {
			path, _ := disk.libraryPathLocked("attempts", record.ID+".json")
			os.Remove(path)
		}
		raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(content)}, "finish_reason": "stop"}}})
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}
	request := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/library/attempts/explain/explanations", strings.NewReader(body))
		r.Header.Set("X-ELP-Directory", disk.directoryIDLocked())
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	if w := request(`{"questions":["q2"]}`); w.Code != 400 || calls != 0 {
		t.Fatal("correct answer accepted", w.Code)
	}
	if w := request(`{"questions":["q1"]}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if calls != 2 {
		t.Fatal("invalid evidence was not retried once", calls)
	}
	saved, err := disk.loadAttemptLocked(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved.Answers, record.Answers) || saved.Status != record.Status || len(saved.Explanations) != 1 {
		t.Fatal("answer/state modified")
	}
	before := saved.Revision
	bad = true
	if w := request(`{"questions":["q1"]}`); w.Code != 502 {
		t.Fatal(w.Code)
	}
	saved, _ = disk.loadAttemptLocked(record.ID)
	if saved.Revision != before || saved.Explanations["q1"].Evidence[0].Quote != "The library opens on Monday." {
		t.Fatal("failure overwrote saved explanation")
	}
	bad = false
	deleteDuringCall = true
	if w := request(`{"questions":["q1"]}`); w.Code != 409 {
		t.Fatal("deleted attempt recreated", w.Code)
	}
	if _, err = disk.loadAttemptLocked(record.ID); err == nil {
		t.Fatal("deleted attempt resurrected")
	}
}

func TestObjectiveExplanationBrowser(t *testing.T) {
	if os.Getenv("ELP_BROWSER_TEST") != "1" {
		t.Skip("browser opt-in")
	}
	oldDisk, oldSettings := disk, settings
	disk = libraryTestStore(t)
	disk.configPath = filepath.Join(t.TempDir(), "config.json")
	settings = &configStore{value: aiConfig{BaseURL: "https://text.test", Model: "text-model", Connected: true}}
	defer func() { disk, settings = oldDisk, oldSettings }()
	content := `{"items":[{"id":"q1","explanation":"开放日期是 Monday，而不是 Tuesday。","trap":"留意题目询问的日期。","evidence":[{"source":"SOURCE","quote":"The library opens on Monday."}]}]}`
	oldClient := client
	defer func() { client = oldClient }()
	client = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		var request map[string]any
		json.NewDecoder(r.Body).Decode(&request)
		raw, _ := json.Marshal(request)
		source := "A"
		if strings.Contains(string(raw), "transcript") {
			source = "transcript"
		}
		response, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": strings.ReplaceAll(content, "SOURCE", source)}, "finish_reason": "stop"}}})
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(response)))}, nil
	})}
	mux := http.NewServeMux()
	registerAPI(mux)
	appDir, _ := filepath.Abs("../app")
	mux.Handle("/", secureStaticServer(appDir))
	server := httptest.NewServer(securityHeaders(mux))
	defer server.Close()
	cmd := exec.Command("node", "../tests/objective-explanations.cjs")
	cmd.Env = append(os.Environ(), "ELP_TEST_URL="+server.URL)
	output, err := cmd.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatal(err)
	}
}
