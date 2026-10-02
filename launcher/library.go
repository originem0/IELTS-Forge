package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const maxPackSize = 8 << 20

var libraryID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,79}$`)
var contentID = regexp.MustCompile(`^[a-f0-9]{64}$`)
var mediaID = regexp.MustCompile(`^[a-f0-9]{64}\.(mp3|wav|ogg|webm|png|jpg|webp)$`)

// Packs contain data, never executable source HTML. Content hashes make question
// revisions immutable so updating a bank cannot change an old attempt's question.
type libraryPack struct {
	Version int           `json:"version"`
	Title   string        `json:"title"`
	Source  librarySource `json:"source"`
	Units   []libraryUnit `json:"units"`
	Exams   []libraryExam `json:"exams,omitempty"`
}

type librarySource struct {
	Name       string             `json:"name"`
	URL        string             `json:"url"`
	Status     string             `json:"status"`
	Season     string             `json:"season,omitempty"`
	License    string             `json:"license,omitempty"`
	Provenance *libraryProvenance `json:"provenance,omitempty"`
}

// YearKind distinguishes an exam/publication year from a local collection date.
// Source verification alone must never promote material to an authentic exam.
type libraryProvenance struct {
	Year     int      `json:"year"`
	YearKind string   `json:"yearKind"`
	Category string   `json:"category"`
	Evidence []string `json:"evidence,omitempty"`
	Note     string   `json:"note,omitempty"`
}

type libraryUnit struct {
	Provenance *libraryProvenance `json:"provenance,omitempty"`
	ID         string             `json:"id"`
	Skill      string             `json:"skill"`
	Part       string             `json:"part"`
	Title      string             `json:"title"`
	Prompt     string             `json:"prompt"`
	Minutes    int                `json:"minutes"`
	Passages   []libraryParagraph `json:"passages,omitempty"`
	Images     []string           `json:"images,omitempty"`
	Audio      string             `json:"audio,omitempty"`
	Transcript string             `json:"transcript,omitempty"`
	Groups     []libraryGroup     `json:"groups,omitempty"`
	Related    []string           `json:"related,omitempty"`
	Tracks     []libraryTrack     `json:"tracks,omitempty"`
	// Authored units are retired by hiding (not deleting the immutable pack); hidden units are
	// kept for history and un-hiding but filtered out of selection by the client.
	Hidden bool `json:"hidden,omitempty"`
}

type libraryParagraph struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Label string `json:"label,omitempty"`
}

type libraryOption struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type libraryGroup struct {
	ID          string            `json:"id"`
	Kind        string            `json:"kind"` // text, single, multiple, matching
	Instruction string            `json:"instruction"`
	Context     string            `json:"context,omitempty"`
	Table       [][]string        `json:"table,omitempty"`
	Options     []libraryOption   `json:"options,omitempty"`
	MaxWords    int               `json:"maxWords,omitempty"`
	AllowNumber bool              `json:"allowNumber,omitempty"`
	NumberOnly  bool              `json:"numberOnly,omitempty"`
	Questions   []libraryQuestion `json:"questions"`
}

type libraryQuestion struct {
	ID          string          `json:"id"`
	Label       string          `json:"label"`
	Text        string          `json:"text"`
	Answers     []string        `json:"answers"` // alternatives except multiple: all required
	Options     []libraryOption `json:"options,omitempty"`
	Evidence    string          `json:"evidence,omitempty"`
	Explanation string          `json:"explanation,omitempty"`
}

func validatePack(pack libraryPack) error {
	if err := validateLibraryProvenance(pack.Source.Provenance); err != nil {
		return err
	}
	if pack.Version != 1 || strings.TrimSpace(pack.Title) == "" || len(pack.Units) == 0 || len(pack.Units) > 500 {
		return errors.New("题包版本、标题或题目数量无效")
	}
	if strings.TrimSpace(pack.Source.Name) == "" {
		return errors.New("题包必须注明来源")
	}
	if pack.Source.URL != "" {
		u, err := url.Parse(pack.Source.URL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
			return errors.New("来源链接无效")
		}
	}
	if pack.Source.Status != "unverified" && pack.Source.Status != "verified" && pack.Source.Status != "generated" {
		return errors.New("来源核验状态无效")
	}
	units := map[string]bool{}
	for _, unit := range pack.Units {
		if err := validateLibraryProvenance(unit.Provenance); err != nil {
			return err
		}
		if !libraryID.MatchString(unit.ID) || units[unit.ID] || strings.TrimSpace(unit.Title) == "" || strings.TrimSpace(unit.Prompt) == "" || unit.Minutes < 0 || unit.Minutes > 240 {
			return errors.New("题目编号、标题、说明或时间无效")
		}
		units[unit.ID] = true
		parts := map[string]string{"reading": "academic general", "listening": "1 2 3 4", "writing": "Task-1-Academic Task-1-General Task-2", "speaking": "p1 p2 p3"}
		validPart := false
		for _, part := range strings.Fields(parts[unit.Skill]) {
			if unit.Part == part {
				validPart = true
			}
		}
		if !validPart {
			return errors.New("科目或 Task/Part 无效")
		}
		if unit.Skill == "reading" && len(unit.Passages) == 0 {
			return errors.New("阅读题缺少原文")
		}
		if unit.Skill == "listening" && (!mediaID.MatchString(unit.Audio) || strings.TrimSpace(unit.Transcript) == "") {
			return errors.New("听力题必须包含音频与原文")
		}
		if unit.Audio != "" && (!mediaID.MatchString(unit.Audio) || !isAudioID(unit.Audio)) {
			return errors.New("音频引用无效")
		}
		for _, img := range unit.Images {
			if !mediaID.MatchString(img) || isAudioID(img) {
				return errors.New("图片引用无效")
			}
		}
		paragraphs := map[string]bool{}
		for _, p := range unit.Passages {
			if !libraryID.MatchString(p.ID) || paragraphs[p.ID] || strings.TrimSpace(p.Text) == "" {
				return errors.New("文章段落无效")
			}
			paragraphs[p.ID] = true
		}
		// Listening may be a listen-only resource (audio + transcript, no questions) for
		// found recordings whose questions live only on paper; it is simply not auto-graded.
		// Reading is never gradable without its questions, so it still requires a group.
		if unit.Skill == "reading" && len(unit.Groups) == 0 {
			return errors.New("阅读题缺少答题组")
		}
		groups, questions := map[string]bool{}, map[string]bool{}
		for _, group := range unit.Groups {
			if len(group.Table) > 100 {
				return errors.New("题目表格行数过多")
			}
			for _, row := range group.Table {
				if len(row) == 0 || len(row) > 20 || len(row) != len(group.Table[0]) {
					return errors.New("题目表格列数无效")
				}
			}
			if !libraryID.MatchString(group.ID) || groups[group.ID] || len(group.Questions) == 0 || strings.TrimSpace(group.Instruction) == "" {
				return errors.New("题组编号或说明无效")
			}
			groups[group.ID] = true
			if group.Kind != "text" && group.Kind != "single" && group.Kind != "multiple" && group.Kind != "matching" {
				return errors.New("不支持的题型")
			}
			if group.MaxWords < 0 || group.MaxWords > 100 {
				return errors.New("答案词数限制无效")
			}
			options := map[string]bool{}
			for _, opt := range group.Options {
				if !libraryID.MatchString(opt.ID) || options[opt.ID] || strings.TrimSpace(opt.Text) == "" {
					return errors.New("题目选项无效")
				}
				options[opt.ID] = true
			}
			for _, q := range group.Questions {
				questionOptions := options
				if len(q.Options) > 0 {
					questionOptions = map[string]bool{}
					for _, option := range q.Options {
						if !libraryID.MatchString(option.ID) || questionOptions[option.ID] || strings.TrimSpace(option.Text) == "" {
							return errors.New("小题选项无效")
						}
						questionOptions[option.ID] = true
					}
				}
				if group.Kind != "text" && len(questionOptions) < 2 {
					return errors.New("选择或匹配题缺少选项")
				}
				if !libraryID.MatchString(q.ID) || questions[q.ID] || strings.TrimSpace(q.Label) == "" || strings.TrimSpace(q.Text) == "" || len(q.Answers) == 0 {
					return errors.New("小题或答案无效")
				}
				questions[q.ID] = true
				seen := map[string]bool{}
				for _, answer := range q.Answers {
					if strings.TrimSpace(answer) == "" || seen[answer] || (group.Kind != "text" && !questionOptions[answer]) {
						return errors.New("答案为空、重复或不在选项中")
					}
					seen[answer] = true
				}
				if q.Evidence != "" && !paragraphs[q.Evidence] {
					return errors.New("原文证据引用不存在")
				}
			}
		}
	}
	for _, unit := range pack.Units {
		for _, related := range unit.Related {
			if !units[related] || related == unit.ID {
				return errors.New("关联题目不存在或指向自身")
			}
		}
	}
	return validateLibraryExams(pack)
}

func isAudioID(id string) bool {
	return strings.HasSuffix(id, ".mp3") || strings.HasSuffix(id, ".wav") || strings.HasSuffix(id, ".ogg") || strings.HasSuffix(id, ".webm")
}

// Call under disk's lock: directory switching cannot redirect half a write.
// Reject symlink escapes even when an existing user directory contains links.
func (s *diskStore) libraryPathLocked(collection, filename string) (string, error) {
	if s.directory == "" {
		return "", errors.New("请先绑定永久数据目录")
	}
	root, err := filepath.EvalSymlinks(s.directory)
	if err != nil {
		return "", err
	}
	dir := root
	for _, segment := range []string{"library", collection} {
		dir = filepath.Join(dir, segment)
		if err := os.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return "", err
		}
		resolved, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return "", err
		}
		rel, err := filepath.Rel(root, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return "", errors.New("资料目录不能指向数据目录之外")
		}
	}
	path := filepath.Join(dir, filename)
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("资料文件不能是符号链接")
	}
	return path, nil
}

func atomicLibraryWrite(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".library-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func hashContent(data []byte) string { hash := sha256.Sum256(data); return hex.EncodeToString(hash[:]) }

func (s *diskStore) importPack(pack libraryPack, directoryID ...string) (string, error) {
	completeLibraryProvenance(&pack)
	if err := validatePack(pack); err != nil {
		return "", err
	}
	data, err := json.Marshal(pack)
	if err != nil {
		return "", err
	}
	if len(data) > maxPackSize {
		return "", errors.New("题包过大，请分批导入")
	}
	id := hashContent(data)
	s.Lock()
	defer s.Unlock()
	if err := s.checkDirectoryLocked(directoryID...); err != nil {
		return "", err
	}
	checkedMedia := map[string]bool{}
	for _, unit := range pack.Units {
		refs := append([]string{}, unit.Images...)
		if unit.Audio != "" {
			refs = append(refs, unit.Audio)
		}
		for _, ref := range refs {
			if checkedMedia[ref] {
				continue
			}
			checkedMedia[ref] = true
			path, err := s.libraryPathLocked("media", ref)
			if err != nil {
				return "", err
			}
			digest, err := hashFile(path, 128<<20)
			if err != nil || digest != strings.Split(ref, ".")[0] {
				return "", fmt.Errorf("附件缺失或损坏：%s", ref)
			}
		}
	}
	path, err := s.libraryPathLocked("packs", id+".json")
	if err != nil {
		return "", err
	}
	if previous, err := os.ReadFile(path); err == nil {
		if !bytes.Equal(previous, data) {
			return "", errors.New("已存在的题包内容损坏")
		}
		return id, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return id, atomicLibraryWrite(path, data)
}

func (s *diskStore) loadPackLocked(id string) (libraryPack, error) {
	var pack libraryPack
	if !contentID.MatchString(id) {
		return pack, errors.New("题包编号无效")
	}
	path, err := s.libraryPathLocked("packs", id+".json")
	if err != nil {
		return pack, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return pack, err
	}
	if hashContent(raw) != id {
		return pack, errors.New("题包校验失败")
	}
	if err := json.Unmarshal(raw, &pack); err != nil {
		return pack, err
	}
	return pack, validatePack(pack)
}

func decodeLibraryJSON(w http.ResponseWriter, r *http.Request, limit int64, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("只允许一个 JSON 对象")
	}
	return nil
}

func registerLibraryAPI(mux *http.ServeMux) {
	registerLibraryDeleteAPI(mux)
	mux.HandleFunc("POST /api/library/packs", func(w http.ResponseWriter, r *http.Request) {
		var pack libraryPack
		if err := decodeLibraryJSON(w, r, maxPackSize, &pack); err != nil {
			writeError(w, 400, "题包格式无效："+err.Error())
			return
		}
		id, err := disk.importPack(pack, r.Header.Get("X-ELP-Directory"))
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 201, map[string]string{"id": id})
	})
	mux.HandleFunc("GET /api/library/packs", handleLibraryCatalogue)
	mux.HandleFunc("GET /api/library/packs/{id}", func(w http.ResponseWriter, r *http.Request) {
		disk.Lock()
		defer disk.Unlock()
		pack, err := disk.loadPackLocked(r.PathValue("id"))
		if err != nil {
			writeError(w, 404, "题包不存在或校验失败")
			return
		}
		writeJSON(w, 200, pack)
	})
	registerLibraryMediaAPI(mux)
	registerLibraryAttemptsAPI(mux)
	registerLibraryResultsAPI(mux)
	registerLibraryExamAPI(mux)
	registerLibrarySummaryAPI(mux)
	registerLibraryImportAPI(mux)
	registerBackupAPI(mux)
}

func registerLibraryMediaAPI(mux *http.ServeMux) {
	types := map[string]string{"audio/mpeg": "mp3", "audio/wav": "wav", "audio/ogg": "ogg", "audio/webm": "webm", "image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}
	mux.HandleFunc("POST /api/library/media", func(w http.ResponseWriter, r *http.Request) {
		ext := types[strings.Split(r.Header.Get("Content-Type"), ";")[0]]
		if ext == "" {
			writeError(w, 400, "不支持的媒体类型")
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 128<<20))
		if err != nil || len(raw) == 0 {
			writeError(w, 400, "媒体为空或超过 128 MB")
			return
		}
		if libraryMediaExtension(raw) != ext {
			writeError(w, 400, "媒体内容与文件类型不匹配")
			return
		}
		id := hashContent(raw) + "." + ext
		disk.Lock()
		defer disk.Unlock()
		if err := disk.checkDirectoryLocked(r.Header.Get("X-ELP-Directory")); err != nil {
			writeError(w, 409, err.Error())
			return
		}
		path, err := disk.libraryPathLocked("media", id)
		if err == nil {
			err = atomicLibraryWrite(path, raw)
		}
		if err != nil {
			writeError(w, 500, "媒体保存失败")
			return
		}
		writeJSON(w, 201, map[string]string{"id": id})
	})
	mux.HandleFunc("GET /api/library/media/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !mediaID.MatchString(id) {
			writeError(w, 400, "媒体编号无效")
			return
		}
		disk.Lock()
		path, err := disk.libraryPathLocked("media", id)
		var file *os.File
		if err == nil {
			file, err = os.Open(path)
		}
		disk.Unlock()
		if err != nil {
			writeError(w, 404, "媒体不存在")
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			writeError(w, 500, "媒体读取失败")
			return
		}
		for mime, ext := range types {
			if strings.HasSuffix(id, "."+ext) {
				w.Header().Set("Content-Type", mime)
			}
		}
		http.ServeContent(w, r, id, info.ModTime(), file)
	})
}
