package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// Found exam recordings whose questions live only on paper are imported as listen-only
// resources: a listening unit with audio + transcript but no question groups. Reading has
// no such mode — without questions it cannot be graded, so it still requires a group.
func TestListenOnlyResourceValidatesWithoutGroups(t *testing.T) {
	audio := strings.Repeat("a", 64) + ".mp3"
	listenOnly := libraryPack{
		Version: 1, Title: "剑桥听音资源",
		Source: librarySource{Name: "Cambridge IELTS 21", Status: "verified"},
		Units: []libraryUnit{{
			ID: "camb21-test1-part1", Skill: "listening", Part: "1",
			Title: "Test 1 · Part 1", Prompt: "听录音，题目见纸质书。",
			Audio: audio, Transcript: "Welcome to the test.",
		}},
	}
	if err := validatePack(listenOnly); err != nil {
		t.Fatalf("listen-only listening unit (no groups) must validate, got: %v", err)
	}

	// Listening still requires audio + transcript even when it carries no questions.
	noAudio := listenOnly
	noAudio.Units = []libraryUnit{listenOnly.Units[0]}
	noAudio.Units[0].Audio = ""
	if err := validatePack(noAudio); err == nil {
		t.Fatal("listening unit without audio must still be rejected")
	}

	// Reading without a question group stays rejected — it can never be graded.
	reading := libraryPack{
		Version: 1, Title: "阅读",
		Source: librarySource{Name: "src", Status: "unverified"},
		Units: []libraryUnit{{
			ID: "r1", Skill: "reading", Part: "academic", Title: "P", Prompt: "read",
			Passages: []libraryParagraph{{ID: "P1", Text: "para"}},
		}},
	}
	if err := validatePack(reading); err == nil {
		t.Fatal("reading unit without a question group must be rejected")
	}
}

// Authored units are retired by marking them hidden rather than deleting the immutable pack
// (which would break history). The strict import decoder must accept the field, not reject it
// as unknown, and the pack must still validate.
func TestHiddenUnitFieldAccepted(t *testing.T) {
	raw := []byte(`{"version":1,"title":"我的题目","source":{"name":"我的题目","status":"unverified"},"units":[{"id":"u1","skill":"writing","part":"Task-2","title":"t","prompt":"p","minutes":40,"hidden":true}]}`)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var pack libraryPack
	if err := decoder.Decode(&pack); err != nil {
		t.Fatalf("strict decode rejected the hidden field: %v", err)
	}
	if !pack.Units[0].Hidden {
		t.Fatal("hidden field did not decode")
	}
	if err := validatePack(pack); err != nil {
		t.Fatalf("hidden unit must still validate: %v", err)
	}
}
