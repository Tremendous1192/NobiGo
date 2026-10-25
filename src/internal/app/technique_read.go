// 作成日: 2026-10-25。技術の複数手を有限探索する。探索打切り・コウ循環を成功とせず、実棋譜は変更しない。
package app

import (
	"context"
	"slices"
)

const techniqueNodeLimit = 8192
const techniqueDepthLimit = 64

type captureVerdict uint8

const (
	captureUnknown captureVerdict = iota
	captureForced
	captureEscapes
)

// captureReader は1候補だけの探索予算と対象石を所有する。呼出し元の作業内で順次使用する。
type captureReader struct {
	attacker               Color
	targets                []int
	nodes                  int
	limit                  int
	net                    bool
	minX, maxX, minY, maxY int
}

// targetGroup は元の対象石が残っている連を返す。既に取った場所への新しい石は追跡しない。
func (r *captureReader) targetGroup(b *Board) ([]int, []int) {
	for _, p := range r.targets {
		if b.Cells[p] == r.attacker.Other() {
			return b.Group(p)
		}
	}
	return nil, nil
}

// solve は攻め側に成功手があり、守り側の必要な全応手にも成功する場合だけ捕獲を認める。
func (r *captureReader) solve(ctx context.Context, b *Board, depth int) (captureVerdict, []int) {
	r.nodes++
	if ctx.Err() != nil || r.nodes > r.limit {
		return captureUnknown, nil
	}
	group, liberties := r.targetGroup(b)
	if len(group) == 0 {
		return captureForced, nil
	}
	if depth == 0 || b.NoResult || b.Passes >= 2 {
		return captureUnknown, nil
	}
	maxLiberties := 2
	if r.net {
		maxLiberties = 3
	}
	if len(liberties) > maxLiberties {
		return captureEscapes, nil
	}
	if r.net {
		for _, p := range group {
			x, y := p%b.Size, p/b.Size
			if x < r.minX || x > r.maxX || y < r.minY || y > r.maxY {
				return captureEscapes, nil
			}
		}
	}
	attacking := b.ToMove == r.attacker
	points := slices.Clone(liberties)
	if !attacking {
		// 離れた場所の捕獲や反復無勝負も応手になるため、局所の逃げだけでは成立を断定しない。
		for p, c := range b.Cells {
			if c == Empty && !slices.Contains(points, p) {
				points = append(points, p)
			}
		}
		points = append(points, -1)
	}
	unknown := false
	var longest []int
	legal := 0
	for _, point := range points {
		r.nodes++
		if ctx.Err() != nil || r.nodes > r.limit {
			return captureUnknown, nil
		}
		if b.Play(point) != nil {
			continue
		}
		legal++
		verdict, line := r.solve(ctx, b, depth-1)
		if !b.Undo(1) {
			return captureUnknown, nil
		}
		if attacking && verdict == captureForced {
			return captureForced, append([]int{point}, line...)
		}
		if !attacking && verdict == captureEscapes {
			return captureEscapes, nil
		}
		if verdict == captureUnknown {
			unknown = true
		}
		if verdict == captureForced && len(line)+1 > len(longest) {
			longest = append([]int{point}, line...)
		}
	}
	if unknown || legal == 0 {
		return captureUnknown, nil
	}
	if attacking {
		return captureEscapes, nil
	}
	return captureForced, longest
}

