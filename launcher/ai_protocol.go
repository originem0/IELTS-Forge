package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// A model name never selects a protocol: relays often mix vendors on one endpoint.
type aiEndpoint struct {
	BaseURL        string `json:"baseUrl"`
	APIKey         string `json:"apiKey"`
	Model          string `json:"model"`
	Protocol       string `json:"protocol"`
	Provider       string `json:"provider,omitempty"`
	VisionVerified bool   `json:"visionVerified,omitempty"`
	Connected      bool   `json:"connected"`
}

func (e aiEndpoint) config() aiConfig {
	return aiConfig{BaseURL: e.BaseURL, APIKey: e.APIKey, Model: e.Model, Protocol: e.Protocol, Provider: e.Provider, VisionVerified: e.VisionVerified, Connected: e.Connected}
}
func protocolOf(c aiConfig) string {
	if c.Protocol == "" {
		return "openai"
	}
	return c.Protocol
}
func sameAIEndpoint(a, b aiConfig) bool {
	return strings.TrimRight(a.BaseURL, "/") == strings.TrimRight(b.BaseURL, "/") && protocolOf(a) == protocolOf(b)
}
func publicVision(e *aiEndpoint) any {
	if e == nil {
		return nil
	}
	return map[string]any{"baseUrl": e.BaseURL, "model": e.Model, "protocol": protocolOf(e.config()), "provider": e.Provider, "visionVerified": e.VisionVerified, "connected": e.Connected}
}
func validateAIEndpoint(c aiConfig) error {
	if len(c.BaseURL) > 2048 || len(c.APIKey) > 8192 {
		return errors.New("配置内容过长")
	}
	switch c.Provider {
	case "", "custom", "openai", "anthropic", "glm", "gemini", "grok", "deepseek":
	default:
		return errors.New("不支持的服务来源")
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("请填写不含账号、查询参数或片段的 http / https 接口地址")
	}
	if err := validateAITransport(u); err != nil {
		return err
	}
	switch protocolOf(c) {
	case "openai", "anthropic", "gemini":
	default:
		return errors.New("不支持的接口协议")
	}
	return nil
}
func aiURL(c aiConfig, resource string) string {
	base := strings.TrimRight(c.BaseURL, "/")
	// Accept a base URL or a copied chat endpoint without duplicating its suffix.
	for _, suffix := range []string{"/chat/completions", "/messages", "/models"} {
		base = strings.TrimSuffix(base, suffix)
	}
	u, _ := url.Parse(base)
	if u != nil && u.Path == "" {
		if protocolOf(c) == "gemini" {
			base += "/v1beta"
		} else {
			base += "/v1"
		}
	}
	return base + "/" + resource
}
func validateAITransport(u *url.URL) error {
	if u != nil && u.Scheme == "https" && u.Hostname() != "" {
		return nil
	}
	if u != nil && u.Scheme == "http" {
		host := u.Hostname()
		if strings.EqualFold(host, "localhost") || net.ParseIP(host).IsLoopback() {
			return nil
		}
	}
	return errors.New("远程 AI 接口必须使用 HTTPS；仅本机回环地址允许 HTTP")
}

