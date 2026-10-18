// 作成日: 2026-10-18。囲碁の盤面と着手履歴を管理する。変更は対局所有者1つに限定する。

// Package app は表示に依存しない囲碁の規則と対局状態を管理する。
package app

import (
	"errors"
	"fmt"
	"slices"
)

type Color uint8

const (
	Empty Color = iota
	Black
	White
)

type Rule string

const (
	Japanese Rule = "japanese"
	Chinese  Rule = "chinese"
	Jungo    Rule = "jungo"
	MaxMoves      = 4096
)

var (
	ErrOccupied = errors.New("point is occupied")
	ErrSuicide  = errors.New("suicide is forbidden")
	ErrKo       = errors.New("immediate ko recapture is forbidden")
	ErrFinished = errors.New("game is not accepting moves")
)

// Other は相手の色を返す。空点には空を返す。
func (c Color) Other() Color {
	if c == Black {
		return White
	}
	if c == White {
		return Black
	}
	return Empty
}

// ValidRule は採用ルールを判定する。
func ValidRule(r Rule) bool { return r == Japanese || r == Chinese || r == Jungo }

// ValidSize は指定された6種類の路数だけを受理する。
func ValidSize(n int) bool { return slices.Contains([]int{7, 9, 11, 13, 15, 19}, n) }

// Komi は白へのコミを返す。
func Komi(r Rule) float64 {
	switch r {
	case Japanese:
		return 6.5
	case Chinese:
		return 7.5
	default:
		return 0
	}
}

type Move struct {
	Color Color `json:"color"`
	Point int   `json:"point"` // -1はパス。その他は左上からの行優先番号。
}
type position struct {
	cells    []Color
	toMove   Color
	captured [3]int
	passes   int
	noResult bool
}

// Board の公開スライスは読取り専用。状態変更にはPlay・Undoを使う。
type Board struct {
	Size     int
	Rule     Rule
	Cells    []Color
	ToMove   Color
	Captured [3]int // 色ごとに取り上げた相手の石数。
	Passes   int
	NoResult bool
	Moves    []Move
	history  []position
}

// NewBoard は空盤を作る。無効なルール・路数を拒否する。
func NewBoard(size int, rule Rule) (*Board, error) {
	if !ValidSize(size) || !ValidRule(rule) {
		return nil, errors.New("invalid board settings")
	}
	b := &Board{Size: size, Rule: rule, Cells: make([]Color, size*size), ToMove: Black}
	b.record()
	return b, nil
}

// record は取り消し用に現在局面を所有コピーする。
func (b *Board) record() {
	b.history = append(b.history, position{slices.Clone(b.Cells), b.ToMove, b.Captured, b.Passes, b.NoResult})
}

// Neighbors は盤内の上下左右を返す。
func Neighbors(point, size int) []int {
	if point < 0 || point >= size*size {
		return nil
	}
	n := make([]int, 0, 4)
	if point >= size {
		n = append(n, point-size)
	}
	if point < size*(size-1) {
		n = append(n, point+size)
	}
	if point%size > 0 {
		n = append(n, point-1)
	}
	if point%size < size-1 {
		n = append(n, point+1)
	}
	return n
}

// group は連と重複しない呼吸点を返す。
func group(cells []Color, size, point int) (stones, liberties []int) {
	if point < 0 || point >= len(cells) || cells[point] == Empty {
		return nil, nil
	}
	seen := make([]bool, len(cells))
	seen[point] = true
	stones = []int{point}
	empty := make([]bool, len(cells))
	for i := 0; i < len(stones); i++ {
		for _, n := range Neighbors(stones[i], size) {
			if cells[n] == Empty && !empty[n] {
				empty[n] = true
				liberties = append(liberties, n)
			}
			if cells[n] == cells[point] && !seen[n] {
				seen[n] = true
				stones = append(stones, n)
			}
		}
	}
	return stones, liberties
}

// Group は指定点の連と呼吸点の独立した配列を返す。
func (b *Board) Group(point int) ([]int, []int) { return group(b.Cells, b.Size, point) }

