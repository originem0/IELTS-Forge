package main

import (
	"encoding/json"
	"testing"
)

func TestStructuredReviewContract(t *testing.T) {
	for _, kind := range []string{"writing-task1", "writing-task2", "speaking"} {
		first := "TR"
		if kind == "writing-task1" {
			first = "TA"
		}
		codes := []string{first, "CC", "LR", "GRA"}
		if kind == "speaking" {
			codes = []string{"FC", "LR", "GRA", "P"}
		}
		criteria := []any{}
		for _, code := range codes {
			var score any = 6.5
			if code == "P" {
				score = nil
			}
			criteria = append(criteria, map[string]any{"code": code, "score": score, "evidence": "Specific source evidence"})
		}
		valid := map[string]any{"topic": "Transport", "overall": 6.5, "range": []float64{6, 7}, "criteria": criteria, "overview": "Clear position", "transcript": "I travel by bus.", "corrections": []any{}, "improvements": []any{}, "modelAnswer": "Public transport helps.", "language": map[string]any{"collocations": []any{}, "sentencePatterns": []any{}}}
		check := func(value map[string]any) error {
			raw, _ := json.Marshal(value)
			return validateOutputContract(string(raw), "review-json-v1-"+kind)
		}
		if err := check(valid); err != nil {
			t.Fatal(kind, err)
		}
		for _, key := range []string{"topic", "overall", "criteria", "overview", "modelAnswer", "language", "corrections", "improvements"} {
			saved := valid[key]
			valid[key] = nil
			if check(valid) == nil {
				t.Fatalf("%s accepted missing %s", kind, key)
			}
			valid[key] = saved
		}
		for _, score := range []float64{-1, 9.5, 6.2} {
			valid["overall"] = score
			if check(valid) == nil {
				t.Fatal("accepted invalid band", score)
			}
		}
		valid["overall"] = 6.5
		criteria[0].(map[string]any)["code"] = "wrong"
		if check(valid) == nil {
			t.Fatal("accepted wrong rubric")
		}
		criteria[0].(map[string]any)["code"] = codes[0]
		if kind == "speaking" {
			criteria[3].(map[string]any)["score"] = 7
			if check(valid) == nil {
				t.Fatal("invented pronunciation")
			}
		}
	}
}
