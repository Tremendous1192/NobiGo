// 作成日: 2026-10-25。今回の棋譜を一度再生して教材を集める。解析作業内で実行し、取消・有限候補数を守る。
package app

import (
	"context"
	"errors"
	"slices"

	"github.com/Tremendous1192/NobiGo/engine"
)

// retainTechnique は同じ技術・手番側の最初の例だけを保持し、教材配列を最大16件に抑える。
func retainTechnique(lessons []TechniqueLesson, candidate TechniqueLesson) []TechniqueLesson {
	for i, m := range lessons {
		if m.ID == candidate.ID && m.Color == candidate.Color {
			if candidate.Turn < m.Turn {
				lessons[i] = candidate
			}
			return lessons
		}
	}
	return append(lessons, candidate)
}

// FindGameTechniques は基本技術を全手で確認し、複数手は全局を32区間に分け各区間の最大2候補を調べる。
// 戻り値は所有コピー。期限・取消では確認済み候補とエラーを返し、未確認候補を混ぜない。
func FindGameTechniques(ctx context.Context, board *Board, evaluations []*engine.Analysis) ([]TechniqueLesson, error) {
	if board == nil || len(board.Moves) > MaxMoves {
		return nil, errors.New("invalid technique game")
	}
	b, err := NewBoard(board.Size, board.Rule)
	if err != nil {
		return nil, err
	}
	var lessons []TechniqueLesson
	var reads [32]int
	for i, move := range board.Moves {
		if ctx.Err() != nil {
			return lessons, ctx.Err()
		}
		if move.Color != b.ToMove {
			return nil, errors.New("invalid technique move color")
		}
		points := []int{move.Point}
		if i < len(evaluations) && summaryEvaluation(evaluations[i]) {
			for _, m := range evaluations[i].Moves[:min(4, len(evaluations[i].Moves))] {
				if m.Visits >= 2 && !slices.Contains(points, m.Point) {
					points = append(points, m.Point)
				}
			}
		}
		bucket := min(31, i*32/max(1, len(board.Moves)))
		for _, point := range points {
			if ctx.Err() != nil {
				return lessons, ctx.Err()
			}
			for _, m := range BasicTechniques(b, point, i+1) {
				lessons = retainTechnique(lessons, m)
			}
			if reads[bucket] < 2 && point >= 0 && techniqueReadCandidate(b, point) {
				reads[bucket]++
				for _, m := range ReadTechniques(ctx, b, point, i+1) {
					lessons = retainTechnique(lessons, m)
				}
			}
		}
		if err = b.Play(move.Point); err != nil {
			return nil, err
		}
	}
	return lessons, ctx.Err()
}

// techniqueReadCandidate は二呼吸点の敵連が近くにある合法手だけに探索予算を使う。
func techniqueReadCandidate(b *Board, point int) bool {
	if !b.Legal(point) {
		return false
	}
	x, y := point%b.Size, point/b.Size
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			nx, ny := x+dx, y+dy
			if nx < 0 || nx >= b.Size || ny < 0 || ny >= b.Size {
				continue
			}
			p := ny*b.Size + nx
			if b.Cells[p] != b.ToMove.Other() {
				continue
			}
			_, libs := b.Group(p)
			if len(libs) == 2 {
				return true
			}
		}
	}
	return false
}