func doAIRequest(r *http.Request) (*http.Response, error) {
	// Recheck persisted legacy configurations at the actual send boundary.
	if err := validateAITransport(r.URL); err != nil {
		return nil, err
	}
	// Custom key headers are not protected by Go's Authorization redirect rules.
	// Refuse redirects rather than leak either station's key to another origin.
	transport := *client
	transport.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return transport.Do(r)
}
func authAI(r *http.Request, c aiConfig) {
	r.Header.Set("Content-Type", "application/json")
	switch protocolOf(c) {
	case "anthropic":
		r.Header.Set("anthropic-version", "2023-06-01")
		if c.APIKey != "" {
			r.Header.Set("x-api-key", c.APIKey)
		}
	case "gemini":
		if c.APIKey != "" {
			r.Header.Set("x-goog-api-key", c.APIKey)
		}
	default:
		if c.APIKey != "" {
			r.Header.Set("Authorization", "Bearer "+c.APIKey)
		}
	}
}
func redactAIError(message string, c aiConfig) string {
	if c.APIKey != "" {
		message = strings.ReplaceAll(message, c.APIKey, "[已隐藏密钥]")
	}
	if len(message) > 1200 {
		message = message[:1200]
	}
	return message
}
func requestAIJSON(ctx context.Context, c aiConfig, method, target string, payload any) (map[string]any, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(raw)
	}
	r, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	authAI(r, c)
	response, err := doAIRequest(r)
	if err != nil {
		return nil, errors.New("无法连接模型服务，请检查地址和网络")
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxAIResponse+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxAIResponse {
		return nil, errors.New("模型服务响应过大")
	}
	var parsed map[string]any
	if json.Unmarshal(raw, &parsed) != nil {
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return nil, outputFormatError("模型服务返回了无法解析的响应")
		}
		return nil, fmt.Errorf("模型服务返回了无法解析的响应（HTTP %d），请检查接口协议和地址", response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if detail, ok := parsed["error"].(map[string]any); ok {
			if text, ok := detail["message"].(string); ok {
				return nil, errors.New(redactAIError(text, c))
			}
		}
		return nil, fmt.Errorf("模型服务返回 HTTP %d", response.StatusCode)
	}
	return parsed, nil
}
func imageSource(src string, gemini bool) (map[string]any, error) {
	if strings.HasPrefix(src, "data:") {
		metadata, data, ok := strings.Cut(strings.TrimPrefix(src, "data:"), ",")
		if !ok || !strings.HasSuffix(metadata, ";base64") {
			return nil, errors.New("图片数据格式无效")
		}
		mime := strings.TrimSuffix(metadata, ";base64")
		if mime == "image/jpg" {
			mime = "image/jpeg"
		}
		if gemini {
			return map[string]any{"inlineData": map[string]string{"mimeType": mime, "data": data}}, nil
		}
		return map[string]any{"type": "image", "source": map[string]string{"type": "base64", "media_type": mime, "data": data}}, nil
	}
	if gemini {
		return nil, errors.New("Gemini 原生接口需要实际图片数据，请上传题图后重试")
	}
	return map[string]any{"type": "image", "source": map[string]string{"type": "url", "url": src}}, nil
}
func nativeParts(content any, gemini bool) ([]any, error) {
	if text, ok := content.(string); ok {
		if gemini {
			return []any{map[string]string{"text": text}}, nil
		}
		return []any{map[string]string{"type": "text", "text": text}}, nil
	}
	var result []any
	parts, _ := content.([]any)
	for _, raw := range parts {
		p, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		switch p["type"] {
		case "text":
			text, _ := p["text"].(string)
			if gemini {
				result = append(result, map[string]string{"text": text})
			} else {
				result = append(result, map[string]string{"type": "text", "text": text})
			}
		case "image_url":
			image, _ := p["image_url"].(map[string]any)
			src, _ := image["url"].(string)
			part, err := imageSource(src, gemini)
			if err != nil {
				return nil, err
			}
			result = append(result, part)
		}
	}
	return result, nil
}
func callNativeChat(ctx context.Context, c aiConfig, messages []chatMessage, temperature float64, maxTokens int) (string, error) {
	gemini := protocolOf(c) == "gemini"
	var system []any
	var conversation []any
	for _, message := range messages {
		parts, err := nativeParts(message.Content, gemini)
		if err != nil {
			return "", err
		}
		if message.Role == "system" {
			system = append(system, parts...)
			continue
		}
		role := message.Role
		if gemini {
			if role == "assistant" {
				role = "model"
			}
			conversation = append(conversation, map[string]any{"role": role, "parts": parts})
		} else {
			conversation = append(conversation, map[string]any{"role": role, "content": parts})
		}
	}
	payload := map[string]any{"model": c.Model, "messages": conversation, "max_tokens": maxTokens}
	if len(system) > 0 {
		payload["system"] = system
	}
	resource := "messages"
	if gemini {
		payload = map[string]any{"contents": conversation, "generationConfig": map[string]any{"maxOutputTokens": maxTokens}}
		if len(system) > 0 {
			payload["systemInstruction"] = map[string]any{"parts": system}
		}
		resource = "models/" + url.PathEscape(strings.TrimPrefix(c.Model, "models/")) + ":generateContent"
	}
	// Leave temperature at provider defaults: some newer models reject overrides.
	parsed, err := requestAIJSON(ctx, c, http.MethodPost, aiURL(c, resource), payload)
	if err != nil {
		return "", err
	}
	var texts []string
	if gemini {
		candidates, _ := parsed["candidates"].([]any)
		if len(candidates) > 0 {
			first, _ := candidates[0].(map[string]any)
			reason, _ := first["finishReason"].(string)
			if reason == "MAX_TOKENS" {
				return "", outputFormatError("模型输出达到长度上限，请更换模型或缩短输入")
			}
			if reason != "" && reason != "STOP" {
				return "", fmt.Errorf("模型没有完成回答（%s）", reason)
			}
			content, _ := first["content"].(map[string]any)
			parts, _ := content["parts"].([]any)
			for _, p := range parts {
				part, ok := p.(map[string]any)
				if !ok || part["thought"] == true {
					continue
				}
				if s, ok := part["text"].(string); ok {
					texts = append(texts, s)
				}
			}
		}
	} else {
		if parsed["stop_reason"] == "max_tokens" {
			return "", outputFormatError("模型输出达到长度上限，请更换模型或缩短输入")
		}
		if reason, _ := parsed["stop_reason"].(string); reason != "" && reason != "end_turn" && reason != "stop_sequence" {
			return "", fmt.Errorf("模型没有完成回答（%s）", reason)
		}
		parts, _ := parsed["content"].([]any)
		for _, p := range parts {
			part, ok := p.(map[string]any)
			if !ok || part["type"] != "text" {
				continue
			}
			if s, ok := part["text"].(string); ok {
				texts = append(texts, s)
			}
		}
	}
	text := strings.TrimSpace(strings.Join(texts, "\n"))
	if text == "" {
		return "", errors.New("模型没有返回可用的文字回答，可能受到额度、内容或模型能力限制")
	}
	return text, nil
}

