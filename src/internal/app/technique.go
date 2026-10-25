// 作成日: 2026-10-25。感想戦の技術候補と紹介順位を扱う。実棋譜を変更せず、確認した効果だけを教材にする。
package app

import (
	"slices"
)

// TechniqueID は保存後も変更しない教材の識別子。
type TechniqueID string

const (
	TechniqueCapture     TechniqueID = "capture"
	TechniqueEscape      TechniqueID = "escape"
	TechniqueConnect     TechniqueID = "connect"
	TechniqueAtari       TechniqueID = "atari"
	TechniqueDoubleAtari TechniqueID = "double-atari"
	TechniqueLadder      TechniqueID = "ladder"
	TechniqueNet         TechniqueID = "net"
	TechniqueSnapback    TechniqueID = "snapback"
)

// TechniqueIDs は初期教材を表示順の独立配列で返す。
func TechniqueIDs() []TechniqueID {
	return []TechniqueID{TechniqueCapture, TechniqueEscape, TechniqueConnect, TechniqueAtari, TechniqueDoubleAtari, TechniqueLadder, TechniqueNet, TechniqueSnapback}
}

// TechniqueRecord は全ルール共通の紹介回数と紹介順を保存する。LastShownは日時ではなく通算紹介順。
type TechniqueRecord struct {
	ID        TechniqueID `json:"id"`
	Count     uint32      `json:"count"`
	LastShown uint64      `json:"lastShown"`
}

// TechniqueLesson は着手前の棋譜位置と独立した教材手順を保持する。Lineは実戦とは異なることがある。
type TechniqueLesson struct {
	ID                                              TechniqueID
	Turn                                            int
	Color                                           Color
	Line                                            []int
	Targets                                         []int
	Stones, BeforeLiberties, AfterLiberties, Groups int
}

// techniqueGroups は指定色の連を一度ずつ列挙する。
func techniqueGroups(b *Board, color Color) [][]int {
	var result [][]int
	seen := make([]bool, len(b.Cells))
	for p, stone := range b.Cells {
		if stone != color || seen[p] {
			continue
		}
		group, _ := b.Group(p)
		for _, q := range group {
			seen[q] = true
		}
		result = append(result, group)
	}
	return result
}

// BasicTechniques は合法な一手の取り・救出・連結・アタリを検出する。自分のアタリを好手として扱わない。
func BasicTechniques(b *Board, point, turn int) []TechniqueLesson {
	if b == nil || point < 0 || turn < 1 || !b.Legal(point) {
		return nil
	}
	// 履歴を保持した所有コピーで確認し、コウを含む実局面の合法性を維持する。
	next := cloneTechniqueBoard(b)
	if next.Play(point) != nil {
		return nil
	}
	_, ownLiberties := next.Group(point)
	if len(ownLiberties) <= 1 {
		return nil
	}
	base := TechniqueLesson{Turn: turn, Color: b.ToMove, Line: []int{point}}
	var result []TechniqueLesson
	var captured, rescued, joined, attacked []int
	joinedCount, oldLiberties, newLiberties := 0, 0, 0
	for _, group := range techniqueGroups(b, b.ToMove) {
		_, before := b.Group(group[0])
		_, after := next.Group(group[0])
		if len(before) == 1 && len(after) > 1 && len(group) > len(rescued) {
			rescued = slices.Clone(group)
			oldLiberties, newLiberties = 1, len(after)
		}
		adjacent := false
		for _, p := range Neighbors(point, b.Size) {
			adjacent = adjacent || slices.Contains(group, p)
		}
		if adjacent {
			joinedCount++
			joined = append(joined, group...)
		}
	}
	for _, group := range techniqueGroups(b, b.ToMove.Other()) {
		if next.Cells[group[0]] == Empty {
			captured = append(captured, group...)
			continue
		}
		_, before := b.Group(group[0])
		_, after := next.Group(group[0])
		if len(before) > 1 && len(after) == 1 {
			attacked = append(attacked, group...)
		}
	}
	add := func(id TechniqueID, targets []int) {
		m := base
		m.ID, m.Targets, m.Stones = id, slices.Clone(targets), len(targets)
		if id == TechniqueEscape {
			m.BeforeLiberties, m.AfterLiberties = oldLiberties, newLiberties
		}
		if id == TechniqueConnect {
			m.Groups = joinedCount
		}
		result = append(result, m)
	}
	if len(captured) > 0 {
		add(TechniqueCapture, captured)
	}
	if len(rescued) > 0 {
		add(TechniqueEscape, rescued)
	}
	if joinedCount >= 2 {
		add(TechniqueConnect, joined)
	}
	if len(attacked) > 0 {
		add(TechniqueAtari, attacked)
	}
	return result
}

// cloneTechniqueBoard は元の盤面・棋譜・履歴を変更しない作業盤を返す。既存履歴のセル配列は読取り専用で共有する。
func cloneTechniqueBoard(b *Board) *Board {
	copy := *b
	copy.Cells = slices.Clone(b.Cells)
	copy.Moves = slices.Clone(b.Moves)
	copy.history = slices.Clone(b.history)
	return &copy
}

// SelectTechnique は未紹介・少ない回数・古い紹介順を優先し、同条件なら自分の局面と教材順を使う。
func SelectTechnique(lessons []TechniqueLesson, history []TechniqueRecord, player Color) *TechniqueLesson {
	progress := make(map[TechniqueID]TechniqueRecord, len(history))
	for _, h := range history {
		progress[h.ID] = h
	}
	ids := TechniqueIDs()
	better := func(a, b TechniqueLesson) bool {
		x, y := progress[a.ID], progress[b.ID]
		if x.Count != y.Count {
			return x.Count < y.Count
		}
		if x.LastShown != y.LastShown {
			return x.LastShown < y.LastShown
		}
		if (a.Color == player) != (b.Color == player) {
			return a.Color == player
		}
		if a.ID != b.ID {
			return slices.Index(ids, a.ID) < slices.Index(ids, b.ID)
		}
		return a.Turn < b.Turn
	}
	var best *TechniqueLesson
	for _, lesson := range lessons {
		if !slices.Contains(ids, lesson.ID) || lesson.Turn < 1 || len(lesson.Line) == 0 {
			continue
		}
		if best == nil || better(lesson, *best) {
			copy := lesson
			best = &copy
		}
	}
	if best != nil {
		best.Line = slices.Clone(best.Line)
		best.Targets = slices.Clone(best.Targets)
	}
	return best
}
