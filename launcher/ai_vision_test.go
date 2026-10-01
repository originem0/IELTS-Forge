package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The fake upstream must read the actual PNG, just as a vision model would.
func readProbePixels(t *testing.T, body map[string]any, protocol string) string {
	t.Helper()
	var data, prompt string
	if protocol == "gemini" {
		parts := body["contents"].([]any)[0].(map[string]any)["parts"].([]any)
		prompt = parts[0].(map[string]any)["text"].(string)
		data = parts[1].(map[string]any)["inlineData"].(map[string]any)["data"].(string)
	} else {
		parts := body["messages"].([]any)[0].(map[string]any)["content"].([]any)
		prompt = parts[0].(map[string]any)["text"].(string)
		if protocol == "anthropic" {
			data = parts[1].(map[string]any)["source"].(map[string]any)["data"].(string)
		} else {
			data = strings.SplitN(parts[1].(map[string]any)["image_url"].(map[string]any)["url"].(string), ",", 2)[1]
		}
	}
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		t.Fatal(err)
	}
	picture, err := png.Decode(bytesReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if picture.Bounds().Dx() != 496 || picture.Bounds().Dy() != 96 {
		t.Fatal("invalid probe size")
	}
	var colors []string
	for i := 0; i < 6; i++ {
		r, g, b, _ := picture.At(48+i*80, 48).RGBA()
		switch {
		case r == 65535 && g == 0 && b == 0:
			colors = append(colors, "red")
		case r == 0 && g > 0 && b == 0:
			colors = append(colors, "green")
		case r == 0 && g == 0 && b == 65535:
			colors = append(colors, "blue")
		case r == 65535 && g == 65535 && b == 0:
			colors = append(colors, "yellow")
		default:
			t.Fatal("unexpected probe pixel")
		}
	}
	answer := strings.Join(colors, " ")
	if strings.Contains(prompt, answer) {
		t.Fatal("probe reveals its answer without the image")
	}
	return answer
}

func visionReply(protocol, answer string) string {
	text, _ := json.Marshal(answer)
	switch protocol {
	case "anthropic":
		return `{"content":[{"type":"text","text":` + string(text) + `}],"stop_reason":"end_turn"}`
	case "gemini":
		return `{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":` + string(text) + `}]}}]}`
	default:
		return `{"choices":[{"finish_reason":"stop","message":{"content":` + string(text) + `}}]}`
	}
}

