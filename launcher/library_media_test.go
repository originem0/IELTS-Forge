package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func rawMP3Fixture(header []byte, frameSize int) []byte {
	raw := make([]byte, frameSize*2)
	copy(raw, header)
	copy(raw[frameSize:], header)
	return raw
}

func TestUntaggedMP3PreservesBytesAcrossImportAndUpload(t *testing.T) {
	previous := disk
	disk = libraryTestStore(t)
	defer func() { disk = previous }()
	mux := http.NewServeMux()
	registerLibraryMediaAPI(mux)
	for _, fixture := range []struct {
		name   string
		header []byte
		size   int
	}{
		{"mpeg1", []byte{0xff, 0xfb, 0x90, 0x00}, 417},
		{"mpeg2", []byte{0xff, 0xf3, 0x80, 0x00}, 208},
		{"mpeg2.5", []byte{0xff, 0xe3, 0x80, 0x00}, 417},
		{"padded", []byte{0xff, 0xfb, 0x92, 0x00}, 418},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			raw := rawMP3Fixture(fixture.header, fixture.size)
			id := hashContent(raw) + ".mp3"
			preview := &bankImportPreview{Stage: t.TempDir(), Media: map[string]bool{}}
			if err := preview.readMedia(bytes.NewReader(raw), int64(len(raw))); err != nil || !preview.Media[id] {
				t.Fatal("original MP3 rejected", err, preview.Media)
			}
			saved, err := os.ReadFile(filepath.Join(preview.Stage, "library", "media", id))
			if err != nil || !bytes.Equal(saved, raw) {
				t.Fatal("import rewrote audio", err)
			}
			request := httptest.NewRequest("POST", "/api/library/media", bytes.NewReader(raw))
			request.Header.Set("Content-Type", "audio/mpeg")
			request.Header.Set("X-ELP-Directory", disk.directoryIDLocked())
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			var result map[string]string
			json.Unmarshal(response.Body.Bytes(), &result)
			if response.Code != 201 || result["id"] != id {
				t.Fatal(response.Code, response.Body.String())
			}
			request = httptest.NewRequest("GET", "/api/library/media/"+id, nil)
			request.Header.Set("Range", "bytes=0-3")
			response = httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != 206 || !bytes.Equal(response.Body.Bytes(), raw[:4]) || response.Header().Get("Content-Type") != "audio/mpeg" {
				t.Fatal("MP3 range replay failed")
			}
		})
	}
}

func TestMP3DetectionRejectsFalseSyncAndInvalidHeaders(t *testing.T) {
	valid := rawMP3Fixture([]byte{0xff, 0xfb, 0x90, 0}, 417)
	for _, raw := range [][]byte{
		[]byte("<html>not audio</html>"), {0xff, 0xfb}, valid[:417],
		rawMP3Fixture([]byte{0xff, 0xeb, 0x90, 0}, 417),
		rawMP3Fixture([]byte{0xff, 0xfb, 0x00, 0}, 417),
		rawMP3Fixture([]byte{0xff, 0xfb, 0xfc, 0}, 417),
	} {
		if libraryMediaExtension(raw) != "" {
			t.Fatal("invalid untagged audio accepted")
		}
	}
	valid[418] = 0xf3
	if isUntaggedMP3(valid) {
		t.Fatal("inconsistent frame versions accepted")
	}
}

func TestLibraryZIPAggregateLimit(t *testing.T) {
	for _, item := range []struct {
		size     uint64
		accepted bool
	}{
		{300 << 20, true}, {maxLibraryImport, true}, {maxLibraryImport + 1, false}, {^uint64(0), false},
	} {
		var archive bytes.Buffer
		writer := zip.NewWriter(&archive)
		_, err := writer.CreateRaw(&zip.FileHeader{Name: "ignored.txt", Method: zip.Store, UncompressedSize64: item.size})
		if err != nil {
			t.Fatal(err)
		}
		if err = writer.Close(); err != nil {
			t.Fatal(err)
		}
		preview := &bankImportPreview{Stage: t.TempDir(), Media: map[string]bool{}}
		err = preview.readZIP(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
		if (err == nil) != item.accepted {
			t.Fatalf("size %d: %v", item.size, err)
		}
	}
}
