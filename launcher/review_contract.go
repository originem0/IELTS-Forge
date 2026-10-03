package main

import (
	"encoding/json"
	"errors"
	"strings"
)

type reviewCriterion struct {
	Code     string   `json:"code"`
	Score    *float64 `json:"score"`
	Evidence string   `json:"evidence"`
}
type reviewCorrection struct {
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
	Type        string `json:"type"`
	Reason      string `json:"reason"`
}
type reviewImprovement struct {
	Original   string `json:"original"`
	Suggestion string `json:"suggestion"`
	Reason     string `json:"reason"`
}
type reviewExpression struct {
	English string `json:"english"`
	Chinese string `json:"chinese"`
}
type structuredReview struct {
	Topic            string              `json:"topic"`
	Overall          *float64            `json:"overall"`
	Range            []float64           `json:"range"`
	Criteria         []reviewCriterion   `json:"criteria"`
	Overview         string              `json:"overview"`
	Transcript       string              `json:"transcript"`
	Corrections      []reviewCorrection  `json:"corrections"`
	Improvements     []reviewImprovement `json:"improvements"`
	ModelAnswer      string              `json:"modelAnswer"`
	ModelExplanation string              `json:"modelExplanation"`
	Language         struct {
		Collocations     []reviewExpression `json:"collocations"`
		SentencePatterns []reviewExpression `json:"sentencePatterns"`
	} `json:"language"`
}

func validReviewBand(n float64) bool { return n >= 0 && n <= 9 && n*2 == float64(int(n*2)) }
func reviewText(s string) bool       { return strings.TrimSpace(s) != "" }
func validateStructuredReview(content, contract string) error {
	invalid := errors.New("评分报告缺少有效正文、小分、证据或必需数组")
	var v structuredReview
	if json.Unmarshal([]byte(content), &v) != nil {
		return invalid
	}
	speaking := contract == "review-json-v1-speaking"
	first := "TR"
	if contract == "review-json-v1-writing-task1" {
		first = "TA"
	}
	expected := []string{first, "CC", "LR", "GRA"}
	if speaking {
		expected = []string{"FC", "LR", "GRA", "P"}
	}
	if !reviewText(v.Topic) || v.Overall == nil || !validReviewBand(*v.Overall) || !reviewText(v.Overview) || !reviewText(v.ModelAnswer) || len(v.Criteria) != 4 || v.Corrections == nil || v.Improvements == nil || v.Language.Collocations == nil || v.Language.SentencePatterns == nil {
		return invalid
	}
	if speaking && (!reviewText(v.Transcript) || len(v.Range) != 2 || !validReviewBand(v.Range[0]) || !validReviewBand(v.Range[1]) || v.Range[0] > *v.Overall || v.Range[1] < *v.Overall) {
		return invalid
	}
	seen := map[string]bool{}
	for _, c := range v.Criteria {
		allowed := false
		for _, code := range expected {
			if c.Code == code {
				allowed = true
			}
		}
		if !allowed || seen[c.Code] || !reviewText(c.Evidence) {
			return invalid
		}
		seen[c.Code] = true
		if speaking && c.Code == "P" {
			if c.Score != nil {
				return errors.New("只有文字转写时不能给发音分数")
			}
		} else if c.Score == nil || !validReviewBand(*c.Score) {
			return invalid
		}
	}
	for _, c := range v.Corrections {
		if !reviewText(c.Original) || !reviewText(c.Replacement) || !reviewText(c.Type) || !reviewText(c.Reason) {
			return invalid
		}
	}
	for _, c := range v.Improvements {
		if !reviewText(c.Original) || !reviewText(c.Suggestion) || !reviewText(c.Reason) {
			return invalid
		}
	}
	for _, list := range [][]reviewExpression{v.Language.Collocations, v.Language.SentencePatterns} {
		for _, e := range list {
			if !reviewText(e.English) || !reviewText(e.Chinese) {
				return invalid
			}
		}
	}
	return nil
}
