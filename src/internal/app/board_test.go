// 作成日: 2026-10-18。合法手・取り石・コウ・履歴復元の境界を検証する。
package app

import (
	"errors"
	"slices"
	"testing"
)

// boardFixture はテスト局面を作り、初期履歴にも同じ配置を保持する。
func boardFixture(t *testing.T, size int, black, white []int) *Board {
	t.Helper()
	b, err := NewBoard(size, Japanese)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range black {
		b.Cells[p] = Black
	}
	for _, p := range white {
		b.Cells[p] = White
	}
	b.history = nil
	b.record()
	return b
}

// TestBoardSettings は許可盤サイズとルールだけを受理することを検証する。
func TestBoardSettings(t *testing.T) {
	for _, size := range []int{7, 9, 11, 13, 15, 19} {
		for _, rule := range []Rule{Japanese, Chinese, Jungo} {
			b, err := NewBoard(size, rule)
			if err != nil || len(b.Cells) != size*size || b.ToMove != Black {
				t.Fatalf("invalid initial state: %v", err)
			}
		}
	}
	for _, size := range []int{-1, 0, 6, 8, 20} {
		if _, err := NewBoard(size, Japanese); err == nil {
			t.Fatalf("accepted size %d", size)
		}
	}
	if _, err := NewBoard(9, Rule("unknown")); err == nil {
		t.Fatal("accepted unknown rule")
	}
}

// TestCaptureAndUndo は端の取り石と取り消し時の完全復元を検証する。
func TestCaptureAndUndo(t *testing.T) {
	b := boardFixture(t, 7, nil, nil)
	for _, p := range []int{1, 0, 7} {
		if err := b.Play(p); err != nil {
			t.Fatal(err)
		}
	}
	if b.Cells[0] != Empty || b.Captured[Black] != 1 || b.ToMove != White {
		t.Fatal("capture did not update state")
	}
	if !b.UndoPlayer(Black) || len(b.Moves) != 2 || b.Cells[0] != White || b.Captured[Black] != 0 || b.ToMove != Black {
		t.Fatal("undo failed to restore capture")
	}
	if !b.UndoPlayer(Black) || len(b.Moves) != 0 {
		t.Fatal("repeated undo failed")
	}
	if b.UndoPlayer(Black) {
		t.Fatal("empty history was undone")
	}
}

// TestConnectedCapture は連全体を取り重複計上しないことを検証する。
func TestConnectedCapture(t *testing.T) {
	b := boardFixture(t, 7, []int{1, 2, 7, 10, 15}, []int{8, 9})
	if err := b.Play(16); err != nil {
		t.Fatal(err)
	}
	if b.Captured[Black] != 2 || b.Cells[8] != Empty || b.Cells[9] != Empty {
		t.Fatal("group was not captured")
	}
}

// TestRejectedMovePreservesState は自殺手・範囲外・占有点で状態が変わらないことを検証する。
func TestRejectedMovePreservesState(t *testing.T) {
	b := boardFixture(t, 7, []int{6}, []int{1, 7})
	for _, p := range []int{0, -2, 49, 6} {
		before := append([]Color(nil), b.Cells...)
		if err := b.Play(p); err == nil {
			t.Fatalf("accepted invalid point %d", p)
		}
		if !slices.Equal(before, b.Cells) || len(b.Moves) != 0 || b.ToMove != Black {
			t.Fatal("rejected move mutated board")
		}
	}
	if !errors.Is(b.Play(0), ErrSuicide) {
		t.Fatal("suicide error not classified")
	}
}

// TestKo は即時取り返しを禁止し、コウ材への応答後の取り返しを許可する。
func TestKo(t *testing.T) {
	b := boardFixture(t, 7, []int{17, 23, 31}, []int{24, 18, 26, 32})
	if err := b.Play(25); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(b.Play(24), ErrKo) {
		t.Fatal("immediate recapture accepted")
	}
	for _, p := range []int{0, 1, 24} {
		if err := b.Play(p); err != nil {
			t.Fatal(err)
		}
	}
	if b.Cells[25] != Empty || b.Captured[White] != 1 {
		t.Fatal("recapture failed")
	}
}

// TestPassAndReplay は連続パス・白番プレイヤーの待った・棋譜再生を検証する。
func TestPassAndReplay(t *testing.T) {
	b := boardFixture(t, 9, nil, nil)
	if err := b.Play(40); err != nil {
		t.Fatal(err)
	}
	if b.UndoPlayer(White) {
		t.Fatal("cpu opening was undone")
	}
	for _, p := range []int{-1, -1} {
		if err := b.Play(p); err != nil {
			t.Fatal(err)
		}
	}
	if b.Passes != 2 || !errors.Is(b.Play(0), ErrFinished) {
		t.Fatal("game did not stop after passes")
	}
	restored, err := Replay(9, Japanese, b.Moves, len(b.Moves))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(b.Cells, restored.Cells) || restored.Passes != 2 {
		t.Fatal("replay differs")
	}
	if !b.UndoPlayer(White) || len(b.Moves) != 1 || b.Passes != 0 || b.ToMove != White {
		t.Fatal("white undo failed")
	}
	if _, err = Replay(9, Japanese, []Move{{White, 1}}, 1); err == nil {
		t.Fatal("wrong replay color accepted")
	}
}

// TestTripleKoNoResult は合法な三コウの反復で無勝負となることを検証する。
func TestTripleKoNoResult(t *testing.T) {
	b := boardFixture(t, 19, nil, nil)
	for i, x := range []int{3, 9, 15} {
		p := 3*19 + x
		attacker := Black
		if i == 1 {
			attacker = White
		}
		for _, n := range []int{p - 19, p - 1, p + 19} {
			b.Cells[n] = attacker
		}
		for _, n := range []int{p, p + 1 - 19, p + 2, p + 1 + 19} {
			b.Cells[n] = attacker.Other()
		}
	}
	b.history = nil
	b.record()
	cycle := []int{61, 67, 73, 60, 66, 72}
	for round := 0; round < 2; round++ {
		for _, p := range cycle {
			if err := b.Play(p); err != nil {
				t.Fatalf("round %d point %d: %v", round, p, err)
			}
		}
	}
	if !b.NoResult {
		t.Fatal("triple repetition was not recognized")
	}
	if !b.Undo(1) || b.NoResult {
		t.Fatal("undo did not restore no-result flag")
	}
}

// BenchmarkLegal19 は19路の合法手列挙に必要な時間と割当量を測る。
func BenchmarkLegal19(b *testing.B) {
	board, err := NewBoard(19, Japanese)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for p := range board.Cells {
			board.Legal(p)
		}
	}
}
