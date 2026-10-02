package main

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Complete multi-year libraries contain several hundred MB of original audio.
// Multipart and media stay on disk; each attachment still has a 128 MiB limit.
const maxLibraryImport = 1 << 30

type bankImportPack struct {
	Pack     libraryPack
	ID       string
	Modified int64
}
type bankImportPreview struct {
	owner       *os.File
	DirectoryID string
	Stage       string
	Created     time.Time
	Packs       []bankImportPack
	Media       map[string]bool
	Ignored     int
	Committed   bool
	Added       int
}

var bankImports = struct {
	sync.Mutex
	items map[string]*bankImportPreview
}{items: map[string]*bankImportPreview{}}

func removeImportStage(stage string) {
	// Only our fresh, non-executable staging directory is removed. Uploaded ZIP
	// names never become filesystem paths; files are stored by content hash.
	if filepath.Dir(stage) == filepath.Clean(os.TempDir()) && strings.HasPrefix(filepath.Base(stage), "elp-bank-import-") {
		_ = os.RemoveAll(stage)
	}
}

func libraryMediaExtension(raw []byte) string {
	switch http.DetectContentType(raw) {
	case "audio/mpeg":
		return "mp3"
	case "audio/wave", "audio/wav", "audio/x-wav":
		return "wav"
	case "application/ogg", "audio/ogg":
		return "ogg"
	case "video/webm", "audio/webm":
		return "webm"
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpg"
	case "image/webp":
		return "webp"
	}
	if isUntaggedMP3(raw) {
		return "mp3"
	}
	return ""
}

// Go's HTTP sniffer only recognises MP3 with an ID3 tag. Require two coherent
// Layer III frame headers at the calculated boundary, not just an FF sync byte.
// Detection never rewrites tags or audio, so existing content IDs stay valid.
func isUntaggedMP3(raw []byte) bool {
	frame := func(b []byte) (int, int, int) {
		if len(b) < 4 || b[0] != 0xff || b[1]&0xe0 != 0xe0 || (b[1]>>1)&3 != 1 || b[3]&3 == 2 {
			return 0, 0, 0
		}
		version, rate, frequency := int((b[1]>>3)&3), int(b[2]>>4), int((b[2]>>2)&3)
		if version == 1 || rate == 0 || rate == 15 || frequency == 3 {
			return 0, 0, 0
		}
		sampleRate := []int{44100, 48000, 32000}[frequency]
		bitrates := []int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
		coefficient := 144000
		if version != 3 {
			bitrates = []int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
			sampleRate /= 2
			coefficient = 72000
			if version == 0 {
				sampleRate /= 2
			}
		}
		return coefficient*bitrates[rate]/sampleRate + int((b[2]>>1)&1), version, sampleRate
	}
	size, version, frequency := frame(raw)
	if size == 0 || len(raw) < size+4 {
		return false
	}
	nextSize, nextVersion, nextFrequency := frame(raw[size:])
	return nextSize != 0 && nextVersion == version && nextFrequency == frequency
}

func (preview *bankImportPreview) readFile(name string, modified int64, size int64, reader io.Reader) error {
	ext := strings.ToLower(path.Ext(name))
	if ext != ".json" && !strings.Contains("|.mp3|.wav|.ogg|.webm|.png|.jpg|.jpeg|.webp|", "|"+ext+"|") {
		preview.Ignored++
		return nil
	}
	limit := int64(128 << 20)
	if ext == ".json" {
		limit = maxPackSize
	}
	if size < 0 || size > limit {
		return fmt.Errorf("文件过大：%s", path.Base(name))
	}
	if ext != ".json" {
		return preview.readMedia(reader, size)
	}
	raw, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return err
	}
	if int64(len(raw)) > limit {
		return errors.New("文件超过导入限制")
	}
	if ext == ".json" {
		var probe map[string]json.RawMessage
		if json.Unmarshal(raw, &probe) != nil || probe["units"] == nil {
			preview.Ignored++
			return nil
		}
		var pack libraryPack
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&pack); err != nil {
			return fmt.Errorf("题库文件无法识别：%s", path.Base(name))
		}
		completeLibraryProvenance(&pack)
		if err := validatePack(pack); err != nil {
			return fmt.Errorf("%s：%s", pack.Title, err)
		}
		canonical, _ := json.Marshal(pack)
		id := hashContent(canonical)
		for _, existing := range preview.Packs {
			if existing.ID == id {
				preview.Ignored++
				return nil
			}
		}
		preview.Packs = append(preview.Packs, bankImportPack{Pack: pack, ID: id, Modified: modified})
		return nil
	}
	return nil
}

