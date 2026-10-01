package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	maxRequest      = 1 << 20
	maxAIRequest    = 48 << 20
	maxAIResponse   = 4 << 20
	maxDataFile     = 128 << 20
	dataFilename    = "EnglishLearnPath-data.json"
	backupName      = "EnglishLearnPath-data.backup.json"
	configDirname   = "runtime-data"
	dailyBackupKeep = 30
)

type aiConfig struct {
	BaseURL        string      `json:"baseUrl"`
	APIKey         string      `json:"apiKey"`
	Model          string      `json:"model"`
	Protocol       string      `json:"protocol,omitempty"`
	Provider       string      `json:"provider,omitempty"`
	VisionVerified bool        `json:"visionVerified,omitempty"`
	Connected      bool        `json:"connected"`
	Vision         *aiEndpoint `json:"vision,omitempty"`
}

type configStore struct {
	sync.RWMutex
	value     aiConfig
	path      string
	saved     bool
	restored  bool
	loadError string
}

type diskDataEnvelope struct {
	Version   int             `json:"version"`
	UpdatedAt string          `json:"updatedAt"`
	Data      json.RawMessage `json:"data"`
}

type launcherConfig struct {
	DataDirectory string `json:"dataDirectory"`
}

type diskStore struct {
	sync.RWMutex
	directory       string
	configPath      string
	lastWrite       string
	strictDirectory bool
	directoryLock   *os.File
	libraryIndex    *libraryReadIndex
	studyWrites     int
}

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type chatRequest struct {
	Messages       []chatMessage `json:"messages"`
	Temperature    float64       `json:"temperature,omitempty"`
	MaxTokens      int           `json:"max_tokens,omitempty"`
	OutputContract string        `json:"output_contract,omitempty"`
}

type upstreamResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content          any    `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

var (
	settings     = &configStore{}
	client       = &http.Client{Timeout: 90 * time.Second}
	disk         *diskStore
	appVersion   = "dev"
	httpServer   *http.Server
	shutdownOnce sync.Once
)

func main() {
	appDir, err := findResourceDir("app")
	if err != nil {
		writeStartupError(err)
		return
	}
	docsDir, _ := findResourceDir("docs")
	disk, err = newDiskStore()
	if err != nil {
		writeStartupError(fmt.Errorf("无法初始化永久数据目录：%w", err))
		return
	}
	defer disk.close()
	settings.path = filepath.Join(filepath.Dir(disk.configPath), credentialFilename)
	settings.restore()

	// Let Windows assign a free loopback port. A fixed port can make a newly
	// extracted copy open an older copy that is already running, which would
	// expose that copy's local settings in the browser.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		writeStartupError(fmt.Errorf("无法启动本地服务：%w", err))
		return
	}
	listenAddr := listener.Addr().String()
	appURL := "http://" + listenAddr

	mux := http.NewServeMux()
	registerAPI(mux)
	if docsDir != "" {
		mux.Handle("/docs/", http.StripPrefix("/docs/", http.FileServer(http.Dir(docsDir))))
	}
	mux.Handle("/", secureStaticServer(appDir))

	server := &http.Server{
		Handler:           securityHeaders(guardLocalRequests(listenAddr, mux)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	httpServer = server

	// Packaged smoke tests use an isolated browser without opening the user's tabs.
	if os.Getenv("ENGLISH_LEARN_PATH_NO_BROWSER") != "1" {
		go func() {
			time.Sleep(350 * time.Millisecond)
			_ = openBrowser(appURL)
		}()
	}

	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		writeStartupError(fmt.Errorf("本地服务意外停止：%w", err))
	}
}

// beginShutdown drains in-flight requests (e.g. a data save that is still
// writing) for a short grace period before exiting, so the user's last edit is
// not truncated. Guarded by sync.Once against repeated shutdown calls.
func beginShutdown() {
	shutdownOnce.Do(func() {
		time.Sleep(150 * time.Millisecond) // let the ok response flush to the browser
		if httpServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = httpServer.Shutdown(ctx)
		}
		os.Exit(0)
	})
}

func registerAPI(mux *http.ServeMux) {
	registerStudyAPI(mux)
	registerLibraryAPI(mux)
	mux.HandleFunc("GET /api/app/info", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"name": "English Learning Path", "version": appVersion, "local": true})
	})
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "app": "EnglishLearnPath"})
	})
	mux.HandleFunc("POST /api/app/shutdown", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		go beginShutdown()
	})
	mux.HandleFunc("GET /api/data/status", handleDataStatus)
	mux.HandleFunc("GET /api/data", handleDataLoad)
	mux.HandleFunc("PUT /api/data", handleDataSave)
	mux.HandleFunc("POST /api/data/select-directory", handleDataSelectDirectory)
	mux.HandleFunc("POST /api/data/open-directory", handleDataOpenDirectory)
	mux.HandleFunc("GET /api/ai/status", handleAIStatus)
	mux.HandleFunc("POST /api/ai/config", handleAIConfig)
	mux.HandleFunc("POST /api/ai/models", handleAIModels)
	mux.HandleFunc("POST /api/ai/vision/config", handleVisionConfig)
	mux.HandleFunc("POST /api/ai/vision/disconnect", handleVisionDisconnect)
	mux.HandleFunc("POST /api/ai/disconnect", handleAIDisconnect)
	mux.HandleFunc("POST /api/ai/chat", handleAIChat)
	mux.HandleFunc("GET /api/transcription/status", handleTranscriptionStatus)
	mux.HandleFunc("POST /api/transcription", handleTranscription)
}

func handleDataStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, disk.status())
}

func handleDataLoad(w http.ResponseWriter, _ *http.Request) {
	result, err := disk.snapshot()
	if err != nil {
		logAndError(w, http.StatusInternalServerError, "读取本地数据失败，请检查数据文件夹权限或数据文件是否完整", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func handleDataSave(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxDataFile)
	var payload struct {
		Data json.RawMessage `json:"data"`
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&payload); err != nil || len(payload.Data) == 0 {
		writeError(w, http.StatusBadRequest, "学习数据格式无效")
		return
	}
	result, err := disk.saveConditional(payload.Data, r.Header.Get("X-ELP-Directory"), r.Header.Get("If-Match"))
	if err != nil {
		if errors.Is(err, errDirectoryChanged) || errors.Is(err, errDataConflict) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, errRevisionRequired) {
			writeError(w, http.StatusPreconditionRequired, err.Error())
			return
		}
		logAndError(w, http.StatusInternalServerError, "写入本地文件失败，请检查数据文件夹权限", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func handleDataSelectDirectory(w http.ResponseWriter, _ *http.Request) {
	selected, canceled, err := selectDirectory()
	if err != nil {
		logAndError(w, http.StatusInternalServerError, "打开文件夹选择器失败", err)
		return
	}
	if canceled {
		writeJSON(w, http.StatusOK, map[string]any{"canceled": true, "storage": disk.status()})
		return
	}
	loadedExisting, err := disk.switchDirectory(selected)
	if err != nil {
		logAndError(w, http.StatusInternalServerError, "绑定数据文件夹失败，请检查该文件夹权限，或所选文件夹中的数据文件是否有效", err)
		return
	}
	result, err := disk.snapshot()
	if err != nil {
		logAndError(w, http.StatusInternalServerError, "读取新数据文件夹失败，请检查该文件夹权限", err)
		return
	}
	result["canceled"], result["loadedExisting"] = false, loadedExisting
	writeJSON(w, http.StatusOK, result)
}

func handleDataOpenDirectory(w http.ResponseWriter, _ *http.Request) {
	directory := disk.directoryPath()
	if directory == "" {
		writeError(w, http.StatusPreconditionFailed, "请先选择或创建永久数据文件夹")
		return
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("explorer.exe", directory)
	default:
		writeError(w, http.StatusNotImplemented, "当前系统暂不支持从程序打开文件夹")
		return
	}
	if err := command.Start(); err != nil {
		logAndError(w, http.StatusInternalServerError, "无法打开数据文件夹", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"opened": true})
}

func handleAIStatus(w http.ResponseWriter, _ *http.Request) {
	settings.RLock()
	defer settings.RUnlock()
	cfg := settings.value
	writeJSON(w, http.StatusOK, map[string]any{"connected": cfg.Connected, "model": cfg.Model, "baseUrl": cfg.BaseURL, "protocol": protocolOf(cfg), "provider": cfg.Provider, "visionVerified": cfg.VisionVerified, "vision": publicVision(cfg.Vision), "saved": settings.saved, "restored": settings.restored, "storageError": settings.loadError})
}

func handleAIConfig(w http.ResponseWriter, r *http.Request) {
	var proposed aiConfig
	if err := decodeJSON(w, r, &proposed); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	proposed.BaseURL = strings.TrimRight(strings.TrimSpace(proposed.BaseURL), "/")
	proposed.Model = strings.TrimSpace(proposed.Model)
	proposed.Protocol = protocolOf(proposed)
	proposed.Vision = nil // The independent image form owns its credentials.
	// Blank fields can reuse a saved key only for the exact same endpoint.
	settings.RLock()
	if proposed.APIKey == "" && sameAIEndpoint(proposed, settings.value) {
		proposed.APIKey = settings.value.APIKey
	}
	settings.RUnlock()
	if err := validateConfig(proposed); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	testMessages := []chatMessage{
		{Role: "system", Content: "Reply with exactly: CONNECTED"},
		{Role: "user", Content: "Connection test"},
	}
	probeTokens := 256
	if protocolOf(proposed) != "openai" {
		probeTokens = 4096
	}
	if base, _ := url.Parse(proposed.BaseURL); base != nil && base.Hostname() == "api.openai.com" {
		probeTokens = 4096
	}
	if _, err := callChat(r.Context(), proposed, testMessages, 0, probeTokens, true); err != nil {
		writeError(w, http.StatusBadGateway, "模型连接测试失败："+err.Error())
		return
	}
	proposed.Connected = true
	settings.Lock()
	defer settings.Unlock()
	// Verification belongs to the tested endpoint, key and model, never client input.
	proposed.VisionVerified = settings.value.VisionVerified && sameAIModel(proposed, settings.value)
	proposed.Vision = settings.value.Vision
	if err := settings.persist(proposed); err != nil {
		writeError(w, http.StatusInternalServerError, "连接测试成功，但无法加密保存到本地，请检查程序目录写入权限")
		return
	}
	settings.value = proposed
	settings.saved, settings.restored, settings.loadError = true, false, ""
	writeJSON(w, http.StatusOK, map[string]any{"connected": true, "model": proposed.Model, "saved": true})
}

func handleAIDisconnect(w http.ResponseWriter, _ *http.Request) {
	settings.Lock()
	defer settings.Unlock()
	if settings.value.Vision != nil && settings.value.Vision.Connected {
		remaining := aiConfig{Vision: settings.value.Vision}
		if err := settings.persist(remaining); err != nil {
			writeError(w, http.StatusInternalServerError, "无法删除文字接口配置")
			return
		}
		settings.value = remaining
		settings.restored, settings.loadError = false, ""
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if settings.path != "" {
		if err := os.Remove(settings.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusInternalServerError, "无法删除本地保存的接口配置，请检查文件权限")
			return
		}
	}
	settings.value = aiConfig{}
	settings.saved, settings.restored, settings.loadError = false, false, ""
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func handleAIChat(w http.ResponseWriter, r *http.Request) {
	var input chatRequest
	if err := decodeJSONLimit(w, r, &input, maxAIRequest); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(input.Messages) == 0 || len(input.Messages) > 30 {
		writeError(w, http.StatusBadRequest, "messages 数量无效")
		return
	}
	hasImages := false
	for _, message := range input.Messages {
		if message.Role != "system" && message.Role != "user" && message.Role != "assistant" {
			writeError(w, http.StatusBadRequest, "消息角色无效")
			return
		}
		messageHasImages, err := validateChatContent(message)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		hasImages = hasImages || messageHasImages
	}
	settings.RLock()
	cfg := settings.value
	settings.RUnlock()
	independentImage := hasImages && cfg.Vision != nil && cfg.Vision.Connected
	if independentImage {
		cfg = cfg.Vision.config()
	}
	if !cfg.Connected {
		writeError(w, http.StatusPreconditionFailed, "请先在设置页配置并测试 AI")
		return
	}
	if hasImages {
		if err := ensureVision(r.Context(), cfg, independentImage); err != nil {
			writeError(w, http.StatusBadGateway, imageRouteError(cfg, independentImage, err).Error())
			return
		}
	}
	maxTokens := input.MaxTokens
	if maxTokens <= 0 || maxTokens > 8000 {
		maxTokens = 6000
	}
	content, err := callChat(r.Context(), cfg, input.Messages, input.Temperature, maxTokens, false)
	if err != nil {
		if hasImages {
			err = imageRouteError(cfg, independentImage, err)
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := validateOutputContract(content, input.OutputContract); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"content": content, "model": cfg.Model})
}

func validateOutputContract(content, contract string) error {
	if contract == "" || contract == "text" {
		return nil
	}
	trimmed := strings.TrimSpace(content)
	switch contract {
	case "study-plan-json-v1", "personal-language-bank-json-v1":
		trimmed = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "```json"), "```"))
		trimmed = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "```"), "```"))
		start, end := strings.Index(trimmed, "{"), strings.LastIndex(trimmed, "}")
		if start < 0 || end <= start {
			return errors.New("模型没有遵循网页所需的 JSON 输出格式，请重试或更换兼容模型")
		}
		var value map[string]any
		if err := json.Unmarshal([]byte(trimmed[start:end+1]), &value); err != nil {
			return errors.New("模型返回的 JSON 格式无效，请重试或更换兼容模型")
		}
		required := []string{"summary"}
		if contract == "study-plan-json-v1" {
			required = append(required, "priorities", "phases")
		} else {
			required = append(required, "speaking", "writing")
		}
		for _, key := range required {
			if _, ok := value[key]; !ok {
				return fmt.Errorf("模型返回的 JSON 缺少网页必需字段 %q，请重试或更换兼容模型", key)
			}
		}
		if _, ok := value["summary"].(string); !ok {
			return errors.New("模型返回的 JSON 字段 \"summary\" 必须是文字")
		}
		arrayKeys := []string{"speaking", "writing"}
		if contract == "study-plan-json-v1" {
			arrayKeys = []string{"priorities", "phases"}
		}
		for _, key := range arrayKeys {
			if _, ok := value[key].([]any); !ok {
				return fmt.Errorf("模型返回的 JSON 字段 %q 必须是数组", key)
			}
		}
		return nil
	case "review-markdown-v1-writing", "review-markdown-v1-speaking":
		required := []string{"### 评分与小分", "### 总体评价", "### 确定语法错误", "### 原文优化建议", "### 目标水平范文", "### 最终值得记忆的语料"}
		if contract == "review-markdown-v1-speaking" {
			// Speaking inserts a transcript-cleanup section after the overview.
			// Build the list explicitly; slicing + append here would alias the
			// shared backing array and corrupt later headings.
			required = []string{"### 评分与小分", "### 总体评价", "### 转写整理稿", "### 确定语法错误", "### 原文优化建议", "### 目标水平范文", "### 最终值得记忆的语料"}
		}
		if !strings.HasPrefix(trimmed, "主题：") {
			return errors.New("模型没有遵循网页报告格式，第一行必须是“主题：具体主题”")
		}
		position := -1
		for _, heading := range required {
			// Accept the heading appearing more than once (a model may legitimately
			// quote a section title in the body); only require presence and order.
			next := strings.Index(content, heading)
			if next < 0 {
				return fmt.Errorf("模型没有遵循网页报告格式，缺少 %q；请重试或更换兼容模型", strings.TrimPrefix(heading, "### "))
			}
			if next <= position {
				return errors.New("模型没有按网页所需顺序返回报告章节，请重试或更换兼容模型")
			}
			position = next
		}
		return nil
	default:
		return errors.New("不支持的 AI 输出格式约束")
	}
}

func validateChatContent(message chatMessage) (bool, error) {
	switch content := message.Content.(type) {
	case string:
		if len(content) > 120000 {
			return false, errors.New("单条消息过长")
		}
		return false, nil
	case []any:
		if message.Role != "user" {
			return false, errors.New("只有用户消息可以包含图片")
		}
		textLength, imageCount := 0, 0
		for _, rawPart := range content {
			part, ok := rawPart.(map[string]any)
			if !ok {
				return false, errors.New("消息内容块无效")
			}
			switch partType, _ := part["type"].(string); partType {
			case "text":
				text, ok := part["text"].(string)
				if !ok {
					return false, errors.New("文字内容块无效")
				}
				textLength += len(text)
			case "image_url":
				image, ok := part["image_url"].(map[string]any)
				imageURL, urlOK := image["url"].(string)
				if !ok || !urlOK || !validImageURL(imageURL) {
					return false, errors.New("图片内容块无效")
				}
				imageCount++
			default:
				return false, errors.New("不支持的消息内容块")
			}
		}
		if textLength > 120000 || imageCount > 6 {
			return false, errors.New("多模态消息内容过长或图片过多")
		}
		return imageCount > 0, nil
	default:
		return false, errors.New("消息内容无效")
	}
}

func validImageURL(value string) bool {
	lower := strings.ToLower(value)
	for _, prefix := range []string{"data:image/png;base64,", "data:image/jpeg;base64,", "data:image/jpg;base64,", "data:image/gif;base64,", "data:image/webp;base64,"} {
		if strings.HasPrefix(lower, prefix) {
			return len(value) > len(prefix)
		}
	}
	parsed, err := url.Parse(value)
	return err == nil && (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")) && parsed.Host != ""
}

func callChat(ctx context.Context, cfg aiConfig, messages []chatMessage, temperature float64, maxTokens int, connectionTest bool) (string, error) {
	if protocolOf(cfg) != "openai" {
		return callNativeChat(ctx, cfg, messages, temperature, maxTokens)
	}
	payload := map[string]any{
		"model":      cfg.Model,
		"messages":   messages,
		"max_tokens": maxTokens,
	}
	// DeepSeek V4 defaults to thinking mode. This text-feedback app uses chat
	// mode for both probes and reviews so the output budget reaches the answer.
	// Never send this provider-specific option to unrelated services.
	base, _ := url.Parse(cfg.BaseURL)
	if base != nil && strings.EqualFold(base.Hostname(), "api.openai.com") {
		delete(payload, "max_tokens")
		payload["max_completion_tokens"] = maxTokens
	}
	if base != nil && strings.EqualFold(base.Hostname(), "api.deepseek.com") && strings.HasPrefix(strings.ToLower(cfg.Model), "deepseek-v4-") {
		payload["thinking"] = map[string]string{"type": "disabled"}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, aiURL(cfg, "chat/completions"), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		request.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	response, err := doAIRequest(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxAIResponse))
	if err != nil {
		return "", err
	}
	var parsed upstreamResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("模型返回了无法解析的响应（HTTP %d）", response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if parsed.Error != nil && parsed.Error.Message != "" {
			return "", errors.New(redactAIError(parsed.Error.Message, cfg))
		}
		return "", fmt.Errorf("模型服务返回 HTTP %d", response.StatusCode)
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("模型响应中没有 choices")
	}
	content := extractContent(parsed.Choices[0].Message.Content)
	if strings.TrimSpace(content) == "" {
		if parsed.Choices[0].FinishReason == "length" {
			return "", errors.New("模型输出达到长度上限，尚未生成最终答案；这不是 API Key 无效的提示")
		}
		if strings.TrimSpace(parsed.Choices[0].Message.ReasoningContent) != "" {
			return "", errors.New("模型只返回了思考内容，没有最终答案；请检查模型的思考模式和输出限制")
		}
		return "", errors.New("模型没有返回文字内容")
	}
	return content, nil
}

func extractContent(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		var parts []string
		for _, item := range typed {
			if object, ok := item.(map[string]any); ok {
				if text, ok := object["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func validateConfig(cfg aiConfig) error {
	if cfg.BaseURL == "" || cfg.Model == "" {
		return errors.New("请填写接口地址和模型名称")
	}
	if len(cfg.Model) > 200 {
		return errors.New("配置内容过长")
	}
	return validateAIEndpoint(cfg)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	return decodeJSONLimit(w, r, target, maxRequest)
}

func decodeJSONLimit(w http.ResponseWriter, r *http.Request, target any, limit int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("请求内容无效：%w", err)
	}
	return nil
}

func secureStaticServer(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// URL paths use slash semantics; use path.Clean, not the OS-specific
		// filepath.Clean, to reject traversal. Defense in depth over FileServer.
		if strings.Contains(path.Clean("/"+r.URL.Path), "..") {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
}

// guardLocalRequests protects the /api/ surface of this loopback server against
// DNS-rebinding (a foreign hostname resolved to 127.0.0.1) and cross-site CSRF
// from any web page the user happens to have open. Static assets are governed by
// CSP instead. Non-browser callers (no Sec-Fetch metadata and no Origin, e.g. a
// local curl or the test suite) are allowed so local scripting keeps working.
func guardLocalRequests(listenAddr string, next http.Handler) http.Handler {
	_, port, _ := net.SplitHostPort(listenAddr)
	allowedHosts := map[string]bool{listenAddr: true}
	if port != "" {
		for _, host := range []string{"localhost", "127.0.0.1", "[::1]"} {
			allowedHosts[host+":"+port] = true
		}
	}
	allowedOrigins := map[string]bool{}
	for host := range allowedHosts {
		allowedOrigins["http://"+host] = true
		allowedOrigins["https://"+host] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			// Host allowlist defeats DNS rebinding: a rebound request arrives with
			// the attacker's hostname in Host, which is not the loopback listener.
			if !allowedHosts[r.Host] {
				writeError(w, http.StatusForbidden, "只允许本地学习中心访问该接口")
				return
			}
			// Sec-Fetch-Site is set by all modern browsers and cannot be forged by
			// script; a cross-site/same-site value means this is not our own page.
			switch strings.ToLower(r.Header.Get("Sec-Fetch-Site")) {
			case "cross-site", "same-site":
				writeError(w, http.StatusForbidden, "只允许本地学习中心页面发起请求")
				return
			}
			// Belt and suspenders for any browser that omits Sec-Fetch metadata.
			if origin := r.Header.Get("Origin"); origin != "" && !allowedOrigins[origin] {
				writeError(w, http.StatusForbidden, "请求来源不被允许")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=(self)")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data: blob:; media-src 'self' blob:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func findResourceDir(name string) (string, error) {
	var candidates []string
	if executable, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executable)
		candidates = append(candidates, filepath.Join(executableDir, name))
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, name), filepath.Join(cwd, "..", name))
	}
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err == nil {
			if info, statErr := os.Stat(absolute); statErr == nil && info.IsDir() {
				return absolute, nil
			}
		}
	}
	return "", fmt.Errorf("找不到 %s 资源目录，请确认程序已完整解压", name)
}

func openBrowser(target string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		command = exec.Command("xdg-open", target)
	}
	return command.Start()
}

func writeStartupError(err error) {
	log.Printf("EnglishLearnPath: %v", err)
	path := "EnglishLearnPath-启动失败.txt"
	if executable, execErr := os.Executable(); execErr == nil {
		path = filepath.Join(filepath.Dir(executable), path)
	}
	_ = os.WriteFile(path, []byte(err.Error()+"\r\n"), 0600)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// logAndError records the detailed error to the local log (it may contain file
// system paths) and returns only a path-free message to the client.
func logAndError(w http.ResponseWriter, status int, publicMessage string, err error) {
	log.Printf("EnglishLearnPath: %s: %v", publicMessage, err)
	writeError(w, status, publicMessage)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
