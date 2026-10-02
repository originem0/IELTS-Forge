package main

import (
	"testing"
	"time"
)

func TestLibraryProvenanceDoesNotPromoteLegacyVerification(t *testing.T) {
	pack := sampleLibraryPack()
	pack.Source.Status = "verified"
	s := libraryTestStore(t)
	id, err := s.importPack(pack)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.loadPackLocked(id)
	if err != nil {
		t.Fatal(err)
	}
	p := saved.Units[0].Provenance
	if p == nil || p.Category != "practice" || p.YearKind != "collected" || p.Year != time.Now().Year() {
		t.Fatalf("legacy source was misrepresented: %+v", p)
	}
}

func TestLibraryProvenanceRequiresEvidenceAndKeepsUnitOverrides(t *testing.T) {
	pack := sampleLibraryPack()
	pack.Source.Provenance = &libraryProvenance{Year: 2026, YearKind: "collected", Category: "generated"}
	pack.Units[0].Provenance = &libraryProvenance{Year: 2025, YearKind: "publication", Category: "authentic"}
	if validatePack(pack) == nil {
		t.Fatal("authentic claim accepted without evidence")
	}
	pack.Units[0].Provenance = &libraryProvenance{Year: 2025, YearKind: "season", Category: "recall", Evidence: []string{"https://example.com/reported-2025"}}
	s := libraryTestStore(t)
	id, err := s.importPack(pack)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.loadPackLocked(id)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Units[0].Provenance.Year != 2025 || saved.Units[0].Provenance.Category != "recall" {
		t.Fatal("unit provenance overwritten by pack default")
	}
	for _, category := range []string{"unknown", "unverified", "made-up"} {
		p := *saved.Units[0].Provenance
		p.Category = category
		if validateLibraryProvenance(&p) == nil {
			t.Fatal("accepted unsupported category", category)
		}
	}
}