func (preview *bankImportPreview) readZIP(reader io.ReaderAt, size int64) error {
	archive, err := zip.NewReader(reader, size)
	if err != nil {
		return errors.New("压缩包无法打开，请重新选择完整的题库 ZIP")
	}
	if len(archive.File) > 5000 {
		return errors.New("压缩包文件过多，请选择整理后的题库包")
	}
	var total uint64
	for _, file := range archive.File {
		name := strings.ReplaceAll(file.Name, "\\", "/")
		if strings.Contains(name, ":") || strings.HasPrefix(name, "/") || path.Clean(name) != strings.TrimSuffix(name, "/") || strings.HasPrefix(name, "../") || file.Mode()&os.ModeSymlink != 0 {
			return errors.New("压缩包包含不安全的路径")
		}
		if path.Base(name) == backupManifestName {
			return errors.New("这是学习档案备份，请到 AI 与数据设置中恢复备份")
		}
		if file.UncompressedSize64 > maxLibraryImport-total {
			return errors.New("解压内容超过 1 GB，请拆分题库包")
		}
		total += file.UncompressedSize64
	}
	for _, file := range archive.File {
		if file.FileInfo().IsDir() {
			continue
		}
		input, err := file.Open()
		if err != nil {
			return err
		}
		err = preview.readFile(strings.ReplaceAll(file.Name, "\\", "/"), file.Modified.UnixMilli(), int64(file.UncompressedSize64), input)
		input.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *diskStore) validateImport(preview *bankImportPreview, directoryID string) error {
	reader, err := s.importReader(directoryID)
	if err != nil {
		return err
	}
	preview.DirectoryID = reader.directoryIDLocked()
	if len(preview.Packs) == 0 {
		return errors.New("没有识别到可导入题目。请选择题库包 ZIP 或整理好的题库文件夹，说明和核验文件会自动跳过")
	}
	stage := &diskStore{directory: preview.Stage}
	for _, item := range preview.Packs {
		for _, unit := range item.Pack.Units {
			refs := append([]string{}, unit.Images...)
			if unit.Audio != "" {
				refs = append(refs, unit.Audio)
			}
			for _, ref := range refs {
				if preview.Media[ref] {
					continue
				}
				if err := copyPreviewMedia(preview, reader, ref); err != nil {
					return fmt.Errorf("“%s”缺少配套音频或图片，请选择完整题库包：%w", item.Pack.Title, err)
				}
			}
		}
		if _, err := stage.importPack(item.Pack); err != nil {
			return fmt.Errorf("“%s”未通过校验，请重新获取完整题库包", item.Pack.Title)
		}
	}
	sort.SliceStable(preview.Packs, func(i, j int) bool {
		a, b := preview.Packs[i], preview.Packs[j]
		if a.Modified != b.Modified {
			return a.Modified < b.Modified
		}
		return len(a.Pack.Units) < len(b.Pack.Units)
	})
	return nil
}

func registerLibraryImportAPI(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/library/import/preview", func(w http.ResponseWriter, r *http.Request) {
		// Large local archives need more than the server's normal 15-second
		// body deadline on slower disks. Keep the relaxed deadline route-local.
		_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(10 * time.Minute))
		r.Body = http.MaxBytesReader(w, r.Body, maxLibraryImport+(8<<20))
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			writeError(w, 400, "无法读取文件，导入总大小不能超过 1 GB")
			return
		}
		defer r.MultipartForm.RemoveAll()
		files := r.MultipartForm.File["files"]
		if len(files) == 0 || len(files) > 5000 {
			writeError(w, 400, "请选择题库 ZIP 或文件夹")
			return
		}
		var uploadSize int64
		for _, file := range files {
			if file.Size < 0 || file.Size > maxLibraryImport-uploadSize {
				writeError(w, 400, "导入总大小不能超过 1 GB")
				return
			}
			uploadSize += file.Size
		}
		var modified []int64
		_ = json.Unmarshal([]byte(r.FormValue("modified")), &modified)
		stage, err := os.MkdirTemp("", "elp-bank-import-")
		if err != nil {
			writeError(w, 500, "无法准备导入")
			return
		}
		keep := false
		defer func() {
			if !keep {
				removeImportStage(stage)
			}
		}()
		preview := &bankImportPreview{Stage: stage, Created: time.Now(), Media: map[string]bool{}}
		preview.owner, err = lockDataDirectory(stage)
		if err != nil {
			writeError(w, 500, "无法锁定导入暂存目录")
			return
		}
		defer func() {
			if !keep {
				preview.cleanup()
			}
		}()
		for index, file := range files {
			input, openErr := file.Open()
			if openErr != nil {
				writeError(w, 400, "无法打开所选文件")
				return
			}
			stamp := int64(0)
			if index < len(modified) {
				stamp = modified[index]
			}
			if len(files) == 1 && strings.EqualFold(filepath.Ext(file.Filename), ".zip") {
				err = preview.readZIP(input, file.Size)
			} else {
				err = preview.readFile(file.Filename, stamp, file.Size, input)
			}
			input.Close()
			if err != nil {
				writeError(w, 400, err.Error())
				return
			}
		}
		if err := disk.validateImport(preview, r.Header.Get("X-ELP-Directory")); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		counts := map[string]int{"reading": 0, "listening": 0, "writing": 0, "speaking": 0}
		seen := map[string]bool{}
		titles := []string{}
		for _, item := range preview.Packs {
			titles = append(titles, item.Pack.Title)
			for _, unit := range item.Pack.Units {
				key := item.Pack.Source.URL + "\n" + item.Pack.Source.Name + "\n" + unit.ID
				if !seen[key] {
					seen[key] = true
					counts[unit.Skill]++
				}
			}
		}
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			writeError(w, 500, "无法准备导入")
			return
		}
		token := hex.EncodeToString(random[:])
		bankImports.Lock()
		for id, old := range bankImports.items {
			if time.Since(old.Created) > time.Hour || len(bankImports.items) >= 4 {
				old.cleanup()
				delete(bankImports.items, id)
			}
		}
		bankImports.items[token] = preview
		bankImports.Unlock()
		keep = true
		writeJSON(w, 200, map[string]any{"token": token, "counts": counts, "titles": titles, "attachments": len(preview.Media), "ignored": preview.Ignored})
	})
	mux.HandleFunc("POST /api/library/import/{token}", func(w http.ResponseWriter, r *http.Request) {
		// Retain a receipt: a lost HTTP response must not force a second upload
		// or leave the user unable to determine whether the import committed.
		bankImports.Lock()
		defer bankImports.Unlock()
		preview := bankImports.items[r.PathValue("token")]
		if preview == nil {
			if receipt, err := disk.loadImportReceipt(r.PathValue("token"), r.Header.Get("X-ELP-Directory")); err == nil {
				writeJSON(w, 200, map[string]any{"added": receipt.Added, "existing": receipt.Existing})
				return
			}
			writeError(w, 409, "预览已过期，请重新选择题库包")
			return
		}
		if preview.DirectoryID != r.Header.Get("X-ELP-Directory") {
			writeError(w, 409, errDirectoryChanged.Error())
			return
		}
		disk.RLock()
		currentDirectory := disk.directoryIDLocked()
		disk.RUnlock()
		if currentDirectory != preview.DirectoryID {
			writeError(w, 409, errDirectoryChanged.Error())
			return
		}
		if !preview.Committed {
			added, err := disk.commitImport(preview, r.Header.Get("X-ELP-Directory"))
			if err != nil {
				writeError(w, 400, err.Error())
				return
			}
			preview.Committed = true
			preview.Added = added
			preview.cleanup()
		}
		if err := disk.saveImportReceipt(r.PathValue("token"), preview); err != nil {
			writeError(w, 500, "题库已保存，但回执暂未写入，请重试确认结果")
			return
		}
		delete(bankImports.items, r.PathValue("token"))
		writeJSON(w, 200, map[string]any{"added": preview.Added, "existing": len(preview.Packs) - preview.Added})
	})
	mux.HandleFunc("DELETE /api/library/import/{token}", func(w http.ResponseWriter, r *http.Request) {
		bankImports.Lock()
		preview := bankImports.items[r.PathValue("token")]
		delete(bankImports.items, r.PathValue("token"))
		bankImports.Unlock()
		if preview != nil {
			preview.cleanup()
		}
		writeJSON(w, 200, map[string]bool{"canceled": true})
	})
}
