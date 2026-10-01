package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestImportReceiptSurvivesRetry(t *testing.T) {
	previous := disk
	disk = libraryTestStore(t)
	defer func() { disk = previous }()
	mux := http.NewServeMux()
	registerLibraryImportAPI(mux)
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, err := form.CreateFormFile("files", "pack.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(file).Encode(sampleLibraryPack()); err != nil {
		t.Fatal(err)
	}
	form.Close()
	request := httptest.NewRequest("POST", "/api/library/import/preview", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	request.Header.Set("X-ELP-Directory", disk.directoryIDLocked())
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	var preview struct {
		Token string `json:"token"`
	}
	json.Unmarshal(response.Body.Bytes(), &preview)
	defer func() { bankImports.Lock(); delete(bankImports.items, preview.Token); bankImports.Unlock() }()
	var first string
	for i := 0; i < 2; i++ {
		if i == 1 {
			bankImports.Lock()
			delete(bankImports.items, preview.Token)
			bankImports.Unlock()
		}
		request = httptest.NewRequest("POST", "/api/library/import/"+preview.Token, nil)
		request.Header.Set("X-ELP-Directory", disk.directoryIDLocked())
		response = httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != 200 {
			t.Fatal(response.Body.String())
		}
		if i == 0 {
			first = response.Body.String()
		} else if first != response.Body.String() {
			t.Fatal("retry changed saved receipt")
		}
	}
	oldID := disk.directoryIDLocked()
	disk.directory = t.TempDir()
	request = httptest.NewRequest("POST", "/api/library/import/"+preview.Token, nil)
	request.Header.Set("X-ELP-Directory", oldID)
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != 409 {
		t.Fatal("old receipt accepted in a different directory")
	}
}
