// 作成日: 2026-10-24。概要で使う独立した前後局面と一手比較を用意する。実棋譜・解析・プロフィールを変更しない。
package gui

import (
	"errors"
	"math"

	"github.com/Tremendous1192/NobiGo/internal/app"
)

// summaryScene は表示対象の局面だけを所有する。候補は合法に再現できた場合だけ保持する。
type summaryScene struct {
	before, after, alternative *app.Board
	facts, alternativeFacts    moveFacts
	comparison                 *summaryGroupComparison
}

// summaryGroupComparison は着手前に存在した同じ味方の石群を二つの手で比較する。
type summaryGroupComparison struct {
	point, stones, beforeLiberties, playedLiberties, candidateLiberties int
}

// makeSummaryScene は選定手を棋譜と照合し、履歴付きの独立盤面で候補を一手だけ再現する。
func makeSummaryScene(c *app.Controller, move *app.SummaryMove) (summaryScene, error) {
	var scene summaryScene
	if c == nil || c.Board == nil || move == nil || move.Turn < 1 || move.Turn > len(c.Board.Moves) {
		return scene, errors.New("invalid summary scene")
	}
	m := c.Board.Moves[move.Turn-1]
	if m.Point != move.Point || m.Color != move.Color {
		return scene, errors.New("summary move does not match game")
	}
	var err error
	scene.before, err = c.Snapshot(move.Turn - 1)
	if err != nil {
		return scene, err
	}
	scene.after, err = c.Snapshot(move.Turn)
	if err != nil {
		return scene, err
	}
	scene.facts = collectMoveFacts(scene.before, scene.after, m)
	if move.HasAlternative && move.Alternative != move.Point {
		candidate, err := c.Snapshot(move.Turn - 1)
		if err != nil {
			return scene, err
		}
		// コウの履歴を失うセル配列のコピーだけでは、打てない候補を説明してしまう。
		if candidate.Play(move.Alternative) == nil {
			scene.alternative = candidate
			scene.alternativeFacts = collectMoveFacts(scene.before, candidate, app.Move{Color: m.Color, Point: move.Alternative})
			scene.comparison = compareSummaryGroup(scene.before, scene.after, candidate, m.Color)
		}
	}
	return scene, nil
}

// compareSummaryGroup は同じ味方の石群の空きが変わる場面を選ぶ。数の差だけで手の優劣を決めない。
func compareSummaryGroup(before, played, candidate *app.Board, color app.Color) *summaryGroupComparison {
	var best *summaryGroupComparison
	weight := 0
	seen := make([]bool, len(before.Cells))
	for point, stone := range before.Cells {
		if stone != color || seen[point] {
			continue
		}
		group, old := before.Group(point)
		for _, p := range group {
			seen[p] = true
		}
		_, actual := played.Group(point)
		_, other := candidate.Group(point)
		if len(actual) == len(other) {
			continue
		}
		w := int(math.Abs(float64(len(actual) - len(other))))
		if len(old) == 1 || len(actual) == 1 || len(other) == 1 {
			w += len(before.Cells)
		}
		if best == nil || w > weight {
			best = &summaryGroupComparison{point, len(group), len(old), len(actual), len(other)}
			weight = w
		}
	}
	return best
}

// summaryMarkedPoints は本文で扱う元の石を選ぶ。取った石は着手後の空点にも印を残す。
func summaryMarkedPoints(s summaryScene, move *app.SummaryMove, comparison bool) []int {
	marked := make([]bool, len(s.before.Cells))
	addGroup := func(point int) {
		group, _ := s.before.Group(point)
		for _, p := range group {
			marked[p] = true
		}
	}
	addEffects := func(facts moveFacts, after *app.Board, point int) {
		for p, color := range s.before.Cells {
			if color == move.Color.Other() && after.Cells[p] == app.Empty {
				marked[p] = true
			}
		}
		for _, groups := range [][]changedGroup{facts.rescued, facts.attacked} {
			for _, group := range groups {
				addGroup(group.point)
			}
		}
		if facts.connected >= 2 {
			for _, p := range app.Neighbors(point, s.before.Size) {
				if s.before.Cells[p] == move.Color {
					addGroup(p)
				}
			}
		}
	}
	addEffects(s.facts, s.after, move.Point)
	if comparison && s.alternative != nil {
		addEffects(s.alternativeFacts, s.alternative, move.Alternative)
		if s.comparison != nil {
			addGroup(s.comparison.point)
		}
	}
	var points []int
	for point, selected := range marked {
		if selected {
			points = append(points, point)
		}
	}
	return points
}
