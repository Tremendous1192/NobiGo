// 作成日: 2026-10-19。更新日: 2026-10-24。完了棋譜と解析から概要の根拠を集計する。入力・プロフィールを変更せず、未解析の優劣を推定しない。
package app

import (
	"errors"
	"math"

	"github.com/Tremendous1192/NobiGo/engine"
)

// SummaryMove は一手の実際の効果と着手側の評価を所有する。AlternativeはHasAlternative時だけ有効。
type SummaryMove struct {
	Turn, Point                   int
	Color                         Color
	Captured, Saved, Connected    int
	SelfAtari                     bool
	Evaluated                     bool
	Loss, Swing, WinChange        float64
	Alternative                   int
	HasAlternative, BestCandidate bool
}

// SummaryStage は手数を三分割した区間のCPUの実着手を集計する。意図そのものではない。
type SummaryStage struct {
	From, To, CPUMoves, EdgeMoves, Captured, Saved, Connected int
	CaptureMoves, RescueMoves, ConnectMoves                   int
	Representative                                            int // 1始まりのCPUの代表手。0は着手なし。
}

// GameSummary は解析済み範囲から選んだ概要を保持する。候補はMoves内を参照し、呼出し元は読取り専用で扱う。
type GameSummary struct {
	Moves                            []SummaryMove
	Stages                           [3]SummaryStage
	Highlight, GoodMove, Improvement *SummaryMove
	Evaluated                        int
}

// summaryEvaluation は未完了・低探索・異常な解析を優劣判定から除外する。
func summaryEvaluation(a *engine.Analysis) bool {
	if a == nil || a.Partial || !finiteRange(a.ScoreLead, -20000, 20000) || !finiteRange(a.Winrate, 0, 1) {
		return false
	}
	for _, m := range a.Moves {
		if m.Visits >= 2 {
			return true
		}
	}
	return false
}

