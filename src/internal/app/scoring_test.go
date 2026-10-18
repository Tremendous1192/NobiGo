// 作成日: 2026-10-18。採点方式・死石補正・セキ・純碁の非除去を検証する。
package app

import (
	"slices"
	"testing"
)

// enclosedBoard は二眼の黒に囲まれた死石候補を配置する。
func enclosedBoard(t *testing.T, rule Rule) *Board {
	t.Helper()
	b, err := NewBoard(7, rule)
	if err != nil {
		t.Fatal(err)
	}
	for i := range b.Cells {
		b.Cells[i] = Black
	}
	for _, p := range []int{8, 25, 40} {
		b.Cells[p] = Empty
	}
	b.Cells[24] = White
	b.Captured[Black] = 2
	return b
}

// TestScoringRules は同じ局面でルールごとの得点内訳が異なることを検証する。
func TestScoringRules(t *testing.T) {
	for _, tc := range []struct {
		rule         Rule
		black, white float64
	}{{Japanese, 7, 6.5}, {Chinese, 49, 7.5}, {Jungo, 45, 1}} {
		t.Run(string(tc.rule), func(t *testing.T) {
			b := enclosedBoard(t, tc.rule)
			before := slices.Clone(b.Cells)
			marks := map[int]bool{24: true}
			if tc.rule == Jungo {
				marks = nil
			}
			s, err := ScoreBoard(b, marks, nil)
			if err != nil {
				t.Fatal(err)
			}
			if s.Black != tc.black || s.White != tc.white {
				t.Fatalf("score = %v:%v, want %v:%v", s.Black, s.White, tc.black, tc.white)
			}
			if !slices.Equal(before, b.Cells) || b.Captured[Black] != 2 {
				t.Fatal("scoring changed original position")
			}
		})
	}
}

// TestJungoDrawAndDeadRejection は純碁の引き分けと死石除去の禁止を検証する。
func TestJungoDrawAndDeadRejection(t *testing.T) {
	b, _ := NewBoard(7, Jungo)
	s, err := ScoreBoard(b, nil, nil)
	if err != nil || s.Winner != Empty || s.Margin != 0 {
		t.Fatal("empty jungo board is not draw")
	}
	if err = b.Play(0); err != nil {
		t.Fatal(err)
	}
	if _, err = ScoreBoard(b, map[int]bool{0: true}, nil); err == nil {
		t.Fatal("jungo accepted dead stone removal")
	}
}

// TestSekiCorrection は日本式のセキの地を除外し、明示的な解除を受け付ける。
func TestSekiCorrection(t *testing.T) {
	b, _ := NewBoard(7, Japanese)
	for p := range b.Cells {
		x, y := p%7, p/7
		if x < 3 || x == 3 && y < 2 {
			b.Cells[p] = Black
		} else {
			b.Cells[p] = White
		}
	}
	for _, p := range []int{8, 12, 17, 24} {
		b.Cells[p] = Empty
	}
	s, err := ScoreBoard(b, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Territory[Black] != 0 || s.Territory[White] != 0 {
		t.Fatal("seki eyes were counted")
	}
	s, err = ScoreBoard(b, nil, map[int]bool{0: false, 6: false})
	if err != nil {
		t.Fatal(err)
	}
	if s.Territory[Black] != 1 || s.Territory[White] != 1 {
		t.Fatal("seki override was not applied")
	}
	b.Rule = Chinese
	s, err = ScoreBoard(b, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Territory[Black] != 1 || s.Territory[White] != 1 {
		t.Fatal("chinese eyes were excluded")
	}
}

// TestDeadSuggestion は候補判定と確定採点が独立することを検証する。
func TestDeadSuggestion(t *testing.T) {
	b := enclosedBoard(t, Japanese)
	owner := make([]float64, 49)
	for i := range owner {
		owner[i] = .95
	}
	marks := SuggestDead(b, owner)
	if !marks[24] {
		t.Fatal("expected white dead-stone proposal")
	}
	if len(SuggestDead(b, nil)) != 0 {
		t.Fatal("missing analysis produced proposal")
	}
	b.Rule = Jungo
	if len(SuggestDead(b, owner)) != 0 {
		t.Fatal("jungo produced dead-stone proposal")
	}
}

// TestInvalidScoreSelections は空点・範囲外・矛盾したセキ指定を拒否する。
func TestInvalidScoreSelections(t *testing.T) {
	b := enclosedBoard(t, Japanese)
	for _, point := range []int{-1, 49, 8} {
		if _, err := ScoreBoard(b, map[int]bool{point: true}, nil); err == nil {
			t.Fatalf("accepted invalid dead selection %d", point)
		}
	}
	if _, err := ScoreBoard(b, nil, map[int]bool{0: true, 1: false}); err == nil {
		t.Fatal("accepted contradictory group flags")
	}
	b.NoResult = true
	s, err := ScoreBoard(b, nil, nil)
	if err != nil || s.Winner != Empty || !s.NoResult {
		t.Fatal("no-result became a win")
	}
}