func TestVisionProbeRequiresReadingPixelsAcrossProtocols(t *testing.T) {
	for _, protocol := range []string{"openai", "anthropic", "gemini"} {
		t.Run(protocol, func(t *testing.T) {
			transportAI(t, func(r *http.Request) (int, string) {
				answer := readProbePixels(t, aiBody(t, r), protocol)
				return 200, visionReply(protocol, strings.ToUpper(strings.ReplaceAll(answer, " ", ", "))+".")
			})
			if err := probeVision(context.Background(), aiConfig{BaseURL: "https://vision.test", Protocol: protocol, Model: "exact-choice"}); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, answer := range []string{"CONNECTED", "UNABLE", "purple purple purple purple purple purple"} {
		t.Run(answer, func(t *testing.T) {
			transportAI(t, func(r *http.Request) (int, string) { return 200, visionReply("openai", answer) })
			if err := probeVision(context.Background(), aiConfig{BaseURL: "https://vision.test", Model: "text-only"}); err == nil {
				t.Fatal("accepted a model that ignored the image")
			}
		})
	}
}

func TestTextImageFallbackVerifiesOnceAndRestores(t *testing.T) {
	previous := settings
	settings = &configStore{path: filepath.Join(t.TempDir(), "ai.dpapi"), value: aiConfig{BaseURL: "https://relay.test/v1", APIKey: "text-secret", Model: "selected-alias", Provider: "custom", Connected: true}}
	t.Cleanup(func() { settings = previous })
	calls, probes := 0, 0
	transportAI(t, func(r *http.Request) (int, string) {
		calls++
		body := aiBody(t, r)
		if r.URL.Host != "relay.test" || r.Header.Get("Authorization") != "Bearer text-secret" || body["model"] != "selected-alias" {
			t.Fatal("fallback changed endpoint, key or model")
		}
		if len(body["messages"].([]any)) == 1 {
			probes++
			return 200, visionReply("openai", readProbePixels(t, body, "openai"))
		}
		expected, _ := json.Marshal(imageMessages())
		var decoded any
		if err := json.Unmarshal(expected, &decoded); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(decoded, body["messages"]) {
			t.Fatal("fallback dropped or changed original text/images")
		}
		return 200, visionReply("openai", "review")
	})
	raw, _ := json.Marshal(chatRequest{Messages: imageMessages()})
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		handleAIChat(w, httptest.NewRequest("POST", "/api/ai/chat", bytesReader(raw)))
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		settings.restore()
		if !settings.value.VisionVerified || settings.value.Vision != nil {
			t.Fatal("fallback verification did not persist independently")
		}
	}
	if probes != 1 || calls != 3 {
		t.Fatalf("probes=%d calls=%d", probes, calls)
	}
}

func TestFailedFallbackDoesNotSendPracticeOrBlockText(t *testing.T) {
	previous := settings
	settings = &configStore{path: filepath.Join(t.TempDir(), "ai.dpapi"), value: aiConfig{BaseURL: "https://text.test", Model: "text-only", Connected: true}}
	t.Cleanup(func() { settings = previous })
	calls := 0
	transportAI(t, func(r *http.Request) (int, string) {
		calls++
		body := aiBody(t, r)
		messages := body["messages"].([]any)
		if len(messages) != 1 {
			t.Fatal("sent practice despite failed verification")
		}
		return 200, visionReply("openai", "CONNECTED")
	})
	raw, _ := json.Marshal(chatRequest{Messages: imageMessages()})
	w := httptest.NewRecorder()
	handleAIChat(w, httptest.NewRequest("POST", "/api/ai/chat", bytesReader(raw)))
	if w.Code != 502 || !strings.Contains(w.Body.String(), "当前使用文字模型 text-only") || calls != 1 || settings.value.VisionVerified {
		t.Fatalf("unexpected fallback: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	handleAIChat(w, httptest.NewRequest("POST", "/api/ai/chat", strings.NewReader(`{"messages":[{"role":"user","content":"text practice"}]}`)))
	if w.Code != 200 || calls != 2 {
		t.Fatal("text practice was blocked by vision failure")
	}
}

func TestVisionVerificationCannotBeSuppliedByClientAndTracksModel(t *testing.T) {
	for _, change := range []string{"unchanged", "model", "baseUrl", "apiKey", "protocol"} {
		t.Run(change, func(t *testing.T) {
			previous := settings
			settings = &configStore{path: filepath.Join(t.TempDir(), "ai.dpapi"), value: aiConfig{BaseURL: "https://text.test/v1", Model: "old", APIKey: "old-key", Connected: true, VisionVerified: true}}
			t.Cleanup(func() { settings = previous })
			proposed := settings.value
			switch change {
			case "model":
				proposed.Model = "new"
			case "baseUrl":
				proposed.BaseURL = "https://changed.test/v1"
			case "apiKey":
				proposed.APIKey = "new-key"
			case "protocol":
				proposed.Protocol = "anthropic"
			}
			transportAI(t, func(r *http.Request) (int, string) { return 200, visionReply(protocolOf(proposed), "CONNECTED") })
			raw, _ := json.Marshal(proposed)
			w := httptest.NewRecorder()
			handleAIConfig(w, httptest.NewRequest("POST", "/api/ai/config", bytesReader(raw)))
			if w.Code != 200 || settings.value.VisionVerified != (change == "unchanged") {
				t.Fatalf("untrusted or stale verification saved: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestFailedVisionSavePreservesBothConfigurations(t *testing.T) {
	previous := settings
	settings = &configStore{path: filepath.Join(t.TempDir(), "ai.dpapi"), value: aiConfig{BaseURL: "https://text.test", Model: "text", Connected: true, Vision: &aiEndpoint{BaseURL: "https://image.test", Model: "good-image", Provider: "custom", Connected: true, VisionVerified: true}}}
	t.Cleanup(func() { settings = previous })
	if err := settings.persist(settings.value); err != nil {
		t.Fatal(err)
	}
	before := settings.value
	fileBefore, _ := os.ReadFile(settings.path)
	transportAI(t, func(r *http.Request) (int, string) { return 200, visionReply("openai", "CONNECTED") })
	w := httptest.NewRecorder()
	handleVisionConfig(w, httptest.NewRequest("POST", "/api/ai/vision/config", strings.NewReader(`{"baseUrl":"https://other.test","model":"cannot-see","visionVerified":true}`)))
	fileAfter, _ := os.ReadFile(settings.path)
	if w.Code != 502 || !reflect.DeepEqual(settings.value, before) || string(fileAfter) != string(fileBefore) {
		t.Fatal("failed image verification overwrote working settings")
	}
}
