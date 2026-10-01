package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func transportAI(t *testing.T, handler func(*http.Request) (int, string)) {
	t.Helper()
	previous := client
	client = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		code, body := handler(r)
		return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	t.Cleanup(func() { client = previous })
}
func aiBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}
func imageMessages() []chatMessage {
	return []chatMessage{{Role: "system", Content: "Review the chart."}, {Role: "user", Content: []any{map[string]any{"type": "text", "text": "My answer"}, map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,AAAA"}}}}}
}

func TestNativeChatProtocolsPreserveImagesAndHideReasoning(t *testing.T) {
	for _, protocol := range []string{"anthropic", "gemini"} {
		t.Run(protocol, func(t *testing.T) {
			transportAI(t, func(r *http.Request) (int, string) {
				b := aiBody(t, r)
				if r.Header.Get("Authorization") != "" {
					t.Fatal("native request leaked Bearer header")
				}
				if protocol == "anthropic" {
					if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "native-key" || r.Header.Get("anthropic-version") == "" {
						t.Fatal("bad Claude endpoint/auth")
					}
					if b["system"] == nil {
						t.Fatal("missing system prompt")
					}
					messages := b["messages"].([]any)
					parts := messages[0].(map[string]any)["content"].([]any)
					source := parts[1].(map[string]any)["source"].(map[string]any)
					if source["data"] != "AAAA" || source["media_type"] != "image/png" {
						t.Fatal("Claude image changed")
					}
					return 200, `{"content":[{"type":"thinking","thinking":"private"},{"type":"text","text":"Review result"}],"stop_reason":"end_turn"}`
				}
				if r.URL.Path != "/v1beta/models/test-model:generateContent" || r.Header.Get("x-goog-api-key") != "native-key" || r.URL.RawQuery != "" {
					t.Fatal("bad Gemini endpoint/auth")
				}
				if b["systemInstruction"] == nil {
					t.Fatal("missing Gemini system prompt")
				}
				parts := b["contents"].([]any)[0].(map[string]any)["parts"].([]any)
				source := parts[1].(map[string]any)["inlineData"].(map[string]any)
				if source["data"] != "AAAA" || source["mimeType"] != "image/png" {
					t.Fatal("Gemini image changed")
				}
				return 200, `{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"private","thought":true},{"text":"Review result"}]}}]}`
			})
			result, err := callChat(context.Background(), aiConfig{BaseURL: "https://native.test", Protocol: protocol, APIKey: "native-key", Model: "test-model"}, imageMessages(), 0, 6000, false)
			if err != nil || result != "Review result" {
				t.Fatalf("result=%q error=%v", result, err)
			}
		})
	}
}

func TestRelayModelVendorDoesNotChangeProtocol(t *testing.T) {
	for _, model := range []string{"gpt-custom", "claude-custom", "glm-custom", "gemini-custom", "grok-custom", "deepseek-custom"} {
		t.Run(model, func(t *testing.T) {
			transportAI(t, func(r *http.Request) (int, string) {
				if r.URL.Path != "/gateway/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer relay-key" || r.Header.Get("x-api-key") != "" {
					t.Fatal("relay protocol changed")
				}
				body := aiBody(t, r)
				if body["model"] != model {
					t.Fatal("model alias changed")
				}
				return 200, `{"choices":[{"message":{"content":"ok"}}]}`
			})
			_, err := callChat(context.Background(), aiConfig{BaseURL: "https://relay.test/gateway/v1", APIKey: "relay-key", Model: model}, nil, 0, 256, true)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestModelsPaginationFilteringAndDeduplication(t *testing.T) {
	for _, protocol := range []string{"openai", "anthropic", "gemini"} {
		t.Run(protocol, func(t *testing.T) {
			calls := 0
			transportAI(t, func(r *http.Request) (int, string) {
				calls++
				if r.Method != "GET" {
					t.Fatal("discovery must use GET")
				}
				if !strings.HasSuffix(r.URL.Path, "/models") {
					t.Fatal(r.URL)
				}
				if protocol == "openai" {
					return 200, `{"data":[{"id":"claude-relay"},{"id":"grok-relay"},{"id":"claude-relay"},{"id":17}]}`
				}
				if protocol == "anthropic" {
					if calls == 1 {
						return 200, `{"data":[{"id":"first","display_name":"First"}],"has_more":true,"last_id":"first"}`
					}
					if r.URL.Query().Get("after_id") != "first" {
						t.Fatal("Claude pagination missing")
					}
					return 200, `{"data":[{"id":"second"}],"has_more":false}`
				}
				if calls == 1 {
					return 200, `{"models":[{"name":"models/first","supportedGenerationMethods":["generateContent"]},{"name":"models/embed","supportedGenerationMethods":["embedContent"]}],"nextPageToken":"next"}`
				}
				if r.URL.Query().Get("pageToken") != "next" {
					t.Fatal("Gemini pagination missing")
				}
				return 200, `{"models":[{"name":"models/second","supportedGenerationMethods":["generateContent"]}]}`
			})
			models, err := discoverModels(context.Background(), aiConfig{BaseURL: "https://models.test", Protocol: protocol})
			if err != nil || len(models) != 2 {
				t.Fatalf("models=%v err=%v", models, err)
			}
			if protocol != "openai" && models[0].ID != "first" {
				t.Fatal("native model ID not normalized")
			}
		})
	}
}

func TestModelDiscoveryKeysAreScopedToPurposeEndpointAndProtocol(t *testing.T) {
	previous := settings
	settings = &configStore{value: aiConfig{BaseURL: "https://text.test/v1", APIKey: "text-secret", Model: "text", Vision: &aiEndpoint{BaseURL: "https://image.test/v1", APIKey: "image-secret", Model: "image"}}}
	t.Cleanup(func() { settings = previous })
	cases := []struct{ body, expected string }{
		{`{"baseUrl":"https://text.test/v1","purpose":"text"}`, "text-secret"},
		{`{"baseUrl":"https://image.test/v1","purpose":"vision"}`, "image-secret"},
		{`{"baseUrl":"https://text.test/v1","purpose":"vision"}`, ""},
		{`{"baseUrl":"https://evil.test/v1","purpose":"text"}`, ""},
		{`{"baseUrl":"https://text.test/v1","protocol":"anthropic"}`, ""},
	}
	for _, item := range cases {
		t.Run(item.body, func(t *testing.T) {
			transportAI(t, func(r *http.Request) (int, string) {
				key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
				if key != item.expected || r.Header.Get("x-api-key") != "" {
					t.Fatal("wrong credential reused")
				}
				return 200, `{"data":[{"id":"model"}]}`
			})
			w := httptest.NewRecorder()
			handleAIModels(w, httptest.NewRequest("POST", "/api/ai/models", strings.NewReader(item.body)))
			if w.Code != 200 || strings.Contains(w.Body.String(), "secret") {
				t.Fatalf("unexpected discovery response %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestSeparateImageRoutingAndNoSilentFallback(t *testing.T) {
	previous := settings
	settings = &configStore{value: aiConfig{BaseURL: "https://text.test/v1", APIKey: "text-key", Model: "text-model", Connected: true, Vision: &aiEndpoint{BaseURL: "https://image.test/v1", APIKey: "image-key", Model: "image-model", Connected: true, VisionVerified: true}}}
	t.Cleanup(func() { settings = previous })
	count := 0
	transportAI(t, func(r *http.Request) (int, string) {
		count++
		if r.URL.Host != "image.test" || r.Header.Get("Authorization") != "Bearer image-key" {
			t.Fatal("wrong image endpoint/key")
		}
		body := aiBody(t, r)
		if body["model"] != "image-model" {
			t.Fatal("wrong image model")
		}
		return 400, `{"error":{"message":"image input unsupported"}}`
	})
	raw, _ := json.Marshal(chatRequest{Messages: imageMessages()})
	w := httptest.NewRecorder()
	handleAIChat(w, httptest.NewRequest("POST", "/api/ai/chat", bytesReader(raw)))
	if w.Code != 502 || count != 1 || !strings.Contains(w.Body.String(), "图片批改失败") {
		t.Fatalf("fallback or missing error: %d %s calls=%d", w.Code, w.Body.String(), count)
	}
	if settings.value.Model != "text-model" || settings.value.APIKey != "text-key" {
		t.Fatal("image request changed text settings")
	}
}
func bytesReader(b []byte) io.Reader { return strings.NewReader(string(b)) }

func TestVisionCredentialsPersistAndDeleteIndependently(t *testing.T) {
	previous := settings
	settings = &configStore{path: filepath.Join(t.TempDir(), "ai.dpapi"), value: aiConfig{BaseURL: "https://text.test", APIKey: "text-secret", Model: "text", Connected: true}}
	t.Cleanup(func() { settings = previous })
	transportAI(t, func(r *http.Request) (int, string) {
		body := aiBody(t, r)
		return 200, visionReply("openai", readProbePixels(t, body, "openai"))
	})
	w := httptest.NewRecorder()
	handleVisionConfig(w, httptest.NewRequest("POST", "/api/ai/vision/config", strings.NewReader(`{"baseUrl":"https://image.test/v1","apiKey":"image-secret","model":"vision"}`)))
	if w.Code != 200 {
		t.Fatalf("vision config: %d %s", w.Code, w.Body.String())
	}
	encrypted, err := os.ReadFile(settings.path)
	if err != nil || strings.Contains(string(encrypted), "secret") {
		t.Fatal("credential persistence failed", err)
	}
	restored := &configStore{path: settings.path}
	restored.restore()
	if restored.value.APIKey != "text-secret" || restored.value.Vision.APIKey != "image-secret" {
		t.Fatal("credentials not restored")
	}
	w = httptest.NewRecorder()
	handleAIStatus(w, httptest.NewRequest("GET", "/api/ai/status", nil))
	if strings.Contains(w.Body.String(), "secret") {
		t.Fatal("status exposed credentials")
	}
	w = httptest.NewRecorder()
	handleAIDisconnect(w, httptest.NewRequest("POST", "/api/ai/disconnect", nil))
	if w.Code != 200 || settings.value.Vision.APIKey != "image-secret" || settings.value.APIKey != "" {
		t.Fatal("text removal deleted vision")
	}
	restored.restore()
	if restored.value.Vision == nil || !restored.value.Vision.Connected || restored.value.Connected {
		t.Fatal("image-only config did not restore")
	}
	w = httptest.NewRecorder()
	handleVisionDisconnect(w, httptest.NewRequest("POST", "/api/ai/vision/disconnect", nil))
	if w.Code != 200 || settings.value.Vision != nil {
		t.Fatal("vision removal failed")
	}
}

func TestAIKeysNeverFollowRedirects(t *testing.T) {
	received := false
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received = true }))
	defer other.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer first.Close()
	previous := client
	client = first.Client()
	t.Cleanup(func() { client = previous })
	_, err := discoverModels(context.Background(), aiConfig{BaseURL: first.URL, APIKey: "private", Protocol: "anthropic"})
	if err == nil || received {
		t.Fatal("redirect exposed API credentials")
	}
}
