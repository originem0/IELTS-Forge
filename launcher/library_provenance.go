package main

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

func completeLibraryProvenance(pack *libraryPack) {
	if pack.Source.Provenance == nil {
		category := "practice"
		if pack.Source.Status == "generated" {
			category = "generated"
		}
		if pack.Source.Name == "我的题目" {
			category = "supplied"
		}
		pack.Source.Provenance = &libraryProvenance{Year: time.Now().Year(), YearKind: "collected", Category: category, Note: "年份为本地收录年；来源未提供可核验的出题年份。"}
	}
	for i := range pack.Units {
		if pack.Units[i].Provenance == nil {
			pack.Units[i].Provenance = pack.Source.Provenance
		}
	}
}

func validateLibraryProvenance(p *libraryProvenance) error {
	if p == nil {
		return nil
	} // Immutable legacy versions remain readable.
	if p.Year < 1980 || p.Year > time.Now().Year()+1 {
		return errors.New("题目年份无效")
	}
	switch p.YearKind {
	case "exam", "publication", "season", "created", "collected":
	default:
		return errors.New("必须注明年份依据")
	}
	switch p.Category {
	case "authentic", "recall", "generated", "practice", "supplied":
	default:
		return errors.New("题目性质无效")
	}
	if p.Category == "authentic" && (len(p.Evidence) == 0 || p.YearKind == "collected" || strings.TrimSpace(p.Note) == "") {
		return errors.New("真题标注必须提供核验链接、年份依据与说明")
	}
	for _, link := range p.Evidence {
		u, err := url.Parse(link)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
			return errors.New("题目核验链接无效")
		}
	}
	return nil
}
