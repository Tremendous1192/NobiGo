// 作成日: 2026-10-25。複数手の成立・逃走・打切り・条件付き取り返しを検証する。
package app

import (
	"context"
	"slices"
	"testing"
)

// findLesson は期待する教材がなければ検証を失敗させる。
func findLesson(t *testing.T, lessons []TechniqueLesson, id TechniqueID) TechniqueLesson {
	t.Helper()
	for _, m := range lessons {
		if m.ID == id {
			return m
		}
	}
	t.Fatalf("missing technique %s: %+v", id, lessons)
	return TechniqueLesson{}
}

// TestTechniqueLadder は強制捕獲と逃走を区別し、例示手順を合法に再生できることを検証する。
func TestTechniqueLadder(t *testing.T) {
	b := techniquePosition(t, 7, []int{1, 7, 14}, []int{8}, Black)
	before := slices.Clone(b.Cells)
	m := findLesson(t, ReadTechniques(context.Background(), b, 9, 1), TechniqueLadder)
	for _, p := range m.Line {
		if err := b.Play(p); err != nil {
			t.Fatal(err)
		}
	}
	if b.Cells[8] != Empty {
		t.Fatal("ladder did not capture")
	}
	if !b.Undo(len(m.Line)) || !slices.Equal(b.Cells, before) {
		t.Fatal("reading changed board")
	}
	b.Cells[32] = White
	b.history = nil
	b.record()
	for _, m := range ReadTechniques(context.Background(), b, 9, 1) {
		if m.ID == TechniqueLadder {
			t.Fatal("ladder breaker ignored", m.Line)
		}
	}
}

// TestTechniqueNet は二つの逃げ道を囲う手と、囲いの外へ逃げられる手を区別する。
func TestTechniqueNet(t *testing.T) {
	b := techniquePosition(t, 7, []int{1, 2, 7, 10, 14, 22}, []int{8}, Black)
	m := findLesson(t, ReadTechniques(context.Background(), b, 16, 1), TechniqueNet)
	for _, p := range m.Line {
		if err := b.Play(p); err != nil {
			t.Fatal(err)
		}
	}
	if b.Cells[8] != Empty {
		t.Fatal("net did not capture")
	}
	b = techniquePosition(t, 7, []int{1, 7, 14, 22}, []int{8}, Black)
	for _, m := range ReadTechniques(context.Background(), b, 16, 1) {
		if m.ID == TechniqueNet {
			t.Fatal("net escape ignored", m.Line)
		}
	}
}

// TestTechniqueDoubleAtari は相手が一方を逃げても他方を取れる場合だけ両アタリとして紹介する。
func TestTechniqueDoubleAtari(t *testing.T) {
	b := techniquePosition(t, 7, []int{1, 7, 3, 11}, []int{8, 10}, Black)
	m := findLesson(t, ReadTechniques(context.Background(), b, 9, 1), TechniqueDoubleAtari)
	if len(m.Line) != 3 || m.Groups != 2 {
		t.Fatal("invalid double atari", m)
	}
	// 最後の空点を共有する二連は、別々の逃げ道を迫る両アタリとは区別する。
	b = techniquePosition(t, 7, []int{2, 14}, []int{1, 7}, Black)
	for _, m := range ReadTechniques(context.Background(), b, 8, 1) {
		if m.ID == TechniqueDoubleAtari {
			t.Fatal("shared liberty presented as double atari")
		}
	}
}

// TestTechniqueSnapback は合法な三手で取らせた数より多く取り返すことと、コウの誤認防止を検証する。
func TestTechniqueSnapback(t *testing.T) {
	for _, color := range []Color{Black, White} {
		black, white := []int{2, 9, 14, 15}, []int{1, 8}
		if color == White {
			black, white = white, black
		}
		b := techniquePosition(t, 7, black, white, color)
		m := findLesson(t, ReadTechniques(context.Background(), b, 0, 1), TechniqueSnapback)
		if !slices.Equal(m.Line, []int{0, 7, 0}) || m.Stones != 3 {
			t.Fatal("wrong snapback", m)
		}
	}
	b := techniquePosition(t, 7, []int{1, 7, 15}, []int{8, 2, 10, 16}, Black)
	for _, m := range ReadTechniques(context.Background(), b, 9, 1) {
		if m.ID == TechniqueSnapback {
			t.Fatal("ko presented as snapback")
		}
	}
}

// TestTechniqueReadLimits は取消・深さ・ノード上限を成立と扱わないことを検証する。
func TestTechniqueReadLimits(t *testing.T) {
	b := techniquePosition(t, 7, []int{1, 7, 14}, []int{8}, Black)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if len(ReadTechniques(ctx, b, 9, 1)) != 0 {
		t.Fatal("cancel ignored")
	}
	r := captureReader{attacker: Black, targets: []int{8}, limit: 1}
	if v, _ := r.solve(context.Background(), b, 64); v != captureUnknown {
		t.Fatal("node limit accepted")
	}
	r.limit = 8192
	r.nodes = 0
	if v, _ := r.solve(context.Background(), b, 0); v != captureUnknown {
		t.Fatal("depth limit accepted")
	}
}
