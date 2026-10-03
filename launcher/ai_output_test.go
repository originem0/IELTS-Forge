package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAIChatOutputRetryAndFailureBoundaries(t *testing.T) {
	for _, test := range []struct {
		name          string
		fail          string
		recover       bool
		calls, status int
	}{
		{"format-recovery", "format", true, 2, 200},
		{"format-exhausted", "format", false, 2, 502},
		{"malformed-recovery", "malformed", true, 2, 200},
		{"truncated-recovery", "length", true, 2, 200},
		{"truncated-exhausted", "length", false, 2, 502},
		{"authentication", "auth", false, 1, 502},
		{"refusal", "refusal", false, 1, 502},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				if test.fail == "malformed" && calls == 1 {
					w.Write([]byte("{broken"))
					return
				}
				if test.fail == "auth" {
					w.WriteHeader(401)
					w.Write([]byte(`{"error":{"message":"unauthorized"}}`))
					return
				}
				content := `{"summary":"可复用表达","speaking":[],"writing":[{"domain":"教育","collocations":["equal access｜平等机会"],"sentencePatterns":[],"sourceKeys":["fixture"]}]}`
				finish := "stop"
				if calls == 1 || !test.recover {
					switch test.fail {
					case "format":
						content = "wrong format"
					case "length":
						finish = "length"
					case "refusal":
						finish = "content_filter"
					}
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": finish, "message": map[string]string{"content": content}}}})
			}))
			defer upstream.Close()
			previous := settings
			settings = &configStore{value: aiConfig{BaseURL: upstream.URL, Model: "fixture", Connected: true}}
			defer func() { settings = previous }()
			r := httptest.NewRequest("POST", "/api/ai/chat", strings.NewReader(`{"messages":[{"role":"user","content":"fixture"}],"output_contract":"personal-language-bank-json-v1"}`))
			response := httptest.NewRecorder()
			handleAIChat(response, r)
			if calls != test.calls || response.Code != test.status {
				t.Fatalf("calls=%d status=%d body=%s", calls, response.Code, response.Body.String())
			}
			if response.Code != 200 && strings.Contains(response.Body.String(), "equal access") {
				t.Fatal("failed response exposed partial result")
			}
		})
	}
}