func handleAIModels(w http.ResponseWriter, r *http.Request) {
	var input struct {
		BaseURL  string `json:"baseUrl"`
		APIKey   string `json:"apiKey"`
		Protocol string `json:"protocol"`
		Purpose  string `json:"purpose"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if input.Purpose != "" && input.Purpose != "text" && input.Purpose != "vision" {
		writeError(w, 400, "接口用途无效")
		return
	}
	c := aiConfig{BaseURL: strings.TrimRight(strings.TrimSpace(input.BaseURL), "/"), APIKey: input.APIKey, Protocol: input.Protocol}
	if err := validateAIEndpoint(c); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	settings.RLock()
	saved := settings.value
	if input.Purpose == "vision" {
		saved = aiConfig{}
		if settings.value.Vision != nil {
			saved = settings.value.Vision.config()
		}
	}
	settings.RUnlock()
	if c.APIKey == "" && sameAIEndpoint(c, saved) {
		c.APIKey = saved.APIKey
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	models, err := discoverModels(ctx, c)
	if err != nil {
		writeError(w, 502, "获取模型失败："+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"models": models})
}

type aiModel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func discoverModels(ctx context.Context, c aiConfig) ([]aiModel, error) {
	result := []aiModel{}
	seen := map[string]bool{}
	cursors := map[string]bool{}
	cursor := ""
	for page := 0; page < 20; page++ {
		u, _ := url.Parse(aiURL(c, "models"))
		query := u.Query()
		switch protocolOf(c) {
		case "gemini":
			query.Set("pageSize", "1000")
			if cursor != "" {
				query.Set("pageToken", cursor)
			}
		case "anthropic":
			query.Set("limit", "1000")
			if cursor != "" {
				query.Set("after_id", cursor)
			}
		}
		u.RawQuery = query.Encode()
		parsed, err := requestAIJSON(ctx, c, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		field := "data"
		if protocolOf(c) == "gemini" {
			field = "models"
		}
		entries, ok := parsed[field].([]any)
		if !ok {
			return nil, errors.New("该地址没有返回模型列表，请检查协议或向中转站确认模型列表接口")
		}
		for _, raw := range entries {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			id, _ := item["id"].(string)
			name, _ := item["display_name"].(string)
			if protocolOf(c) == "gemini" {
				id, _ = item["name"].(string)
				id = strings.TrimPrefix(id, "models/")
				name, _ = item["displayName"].(string)
				methods, hasMethods := item["supportedGenerationMethods"].([]any)
				supported := !hasMethods
				for _, m := range methods {
					if m == "generateContent" {
						supported = true
					}
				}
				if !supported {
					continue
				}
			}
			if strings.TrimSpace(id) == "" || len(id) > 200 || strings.ContainsAny(id, "\r\n\x00") || seen[id] {
				continue
			}
			seen[id] = true
			if name == "" {
				name = id
			}
			result = append(result, aiModel{ID: id, Name: name})
		}
		cursor = ""
		switch protocolOf(c) {
		case "gemini":
			cursor, _ = parsed["nextPageToken"].(string)
		case "anthropic":
			if parsed["has_more"] == true {
				cursor, _ = parsed["last_id"].(string)
				if cursor == "" {
					return nil, errors.New("模型列表分页信息缺失")
				}
			}
		}
		if cursor == "" {
			if len(result) == 0 {
				return nil, errors.New("没有可用的对话模型，请检查 Key 权限或账户额度")
			}
			return result, nil
		}
		if cursors[cursor] {
			return nil, errors.New("模型列表分页重复，请联系服务商")
		}
		cursors[cursor] = true
	}
	return nil, errors.New("模型列表分页过多，请联系服务商缩小可用范围")
}

func handleVisionConfig(w http.ResponseWriter, r *http.Request) {
	var c aiConfig
	if err := decodeJSON(w, r, &c); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	c.Vision = nil
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	c.Model = strings.TrimSpace(c.Model)
	c.Protocol = protocolOf(c)
	settings.RLock()
	if c.APIKey == "" && settings.value.Vision != nil && sameAIEndpoint(c, settings.value.Vision.config()) {
		c.APIKey = settings.value.Vision.APIKey
	}
	settings.RUnlock()
	if err := validateConfig(c); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if err := probeVision(r.Context(), c); err != nil {
		writeError(w, 502, "图片接口测试失败："+err.Error())
		return
	}
	c.Connected = true
	settings.Lock()
	defer settings.Unlock()
	updated := settings.value
	updated.Vision = &aiEndpoint{BaseURL: c.BaseURL, APIKey: c.APIKey, Model: c.Model, Protocol: c.Protocol, Provider: c.Provider, VisionVerified: true, Connected: true}
	if err := settings.persist(updated); err != nil {
		writeError(w, 500, "图片接口测试成功，但无法加密保存配置")
		return
	}
	settings.value = updated
	settings.saved = true
	settings.loadError = ""
	writeJSON(w, 200, map[string]any{"saved": true, "model": c.Model, "connected": true})
}
func handleVisionDisconnect(w http.ResponseWriter, r *http.Request) {
	settings.Lock()
	defer settings.Unlock()
	updated := settings.value
	updated.Vision = nil
	if !updated.Connected {
		if err := os.Remove(settings.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			writeError(w, 500, "无法删除图片接口配置")
			return
		}
		settings.value = aiConfig{}
		settings.saved, settings.restored, settings.loadError = false, false, ""
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	if err := settings.persist(updated); err != nil {
		writeError(w, 500, "无法删除图片接口配置")
		return
	}
	settings.value = updated
	settings.restored = false
	settings.loadError = ""
	writeJSON(w, 200, map[string]bool{"ok": true})
}
