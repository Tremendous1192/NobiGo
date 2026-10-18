// 作成日: 2026-10-18。感想戦の説明を合法な棋譜と盤面差分で検証する。外部エンジンは使用しない。
package gui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tremendous1192/PlayGo/engine"
	"github.com/Tremendous1192/PlayGo/internal/app"
)

// adviceFixture は最終手の前後を独立した盤面で用意する。
func adviceFixture(t *testing.T, rule app.Rule, points []int) (*app.Controller, *app.Board, *app.Board) {
	t.Helper()
	b, err := app.NewBoard(7, rule)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range points {
		if err := b.Play(p); err != nil {
			t.Fatal(err)
		}
	}
	before, err := app.Replay(b.Size, rule, b.Moves, len(b.Moves)-1)
	if err != nil {
		t.Fatal(err)
	}
	c := &app.Controller{Board: b, Player: b.Moves[len(b.Moves)-1].Color, Evaluations: make([]*engine.Analysis, len(b.Moves)+1)}
	return c, before, b
}

// TestReviewAutoplayWaitsAndStopsOnError は補完待ちと失敗した局面を自動再生が飛ばさないことを検証する。
func TestReviewAutoplayWaitsAndStopsOnError(t *testing.T) {
	c, _, _ := adviceFixture(t, app.Japanese, []int{0, 1, 7})
	c.Phase = app.Review
	c.Busy = true
	g := &window{controller: c, autoplay: true, reviewTurn: 1}
	g.advanceReview(time.Unix(100, 0))
	if g.reviewTurn != 1 || !g.autoplay {
		t.Fatal("autoplay skipped pending analysis")
	}
	c.Busy = false
	c.Err = errors.New("test analysis failure")
	g.advanceReview(time.Unix(101, 0))
	if g.autoplay || g.reviewTurn != 1 {
		t.Fatal("autoplay skipped failed analysis")
	}
}

// TestMoveLessons は取り・救出・アタリ・連結・自分の危険を区別することを検証する。
func TestMoveLessons(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		points     []int
	}{
		{"capture", "相手の石を1個", []int{0, 1, -1, 7}},
		{"escape", "アタリの石を助け", []int{0, 1, 7}},
		{"atari", "相手の石をアタリ", []int{20, 0, 1}},
		{"connect", "石をつなぎ", []int{0, 48, 2, 47, 1}},
		{"self-atari", "呼吸点が1つ", []int{20, 0, 21, 2, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, before, after := adviceFixture(t, app.Japanese, tc.points)
			effect, lesson := moveLesson(before, after, c.Board.Moves[len(tc.points)-1])
			if !strings.Contains(effect, tc.want) || lesson == "" {
				t.Fatalf("unexpected lesson: %s / %s", effect, lesson)
			}
		})
	}
}

// TestAdviceSeparatesPlayerAndCPU は実際に打った側を基準に説明し、未解析でも事実を表示することを検証する。
func TestAdviceSeparatesPlayerAndCPU(t *testing.T) {
	for _, rule := range []app.Rule{app.Japanese, app.Chinese, app.Jungo} {
		for _, points := range [][]int{{0, 1, 7}, {0, 1, -1, 7}} {
			c, before, after := adviceFixture(t, rule, points)
			turn := len(points)
			own := reviewAdvice(c, turn, before, after)
			c.Player = c.Player.Other()
			cpu := reviewAdvice(c, turn, before, after)
			if !strings.Contains(own, "あなたの手") || !strings.Contains(cpu, "CPUの狙い（推定）") || own == cpu {
				t.Fatalf("wrong perspective: own=%s CPU=%s", own, cpu)
			}
			if strings.Contains(own+cpu, "近似") || strings.Contains(own+cpu, "戻ると解析") || strings.Contains(own+cpu, "比較候補") {
				t.Fatal("repetitive disclaimer, manual prerequisite or unsupported candidate")
			}
		}
	}
}

// TestAdviceCandidateRequiresEvaluations は前後の解析と合法な候補が揃った場合だけ比較候補を示すことを検証する。
func TestAdviceCandidateRequiresEvaluations(t *testing.T) {
	c, before, after := adviceFixture(t, app.Japanese, []int{20, 0, 1})
	c.Evaluations[2] = &engine.Analysis{ScoreLead: 5, Moves: []engine.Candidate{{Point: 2}}}
	if strings.Contains(reviewAdvice(c, 3, before, after), "比較候補") {
		t.Fatal("candidate shown without after evaluation")
	}
	c.Evaluations[3] = &engine.Analysis{ScoreLead: 1}
	if !strings.Contains(reviewAdvice(c, 3, before, after), "比較候補：C7") {
		t.Fatal("candidate missing")
	}
	c.Evaluations[2].Moves[0].Point = 0
	if strings.Contains(reviewAdvice(c, 3, before, after), "比較候補") {
		t.Fatal("occupied candidate shown")
	}
}

// TestPassAdviceUsesRuleAndMover はパスの助言が手番と採点ルールを取り違えないことを検証する。
func TestPassAdviceUsesRuleAndMover(t *testing.T) {
	for _, rule := range []app.Rule{app.Japanese, app.Chinese, app.Jungo} {
		c, before, after := adviceFixture(t, rule, []int{0, -1})
		own := reviewAdvice(c, 2, before, after)
		if strings.Contains(own, "純碁") != (rule == app.Jungo) {
			t.Fatal("wrong rule advice")
		}
		c.Player = app.Black
		if !strings.Contains(reviewAdvice(c, 2, before, after), "CPUはパス") {
			t.Fatal("CPU pass missing")
		}
	}
}