// trial は本体を変更せず合法性と取り石を計算する。
func (b *Board) trial(point int) ([]Color, int, error) {
	if b.NoResult || b.Passes >= 2 {
		return nil, 0, ErrFinished
	}
	if len(b.Moves) >= MaxMoves {
		return nil, 0, errors.New("move limit reached")
	}
	if point == -1 {
		return slices.Clone(b.Cells), 0, nil
	}
	if point < 0 || point >= len(b.Cells) {
		return nil, 0, errors.New("point is outside board")
	}
	if b.Cells[point] != Empty {
		return nil, 0, ErrOccupied
	}
	next := slices.Clone(b.Cells)
	next[point] = b.ToMove
	captured := 0
	for _, n := range Neighbors(point, b.Size) {
		if next[n] == b.ToMove.Other() {
			stones, libs := group(next, b.Size, n)
			if len(libs) == 0 {
				captured += len(stones)
				for _, p := range stones {
					next[p] = Empty
				}
			}
		}
	}
	_, libs := group(next, b.Size, point)
	if len(libs) == 0 {
		return nil, 0, ErrSuicide
	}
	if len(b.history) >= 2 && slices.Equal(next, b.history[len(b.history)-2].cells) {
		return nil, 0, ErrKo
	}
	return next, captured, nil
}

// Legal は着手可能かを盤面を変更せず判定する。
func (b *Board) Legal(point int) bool { _, _, err := b.trial(point); return err == nil }

// Play は合法な着手またはパスを確定する。失敗時は状態を維持する。
func (b *Board) Play(point int) error {
	next, captured, err := b.trial(point)
	if err != nil {
		return err
	}
	b.Moves = append(b.Moves, Move{Color: b.ToMove, Point: point})
	b.Cells = next
	b.Captured[b.ToMove] += captured
	b.ToMove = b.ToMove.Other()
	if point == -1 {
		b.Passes++
	} else {
		b.Passes = 0
		repeats := 0
		// 単純コウ規則では長周期反復をスーパーコウ違反に置き換えず、無勝負とする。
		for i := len(b.history) - 1; i >= 0; i-- {
			if b.history[i].passes > 0 {
				break
			}
			if b.history[i].toMove == b.ToMove && slices.Equal(b.Cells, b.history[i].cells) {
				repeats++
			}
		}
		b.NoResult = repeats >= 2
	}
	b.record()
	return nil
}

// Undo は指定手数を取り消す。履歴不足時は何も変更しない。
func (b *Board) Undo(count int) bool {
	if count < 1 || count > len(b.Moves) {
		return false
	}
	n := len(b.Moves) - count
	p := b.history[n]
	b.Cells = slices.Clone(p.cells)
	b.ToMove = p.toMove
	b.Captured = p.captured
	b.Passes = p.passes
	b.NoResult = p.noResult
	b.Moves = b.Moves[:n]
	b.history = b.history[:n+1]
	return true
}

// UndoPlayer は直前のプレイヤー着手の前へ戻す。CPU初手しかない場合は戻さない。
func (b *Board) UndoPlayer(player Color) bool {
	for i := len(b.Moves) - 1; i >= 0; i-- {
		if b.Moves[i].Color == player {
			return b.Undo(len(b.Moves) - i)
		}
	}
	return false
}

// Replay は棋譜の指定手までを検証して再現する。
func Replay(size int, rule Rule, moves []Move, turn int) (*Board, error) {
	if turn < 0 || turn > len(moves) || turn > MaxMoves {
		return nil, errors.New("invalid replay turn")
	}
	b, err := NewBoard(size, rule)
	if err != nil {
		return nil, err
	}
	for i, m := range moves[:turn] {
		if m.Color != b.ToMove {
			return nil, fmt.Errorf("wrong color at turn %d", i+1)
		}
		if err = b.Play(m.Point); err != nil {
			return nil, fmt.Errorf("replay turn %d: %w", i+1, err)
		}
	}
	return b, nil
}
