package main

import "testing"

func TestLibraryTextScoring(t *testing.T) {
	for _, tc := range []struct {
		name, answer, expected      string
		words                       int
		number, numberOnly, correct bool
	}{
		{"case and space", "  MONDAY  ", "Monday", 1, false, false, true},
		{"spelling", "Munday", "Monday", 1, false, false, false},
		{"plural", "libraries", "library", 1, false, false, false},
		{"hyphen", "well‑known", "well-known", 1, false, false, true},
		{"too long", "the community library", "the community library", 2, false, false, false},
		{"word and number", "29 centimetres", "29 centimetres", 1, true, false, true},
		{"number counts without exception", "29 centimetres", "29 centimetres", 1, false, false, false},
		{"number only", "1906", "1906", 0, false, true, true},
		{"number only rejects prose", "in 1906", "in 1906", 0, false, true, false},
		{"missing", "", "Monday", 1, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := sampleLibraryPack().Units[0]
			u.Groups[0].MaxWords = tc.words
			u.Groups[0].AllowNumber = tc.number
			u.Groups[0].NumberOnly = tc.numberOnly
			u.Groups[0].Questions[0].Answers = []string{tc.expected}
			score := gradeLibraryUnit(u, map[string][]string{"q1": {tc.answer}})
			if score.Items[0].Correct != tc.correct {
				t.Fatalf("unexpected result: %+v", score)
			}
		})
	}
}

func TestLibraryMultipleChoicePartialCreditAndOrder(t *testing.T) {
	u := sampleLibraryPack().Units[0]
	u.Groups[0].Kind = "multiple"
	u.Groups[0].Questions[0].Answers = []string{"A", "C"}
	for _, tc := range []struct {
		answers []string
		points  int
	}{{[]string{"C", "A"}, 2}, {[]string{"A", "B"}, 1}, {[]string{"A", "A"}, 1}, {[]string{"A", "B", "C"}, 0}, {nil, 0}} {
		score := gradeLibraryUnit(u, map[string][]string{"q1": tc.answers})
		if score.Points != tc.points || score.Total != 2 {
			t.Fatalf("%v: %+v", tc.answers, score)
		}
	}
}
