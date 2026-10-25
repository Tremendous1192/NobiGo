// 作成日: 2026-10-25。旧保存形式の移行・専用バックアップ・紹介履歴の冪等性と失敗時の保持を検証する。
package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestTechniqueProfileMigration はv1を読めて、最初の更新前の内容を後続保存でも保持することを検証する。
func TestTechniqueProfileMigration(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Create("Migration fixture")
	if err != nil {
		t.Fatal(err)
	}
	p.Version = 1
	legacy, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	path, _ := s.path(p.ID, ".json")
	if err = os.WriteFile(path, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	p, err = s.Load(p.ID)
	if err != nil || p.Version != 1 || len(p.Techniques) != 0 {
		t.Fatal("legacy load", err)
	}
	ranks := p.Ranks
	if err = s.Save(&p); err != nil || p.Version != 2 || p.Ranks != ranks {
		t.Fatal("migration", err)
	}
	p.Name = "Renamed fixture"
	if err = s.Save(&p); err != nil {
		t.Fatal(err)
	}
	backup, _ := s.path(p.ID, ".v1.bak")
	saved, err := os.ReadFile(backup)
	if err != nil || !bytes.Equal(saved, legacy) {
		t.Fatal("migration backup replaced", err)
	}
}

// TestTechniqueRecording は未表示時不変・表示後一度・再表示固定・競合時保持・利用者分離を検証する。
func TestTechniqueRecording(t *testing.T) {
	c, _ := prepareReviewTest(t)
	c.Phase = Summary
	c.SummarySession = &SummarySession{Ready: true, Lessons: []TechniqueLesson{{ID: TechniqueCapture, Turn: 1, Color: Black, Line: []int{0}}}}
	original := c.Profile
	chosen := c.ChooseTechnique()
	if chosen == nil || c.Profile.Revision != original.Revision {
		t.Fatal("selection saved profile")
	}
	lock := filepath.Join(c.store.Dir, "store.lock")
	if err := os.WriteFile(lock, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.RecordTechniqueShown(); !errors.Is(err, ErrStoreBusy) || !reflect.DeepEqual(c.Profile, original) {
		t.Fatal("failed save changed profile", err)
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if err := c.RecordTechniqueShown(); err != nil {
		t.Fatal(err)
	}
	revision := c.Profile.Revision
	if len(c.Profile.Techniques) != 1 || c.Profile.Techniques[0].Count != 1 {
		t.Fatal("missing history")
	}
	if c.Profile.Ranks != original.Ranks || c.Profile.GamesPlayed != original.GamesPlayed || !reflect.DeepEqual(c.Profile.Results, original.Results) {
		t.Fatal("technique retrained game")
	}
	if err := c.RecordTechniqueShown(); err != nil || c.Profile.Revision != revision {
		t.Fatal("duplicate save")
	}
	c.SummarySession = &SummarySession{Ready: true, Lessons: []TechniqueLesson{{ID: TechniqueEscape, Turn: 2, Color: White, Line: []int{1}}}}
	if c.ChooseTechnique() != chosen {
		t.Fatal("reopened lesson changed")
	}
	other, err := c.store.Create("Other fixture")
	if err != nil || len(other.Techniques) != 0 {
		t.Fatal("history crossed players", err)
	}
	stale := original
	if err = c.store.Save(&stale); err == nil {
		t.Fatal("stale revision overwritten")
	}
}

// TestTechniqueProfileValidation は新しいJSON境界と未知形式を拒否することを検証する。
func TestTechniqueProfileValidation(t *testing.T) {
	s, _ := OpenStore(t.TempDir())
	p, err := s.Create("Validation fixture")
	if err != nil {
		t.Fatal(err)
	}
	p.LastTechniqueGame = p.ID
	for _, records := range [][]TechniqueRecord{
		{{ID: "unknown", Count: 1, LastShown: 1}},
		{{ID: TechniqueCapture, Count: 0, LastShown: 1}},
		{{ID: TechniqueCapture, Count: 1, LastShown: 2}},
		{{ID: TechniqueCapture, Count: 1, LastShown: 1}, {ID: TechniqueCapture, Count: 1, LastShown: 2}},
		{{ID: TechniqueCapture, Count: 1, LastShown: 1}, {ID: TechniqueEscape, Count: 1, LastShown: 1}},
	} {
		p.Techniques = records
		if p.Validate() == nil {
			t.Fatal("invalid history accepted", records)
		}
	}
	p.Techniques = []TechniqueRecord{{ID: TechniqueCapture, Count: 1, LastShown: 1}}
	if err = p.Validate(); err != nil {
		t.Fatal(err)
	}
	p.Version = 1
	if p.Validate() == nil {
		t.Fatal("legacy accepted new fields")
	}
	p.Version = 99
	if s.Save(&p) == nil {
		t.Fatal("unknown version migrated")
	}
}
