// 作成日: 2026-10-18。強度の固定値、終盤の二眼保持、結果の重複防止と調整境界を検証する。
package app

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/Tremendous1192/NobiGo/engine"
)

// TestStrengthBands は各難易度とルール別棋力が順序を保つことを検証する。
func TestStrengthBands(t *testing.T) {
	p := Profile{Ranks: [3]float64{10, 20, 30}}
	s := DefaultSettings()
	last := -100.0
	for i := 0; i < 5; i++ {
		s.Difficulty = i
		v := Strength(p, s)
		if v <= last || DifficultyName(i) == "未設定" {
			t.Fatal(i, v)
		}
		last = v
	}
	s.Difficulty = 1
	s.Rule = Jungo
	if Strength(p, s) != 30 || RankName(0) != "30級" || RankName(29) != "1級" || RankName(38) != "9段" {
		t.Fatal("rank mapping")
	}
}

// TestPureFinishKeepsTwoEyes は三眼の埋め残しだけを埋め、その後は二眼を壊さずパスすることを検証する。
func TestPureFinishKeepsTwoEyes(t *testing.T) {
	b, _ := NewBoard(7, Jungo)
	for i := range b.Cells {
		b.Cells[i] = Black
	}
	for _, p := range []int{8, 10, 12} {
		b.Cells[p] = Empty
	}
	a := engine.Analysis{Moves: []engine.Candidate{{Point: -1}}, Policy: make([]float64, 50)}
	rng := rand.New(rand.NewPCG(1, 2))
	p, err := SelectMove(b, a, 20, rng)
	if err != nil || p < 0 {
		t.Fatalf("safe fill: %d %v", p, err)
	}
	if err = b.Play(p); err != nil {
		t.Fatal(err)
	}
	if err = b.Play(-1); err != nil {
		t.Fatal(err)
	}
	p, err = SelectMove(b, a, 20, rng)
	if err != nil || p != -1 {
		t.Fatalf("two eyes: %d %v", p, err)
	}
}

// TestMoveSelectionLegalAndQuality は占有点の拒否と強度上昇に伴う平均損失低下を検証する。
func TestMoveSelectionLegalAndQuality(t *testing.T) {
	b, _ := NewBoard(7, Japanese)
	a := engine.Analysis{Moves: []engine.Candidate{{Point: 0, ScoreLead: 10, Visits: 10}, {Point: 1, ScoreLead: 0, Visits: 10}}}
	counts := [2]int{}
	for j, strength := range []float64{-2, 44} {
		rng := rand.New(rand.NewPCG(5, 6))
		for i := 0; i < 200; i++ {
			p, err := SelectMove(b, a, strength, rng)
			if err != nil || !b.Legal(p) {
				t.Fatal(p, err)
			}
			if p == 1 {
				counts[j]++
			}
		}
	}
	if counts[0] <= counts[1] || counts[1] != 0 {
		t.Fatal(counts)
	}
	if err := b.Play(0); err != nil {
		t.Fatal(err)
	}
	p, err := SelectMove(b, a, 44, rand.New(rand.NewPCG(1, 2)))
	if err != nil || p != 1 {
		t.Fatal(p, err)
	}
}

// TestCompletedProfileBoundaries は同条件10局の調整、他ルールへの弱い転移、重複排除を検証する。
func TestCompletedProfileBoundaries(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Create("試験")
	if err != nil {
		t.Fatal(err)
	}
	p.Ranks = [3]float64{10, 10, 10}
	original := p
	g := GameResult{Settings: DefaultSettings(), Player: Black, BlackScore: 10, WhiteScore: 6.5, MoveCount: 20}
	for i := 0; i < 10; i++ {
		g.ID = fmt.Sprintf("%032x", i+1)
		p, err = CompletedProfile(p, g)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(original.Results) != 0 || p.GamesPlayed != 10 || p.Wins != 10 || len(p.Adjustments) != 1 || p.Adjustments[0].Offset <= 0 {
		t.Fatal(p)
	}
	if !(p.Ranks[0] > p.Ranks[1] && p.Ranks[1] > 10) {
		t.Fatal(p.Ranks)
	}
	duplicate, err := CompletedProfile(p, g)
	if err != nil || !reflect.DeepEqual(p, duplicate) {
		t.Fatal("duplicate changed profile", err)
	}
	g.ID = fmt.Sprintf("%032x", 11)
	g.NoResult = true
	noResult, err := CompletedProfile(p, g)
	if err != nil || noResult.Ranks != p.Ranks || noResult.Wins != p.Wins {
		t.Fatal("no-result trained", err)
	}
	if err = s.Save(&p); err != nil {
		t.Fatal(err)
	}
}