// ReadTechniques は両アタリ・シチョウ・ゲタ・条件付きウッテガエシを独立盤で確認する。
func ReadTechniques(ctx context.Context, b *Board, point, turn int) []TechniqueLesson {
	if ctx.Err() != nil || b == nil || point < 0 || turn < 1 || !b.Legal(point) {
		return nil
	}
	next := cloneTechniqueBoard(b)
	if next.Play(point) != nil {
		return nil
	}
	var result []TechniqueLesson
	if lesson := snapbackLesson(b, next, point, turn); lesson != nil {
		result = append(result, *lesson)
	}
	_, own := next.Group(point)
	if len(own) < 2 {
		return result
	}
	var atariGroups [][]int
	for _, target := range techniqueGroups(b, b.ToMove.Other()) {
		if next.Cells[target[0]] != b.ToMove.Other() {
			continue
		}
		_, old := b.Group(target[0])
		_, libs := next.Group(target[0])
		if len(libs) == 1 && len(old) > 1 {
			atariGroups = append(atariGroups, target)
		}
		if len(old) != 2 || len(libs) > 2 {
			continue
		}
		net := len(libs) == 2
		near, adjacent := false, false
		minX, maxX, minY, maxY := b.Size, 0, b.Size, 0
		for _, p := range target {
			x, y := p%b.Size, p/b.Size
			minX, maxX, minY, maxY = min(minX, x), max(maxX, x), min(minY, y), max(maxY, y)
			dx, dy := x-point%b.Size, y-point/b.Size
			distance := max(dx, -dx) + max(dy, -dy)
			near = near || distance == 2
			adjacent = adjacent || distance == 1
		}
		if net && (!near || adjacent) {
			continue
		}
		reader := captureReader{attacker: b.ToMove, targets: target, limit: techniqueNodeLimit, net: net, minX: minX - 2, maxX: maxX + 2, minY: minY - 2, maxY: maxY + 2}
		depth := techniqueDepthLimit - 1
		if net {
			depth = 9
		}
		verdict, line := reader.solve(ctx, next, depth)
		if verdict != captureForced || len(line) < 2 {
			continue
		}
		if !net {
			escapes := 0
			for i, p := range line {
				if i%2 == 0 && p >= 0 {
					escapes++
				}
			}
			if escapes < 2 {
				continue
			}
		}
		id := TechniqueLadder
		if net {
			id = TechniqueNet
		}
		result = append(result, TechniqueLesson{ID: id, Turn: turn, Color: b.ToMove, Line: append([]int{point}, line...), Targets: slices.Clone(target), Stones: len(target)})
	}
	if len(atariGroups) >= 2 {
		if line, targets := doubleAtariLine(ctx, next, b.ToMove, atariGroups); len(line) > 0 {
			result = append(result, TechniqueLesson{ID: TechniqueDoubleAtari, Turn: turn, Color: b.ToMove, Line: append([]int{point}, line...), Targets: targets, Stones: len(targets), Groups: len(atariGroups)})
		}
	}
	return result
}

// doubleAtariLine は盤全体の合法な応手に対し、元の対象のどれかを次に取れることを確認する。
func doubleAtariLine(ctx context.Context, b *Board, attacker Color, groups [][]int) ([]int, []int) {
	var targets, example []int
	var escapes []int
	for _, group := range groups {
		_, liberties := b.Group(group[0])
		if len(liberties) != 1 || slices.Contains(escapes, liberties[0]) {
			return nil, nil
		}
		escapes = append(escapes, liberties[0])
		targets = append(targets, group...)
	}
	for reply := -1; reply < len(b.Cells); reply++ {
		if ctx.Err() != nil {
			return nil, nil
		}
		if b.Play(reply) != nil {
			continue
		}
		capture := -2
		for _, group := range groups {
			if b.Cells[group[0]] != attacker.Other() {
				continue
			}
			_, libs := b.Group(group[0])
			if len(libs) != 1 || b.Play(libs[0]) != nil {
				continue
			}
			removed := b.Cells[group[0]] == Empty
			if !b.Undo(1) {
				return nil, nil
			}
			if removed {
				capture = libs[0]
				break
			}
		}
		if !b.Undo(1) || capture == -2 {
			return nil, nil
		}
		if example == nil || slices.Contains(escapes, reply) {
			example = []int{reply, capture}
		}
	}
	return example, targets
}

// snapbackLesson は取らせて同じ点でより多く取り返す合法な三手を確認する。相手が取ることを強制する意味ではない。
func snapbackLesson(before, after *Board, point, turn int) *TechniqueLesson {
	group, libs := after.Group(point)
	if len(libs) != 1 || after.Captured[before.ToMove] != before.Captured[before.ToMove] {
		return nil
	}
	b := cloneTechniqueBoard(after)
	if b.Play(libs[0]) != nil || b.Cells[point] != Empty {
		return nil
	}
	old := b.Captured[before.ToMove]
	var targets []int
	for p, c := range b.Cells {
		if c == before.ToMove.Other() {
			targets = append(targets, p)
		}
	}
	if b.Play(point) != nil || b.Captured[before.ToMove]-old <= len(group) {
		return nil
	}
	targets = slices.DeleteFunc(targets, func(p int) bool { return b.Cells[p] != Empty })
	return &TechniqueLesson{ID: TechniqueSnapback, Turn: turn, Color: before.ToMove, Line: []int{point, libs[0], point}, Targets: targets, Stones: len(targets)}
}
