// 作成日: 2026-10-19。助言の対象と拡大図の独立した局面を作る。解析要求・棋譜変更・保存は行わない。
package gui

import (
	"image"
	"slices"

	"github.com/Tremendous1192/NobiGo/engine"

	"github.com/Tremendous1192/NobiGo/internal/app"
)

type focusTarget struct {
	label string
	point int
}

type diagramGroup struct {
	stones, liberties []int
}

// diagramPosition は固定した石・番号・連と、同じ局面に届いた読取り専用のAI評価を保持する。
type diagramPosition struct {
	size       int
	cells      []app.Color
	numbers    []int
	groups     []diagramGroup
	groupAt    []int
	rule       app.Rule
	moves      []app.Move
	readSource *engine.Analysis
	readReady  bool
	read       tacticalRead
}

type reviewDiagram struct {
	before, after diagramPosition
	targets       []focusTarget
	target        int
	point, center int
	showBefore    bool
}

// currentReviewFocus は同じ手の着目範囲をGUIが所有し、詳細の開閉をまたいで保持する。
func (g *window) currentReviewFocus() *reviewDiagram {
	if g.controller == nil || g.controller.Phase != app.Review || g.reviewBoard == nil {
		return nil
	}
	if g.reviewFocus == nil {
		advice := reviewAdvice(g.controller, g.reviewTurn, g.reviewBefore, g.reviewBoard)
		g.reviewFocus = newReviewDiagram(g.reviewBefore, g.reviewBoard, advice.focus)
	}
	return g.reviewFocus
}

// moveFocusTargets は説明対象を着手・取り・救出・攻撃・連結の順で列挙する。
func moveFocusTargets(before, after *app.Board, move app.Move, facts moveFacts) []focusTarget {
	if before == nil || after == nil || move.Point < 0 || move.Point >= len(after.Cells) {
		return nil
	}
	targets := []focusTarget{{label: "着手した連", point: move.Point}}
	seen := make([]bool, len(before.Cells))
	add := func(label string, point int) {
		if seen[point] {
			return
		}
		stones, _ := before.Group(point)
		for _, p := range stones {
			seen[p] = true
		}
		targets = append(targets, focusTarget{label: label, point: point})
	}
	for p, color := range before.Cells {
		if color == move.Color.Other() && after.Cells[p] == app.Empty {
			add("取られた連", p)
		}
	}
	for _, group := range facts.rescued {
		add("助けた連", group.point)
	}
	for _, group := range facts.attacked {
		add("アタリにした連", group.point)
	}
	if facts.connected >= 2 {
		for _, p := range app.Neighbors(move.Point, before.Size) {
			if before.Cells[p] == move.Color {
				add("つないだ連", p)
			}
		}
	}
	return targets
}

// newDiagramPosition は石と着手番号を複製し、連と共有呼吸点を一度だけ列挙する。
func newDiagramPosition(b *app.Board) diagramPosition {
	if b == nil {
		return diagramPosition{}
	}
	p := diagramPosition{rule: b.Rule, moves: slices.Clone(b.Moves), size: b.Size, cells: slices.Clone(b.Cells), numbers: stoneMoveNumbers(b), groupAt: make([]int, len(b.Cells))}
	for i := range p.groupAt {
		p.groupAt[i] = -1
	}
	for point, color := range p.cells {
		if color == app.Empty || p.groupAt[point] >= 0 {
			continue
		}
		stones, liberties := b.Group(point)
		id := len(p.groups)
		p.groups = append(p.groups, diagramGroup{stones: stones, liberties: liberties})
		for _, n := range stones {
			p.groupAt[n] = id
		}
	}
	return p
}

// newReviewDiagram は助言と同じ対象を固定し、通常は着手後を最初に表示する。
func newReviewDiagram(before, after *app.Board, targets []focusTarget) *reviewDiagram {
	if after == nil {
		return nil
	}
	d := &reviewDiagram{before: newDiagramPosition(before), after: newDiagramPosition(after), targets: slices.Clone(targets), point: -1, center: after.Size * after.Size / 2}
	if len(targets) > 0 {
		d.selectTarget(0)
	}
	return d
}

// position は現在選択した着手前後の読取り専用局面を返す。
func (d *reviewDiagram) position() *diagramPosition {
	if d.showBefore && d.before.size > 0 {
		return &d.before
	}
	return &d.after
}

// selectTarget は説明対象の位置へ表示を移し、前後の選択を維持する。
func (d *reviewDiagram) selectTarget(index int) {
	if index < 0 || index >= len(d.targets) {
		return
	}
	d.target = index
	d.point = d.targets[index].point
	d.center = d.point
}

// selectedGroup は選択石の連を返す。着手前の空点・取り後の空点から連を作らない。
func (d *reviewDiagram) selectedGroup() (diagramGroup, bool) {
	p := d.position()
	if d.point < 0 || d.point >= len(p.groupAt) || p.groupAt[d.point] < 0 {
		return diagramGroup{}, false
	}
	return p.groups[p.groupAt[d.point]], true
}

// selectStone は拡大図の石を選択する。空点や盤外は変更しない。
func (d *reviewDiagram) selectStone(point int) bool {
	p := d.position()
	if point < 0 || point >= len(p.cells) || p.cells[point] == app.Empty {
		return false
	}
	d.point = point
	return true
}

// viewport は着目点の周囲5路を盤内へ収める。遠い連の石は総数と範囲外表示で補う。
func (d *reviewDiagram) viewport() image.Rectangle {
	size := d.position().size
	span := min(5, size)
	center := max(0, min(d.center, size*size-1))
	x, y := max(0, min(center%size-span/2, size-span)), max(0, min(center/size-span/2, size-span))
	return image.Rect(x, y, x+span, y+span)
}

// outsideCounts は図の外にある選択連の石数と呼吸点数を、盤全体の集合から数える。
func (d *reviewDiagram) outsideCounts() (stones, liberties int) {
	group, ok := d.selectedGroup()
	if !ok {
		return
	}
	r, size := d.viewport(), d.position().size
	for _, p := range group.stones {
		if !image.Pt(p%size, p/size).In(r) {
			stones++
		}
	}
	for _, p := range group.liberties {
		if !image.Pt(p%size, p/size).In(r) {
			liberties++
		}
	}
	return
}

// pan は部分図を盤内で移動し、選択した連と着手前後を維持する。
func (d *reviewDiagram) pan(dx, dy int) {
	r, size := d.viewport(), d.position().size
	x := max(0, min(r.Min.X+dx, size-r.Dx()))
	y := max(0, min(r.Min.Y+dy, size-r.Dy()))
	d.center = (y+r.Dy()/2)*size + x + r.Dx()/2
}
