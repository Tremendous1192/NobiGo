// 作成日: 2026-10-19。概要の選定・視点・欠損・棋譜不変を検証する。実エンジンは使用しない。
package app

import (
	"reflect"
	"testing"

	"github.com/Tremendous1192/NobiGo/engine"
)

// summaryFixture は合法な短い棋譜と、手の優劣が既知の評価を用意する。
func summaryFixture(t *testing.T, rule Rule) (*Board, []*engine.Analysis) {
	t.Helper()
	b, err := NewBoard(7, rule)
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range []int{16, 0, 24, 1, 32, 2} {
		if err = b.Play(point); err != nil {
			t.Fatal(err)
		}
	}
	scores := []float64{0, 0, 0, -4, -2, -2, -1}
	a := make([]*engine.Analysis, len(scores))
	for i, score := range scores {
		a[i] = &engine.Analysis{ScoreLead: score, Winrate: .5 + score/100, Moves: []engine.Candidate{{Point: 40, Visits: 32}}}
	}
	a[0].Moves[0].Point = 16
	return b, a
}

// TestSummarySelectionAndPerspective は同じ棋譜でも自分の色に応じて好手と改善候補が切り替わることを検証する。
func TestSummarySelectionAndPerspective(t *testing.T) {
	for _, rule := range []Rule{Japanese, Chinese, Jungo} {
		b, a := summaryFixture(t, rule)
		before, _ := Replay(b.Size, b.Rule, b.Moves, len(b.Moves))
		s, err := BuildGameSummary(b, Black, a)
		if err != nil {
			t.Fatal(err)
		}
		if s.Highlight == nil || s.Highlight.Turn != 3 || s.GoodMove == nil || s.GoodMove.Turn != 1 || s.Improvement == nil || s.Improvement.Turn != 3 || s.Improvement.Loss != 4 {
			t.Fatalf("wrong selection: %+v", s)
		}
		w, err := BuildGameSummary(b, White, a)
		if err != nil {
			t.Fatal(err)
		}
		if w.Improvement == nil || w.Improvement.Turn != 4 || w.Improvement.Loss != 2 || w.Moves[3].Swing != -2 {
			t.Fatal("white perspective lost")
		}
		if !reflect.DeepEqual(b, before) || s.Evaluated != 6 || s.Stages[0].CPUMoves != 1 {
			t.Fatal("source or stage changed")
		}
	}
}

// TestSummaryMissingAndIllegalCandidates は欠損・途中・低探索・違法候補から優劣を断定しないことを検証する。
func TestSummaryMissingAndIllegalCandidates(t *testing.T) {
	for _, mode := range []string{"missing", "partial", "low", "illegal"} {
		t.Run(mode, func(t *testing.T) {
			b, a := summaryFixture(t, Japanese)
			for i := range a {
				switch mode {
				case "missing":
					a[i] = nil
				case "partial":
					a[i].Partial = true
				case "low":
					a[i].Moves[0].Visits = 1
				case "illegal":
					a[i].Moves[0].Point = 16
				}
			}
			s, err := BuildGameSummary(b, Black, a)
			if err != nil {
				t.Fatal(err)
			}
			if s.Improvement != nil {
				t.Fatal("unsupported comparison")
			}
			if mode != "illegal" && (s.Evaluated != 0 || s.Highlight != nil || s.GoodMove != nil) {
				t.Fatal("missing analysis treated as zero loss")
			}
		})
	}
}

// TestSummaryEmptyPassAndInvalid は空・連続パス・不正棋譜と純碁の引き分けを安全に扱うことを検証する。
func TestSummaryEmptyPassAndInvalid(t *testing.T) {
	b, _ := NewBoard(7, Jungo)
	s, err := BuildGameSummary(b, White, nil)
	if err != nil || len(s.Moves) != 0 {
		t.Fatal("empty summary", err)
	}
	if err = b.Play(-1); err != nil {
		t.Fatal(err)
	}
	if err = b.Play(-1); err != nil {
		t.Fatal(err)
	}
	s, err = BuildGameSummary(b, White, nil)
	if err != nil || s.GoodMove != nil || s.Improvement != nil {
		t.Fatal("pass summary", err)
	}
	score, err := ScoreBoard(b, nil, nil)
	if err != nil || score.Winner != Empty {
		t.Fatal("draw changed")
	}
	b.Moves[0].Color = White
	if _, err = BuildGameSummary(b, White, nil); err == nil {
		t.Fatal("invalid color accepted")
	}
}

// TestSummaryCombinedEffectsAndBound は救出の重複集計・自分のアタリの称賛・上限超過を防ぐことを検証する。
func TestSummaryCombinedEffectsAndBound(t *testing.T) {
	for _, tc := range []struct {
		points                     []int
		saved, connected, captured int
		danger                     bool
	}{
		{[]int{0, 1, 14, 48, 7}, 1, 1, 0, false},
		{[]int{2, 1, 8, 7, 0}, 0, 0, 1, true},
	} {
		b, _ := NewBoard(7, Japanese)
		for _, point := range tc.points {
			if err := b.Play(point); err != nil {
				t.Fatal(err)
			}
		}
		a := make([]*engine.Analysis, len(b.Moves)+1)
		for i := range a {
			a[i] = &engine.Analysis{Winrate: .5, Moves: []engine.Candidate{{Point: tc.points[len(tc.points)-1], Visits: 20}}}
		}
		s, err := BuildGameSummary(b, Black, a)
		if err != nil {
			t.Fatal(err)
		}
		last := s.Moves[len(s.Moves)-1]
		if last.Saved != tc.saved || last.Connected != tc.connected || last.Captured != tc.captured || last.SelfAtari != tc.danger {
			t.Fatalf("combined effects=%+v", last)
		}
		if tc.danger && s.GoodMove != nil && s.GoodMove.Turn == len(b.Moves) {
			t.Fatal("self-atari was praised")
		}
	}
	b, _ := NewBoard(7, Japanese)
	b.Moves = make([]Move, MaxMoves+1)
	if _, err := BuildGameSummary(b, Black, nil); err == nil {
		t.Fatal("oversized summary accepted")
	}
}
