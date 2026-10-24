// 作成日: 2026-10-24。概要の一手比較・対象石の同一性・コウ・棋譜不変を検証する。実エンジンは使わない。
package gui

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Tremendous1192/NobiGo/internal/app"
)

// TestSummarySceneComparesSameGroup は救出と連結を区別し、同じ元の石群を候補と比較することを検証する。
func TestSummarySceneComparesSameGroup(t *testing.T) {
	for _, rule := range []app.Rule{app.Japanese, app.Chinese, app.Jungo} {
		c, before, after := adviceFixture(t, rule, []int{0, 1, 14, 48, 7})
		m := &app.SummaryMove{Turn: 5, Point: 7, Color: app.Black, HasAlternative: true, Alternative: 40}
		s, err := makeSummaryScene(c, m)
		if err != nil {
			t.Fatal(err)
		}
		if s.facts.connected != 2 || len(s.facts.rescued) != 1 || s.facts.rescued[0].stones != 1 {
			t.Fatalf("wrong combined facts: %+v", s.facts)
		}
		g := s.comparison
		if g == nil || g.point != 0 || g.stones != 1 || g.beforeLiberties != 1 || g.playedLiberties != 3 || g.candidateLiberties != 1 {
			t.Fatalf("different groups compared: %+v", g)
		}
		if !reflect.DeepEqual(c.Board, after) || !reflect.DeepEqual(s.before, before) || !reflect.DeepEqual(s.after, after) {
			t.Fatal("source board changed")
		}
		s.alternative.Cells[0] = app.Empty
		if !reflect.DeepEqual(c.Board, after) || !reflect.DeepEqual(s.before, before) || !reflect.DeepEqual(s.after, after) {
			t.Fatal("candidate aliases original position")
		}
	}
}

// TestSummaryMarksMatchExplainedStones は救出・連結・取り石・候補で説明する元の石だけを強調することを検証する。
func TestSummaryMarksMatchExplainedStones(t *testing.T) {
	for _, tc := range []struct {
		points      []int
		alternative int
		comparison  bool
		want        []int
	}{
		{[]int{0, 1, 14, 48, 7}, 40, false, []int{0, 14}},
		{[]int{0, 1, 14, 48, 40}, 7, true, []int{0, 14}},
		{[]int{2, 1, 8, 7, 0}, 48, false, []int{1, 7}},
		{[]int{0, -1}, 48, false, nil},
	} {
		c, _, _ := adviceFixture(t, app.Japanese, tc.points)
		last := c.Board.Moves[len(tc.points)-1]
		m := &app.SummaryMove{Turn: len(tc.points), Point: last.Point, Color: last.Color, HasAlternative: true, Alternative: tc.alternative}
		scene, err := makeSummaryScene(c, m)
		if err != nil {
			t.Fatal(err)
		}
		if got := summaryMarkedPoints(scene, m, tc.comparison); !slices.Equal(got, tc.want) {
			t.Fatalf("wrong marked stones: got %v, want %v", got, tc.want)
		}
	}
}

// TestSummarySceneRejectsIllegalCandidates はコウ・占有・盤外・同じ手の候補を採用しないことを検証する。
func TestSummarySceneRejectsIllegalCandidates(t *testing.T) {
	c, _, _ := adviceFixture(t, app.Japanese, []int{2, 1, 8, 7, 0, 48})
	for _, point := range []int{1, 2, 49, -2, 48} {
		m := &app.SummaryMove{Turn: 6, Point: 48, Color: app.White, HasAlternative: true, Alternative: point}
		s, err := makeSummaryScene(c, m)
		if err != nil || s.alternative != nil || s.comparison != nil {
			t.Fatal("unsupported candidate accepted", point, err)
		}
	}
	for _, m := range []*app.SummaryMove{nil, {Turn: 0}, {Turn: 7}, {Turn: 6, Point: 48, Color: app.Black}} {
		if _, err := makeSummaryScene(c, m); err == nil {
			t.Fatal("invalid scene accepted")
		}
	}
}

// TestSummaryScenePassAndWhite は白の比較・候補のパス・実手のパスを石の変化と混同しないことを検証する。
func TestSummaryScenePassAndWhite(t *testing.T) {
	c, _, _ := adviceFixture(t, app.Jungo, []int{0, 1, 7, -1})
	s, err := makeSummaryScene(c, &app.SummaryMove{Turn: 4, Point: -1, Color: app.White, HasAlternative: true, Alternative: 8})
	if err != nil || s.alternative == nil || s.facts.stones != 0 || s.alternativeFacts.stones != 2 {
		t.Fatal("white pass comparison failed", err)
	}
	s, err = makeSummaryScene(c, &app.SummaryMove{Turn: 2, Point: 1, Color: app.White, HasAlternative: true, Alternative: -1})
	if err != nil || s.alternative == nil || s.alternative.Passes != 1 || s.alternativeFacts.stones != 0 {
		t.Fatal("candidate pass was described as stones", err)
	}
}