// BuildGameSummary は一度だけ棋譜を再生して候補を選ぶ。空棋譜を許容し、不正棋譜はエラーにする。
func BuildGameSummary(board *Board, player Color, evaluations []*engine.Analysis) (GameSummary, error) {
	var result GameSummary
	if board == nil || player != Black && player != White || len(board.Moves) > MaxMoves {
		return result, errors.New("invalid summary game")
	}
	b, err := NewBoard(board.Size, board.Rule)
	if err != nil {
		return result, err
	}
	result.Moves = make([]SummaryMove, 0, len(board.Moves))
	for i, move := range board.Moves {
		if move.Color != b.ToMove {
			return GameSummary{}, errors.New("invalid summary move color")
		}
		m := SummaryMove{Turn: i + 1, Point: move.Point, Color: move.Color}
		var endangered [][2]int
		joined := make(map[int]bool)
		seen := make([]bool, len(b.Cells))
		for point, color := range b.Cells {
			if color != move.Color || seen[point] {
				continue
			}
			stones, liberties := b.Group(point)
			for _, stone := range stones {
				seen[stone] = true
			}
			if len(liberties) == 1 {
				endangered = append(endangered, [2]int{point, len(stones)})
			}
			for _, neighbor := range Neighbors(move.Point, b.Size) {
				for _, stone := range stones {
					if stone == neighbor {
						joined[point] = true
					}
				}
			}
		}
		if i+1 < len(evaluations) && summaryEvaluation(evaluations[i]) && summaryEvaluation(evaluations[i+1]) {
			before, after := evaluations[i], evaluations[i+1]
			sign := 1.0
			if move.Color == White {
				sign = -1
			}
			m.Evaluated = true
			m.Swing = sign * (after.ScoreLead - before.ScoreLead)
			m.WinChange = sign * (after.Winrate - before.Winrate)
			m.Loss = math.Max(0, -m.Swing)
			if len(before.Moves) > 0 {
				best := before.Moves[0]
				if best.Visits >= 2 && b.Legal(best.Point) {
					m.BestCandidate = best.Point == move.Point
					m.HasAlternative = !m.BestCandidate
					m.Alternative = best.Point
				}
			}
			result.Evaluated++
		}
		captured := b.Captured[move.Color]
		if err = b.Play(move.Point); err != nil {
			return GameSummary{}, err
		}
		m.Captured = b.Captured[move.Color] - captured
		if move.Point >= 0 {
			m.Connected = max(0, len(joined)-1)
			_, liberties := b.Group(move.Point)
			m.SelfAtari = len(liberties) == 1
			for _, group := range endangered {
				_, liberties := b.Group(group[0])
				if len(liberties) > 1 {
					m.Saved += group[1]
				}
			}
		}
		stage := &result.Stages[i*3/max(1, len(board.Moves))]
		if stage.From == 0 {
			stage.From = i + 1
		}
		stage.To = i + 1
		if move.Color != player {
			stage.CPUMoves++
			stage.Captured += m.Captured
			stage.Saved += m.Saved
			stage.Connected += m.Connected
			if m.Captured > 0 {
				stage.CaptureMoves++
			}
			if m.Saved > 0 {
				stage.RescueMoves++
			}
			if m.Connected > 0 {
				stage.ConnectMoves++
			}
			if move.Point >= 0 {
				x, y := move.Point%b.Size, move.Point/b.Size
				if min(x, y, b.Size-1-x, b.Size-1-y) < max(1, b.Size/5) {
					stage.EdgeMoves++
				}
			}
		}
		result.Moves = append(result.Moves, m)
	}
	for i := range result.Moves {
		m := &result.Moves[i]
		if !m.Evaluated {
			continue
		}
		if result.Highlight == nil || math.Abs(m.Swing) > math.Abs(result.Highlight.Swing) || math.Abs(m.Swing) == math.Abs(result.Highlight.Swing) && math.Abs(m.WinChange) > math.Abs(result.Highlight.WinChange) {
			result.Highlight = m
		}
		if m.Color != player || m.Point < 0 {
			continue
		}
		if m.Loss <= 1 && !m.SelfAtari && (summaryEffect(m) > 0 || m.BestCandidate) {
			if result.GoodMove == nil || summaryEffect(m) > summaryEffect(result.GoodMove) || summaryEffect(m) == summaryEffect(result.GoodMove) && m.Loss < result.GoodMove.Loss {
				result.GoodMove = m
			}
		}
		if m.Loss >= 1 && m.HasAlternative && (result.Improvement == nil || m.Loss > result.Improvement.Loss) {
			result.Improvement = m
		}
	}
	for i := range result.Stages {
		stage := &result.Stages[i]
		stage.Representative = summaryRepresentative(*stage, result.Moves, player)
	}
	return result, nil
}

// summaryRepresentative は区間内で最も多い行動をしたCPUの手を選ぶ。同点は複合効果、手順の順で決める。
func summaryRepresentative(stage SummaryStage, moves []SummaryMove, player Color) int {
	var best *SummaryMove
	weight := -1
	most := max(stage.CaptureMoves, stage.RescueMoves, stage.ConnectMoves)
	for i := range moves {
		m := &moves[i]
		if m.Color == player || m.Turn < stage.From || m.Turn > stage.To {
			continue
		}
		w := 0
		if m.Point >= 0 {
			w++
		}
		for _, effect := range [][2]int{{m.Captured, stage.CaptureMoves}, {m.Saved, stage.RescueMoves}, {m.Connected, stage.ConnectMoves}} {
			if effect[0] > 0 && effect[1] == most {
				w += 2
			}
		}
		if best == nil || w > weight || w == weight && summaryEffect(m) > summaryEffect(best) {
			best, weight = m, w
		}
	}
	if best == nil {
		return 0
	}
	return best.Turn
}

// summaryEffect は説明できる取り・救出・連結を優先するための同点比較値。
func summaryEffect(m *SummaryMove) int { return m.Captured + m.Saved + m.Connected }
