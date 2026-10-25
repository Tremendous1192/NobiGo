// 作成日: 2026-10-25。教材の盤面根拠・合法性・紹介順位と入力不変を検証する。
package app

import (
	"reflect"
	"slices"
	"testing"
)

// techniquePosition は合法性検査用の静的配置を作る。実戦の棋譜やユーザーデータは使わない。
func techniquePosition(t *testing.T, size int, black, white []int, toMove Color) *Board {
	t.Helper()
	b, err := NewBoard(size, Japanese)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range black {
		b.Cells[p] = Black
	}
	for _, p := range white {
		if b.Cells[p] != Empty {
			t.Fatal("overlapping fixture")
		}
		b.Cells[p] = White
	}
	b.ToMove = toMove
	b.history = nil
	b.record()
	return b
}

// TestBasicTechniques は一手の事実を技術別に分類し、元の盤面を保持することを検証する。
func TestBasicTechniques(t *testing.T) {
	for _, tc := range []struct {
		name         string
		black, white []int
		point        int
		id           TechniqueID
		stones       int
	}{
		{"capture", []int{1, 7, 9}, []int{8}, 15, TechniqueCapture, 1},
		{"escape", []int{8}, []int{1, 7, 9}, 15, TechniqueEscape, 1},
		{"connect", []int{7, 9}, nil, 8, TechniqueConnect, 2},
		{"atari", []int{1, 7}, []int{8}, 9, TechniqueAtari, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, player := range []Color{Black, White} {
				black, white := tc.black, tc.white
				if player == White {
					black, white = white, black
				}
				b := techniquePosition(t, 7, black, white, player)
				before := slices.Clone(b.Cells)
				lessons := BasicTechniques(b, tc.point, 1)
				found := false
				for _, m := range lessons {
					if m.ID == tc.id {
						found = true
						if m.Stones != tc.stones || m.Color != player {
							t.Fatalf("wrong evidence: %+v", m)
						}
					}
				}
				if !found || !slices.Equal(before, b.Cells) || len(b.Moves) != 0 {
					t.Fatal("missing evidence or mutated input", lessons)
				}
			}
		})
	}
}

// TestTechniqueRejectsUnsafeAndIllegal は自殺・占有・盤外・パス・自分のアタリを教材にしないことを検証する。
func TestTechniqueRejectsUnsafeAndIllegal(t *testing.T) {
	b := techniquePosition(t, 7, nil, []int{1, 7}, Black)
	for _, point := range []int{-1, 0, 1, 49} {
		if len(BasicTechniques(b, point, 1)) != 0 {
			t.Fatal("illegal lesson", point)
		}
	}
	b = techniquePosition(t, 7, []int{8}, []int{0, 2, 7, 9}, Black)
	if len(BasicTechniques(b, 1, 1)) != 0 {
		t.Fatal("self atari praised")
	}
	if SelectTechnique(nil, nil, Black) != nil {
		t.Fatal("invented lesson")
	}
}

// TestTechniqueSelection は未紹介・反復紹介・自分の局面・所有コピーを検証する。
func TestTechniqueSelection(t *testing.T) {
	lessons := []TechniqueLesson{{ID: TechniqueCapture, Turn: 1, Color: Black, Line: []int{1}}, {ID: TechniqueEscape, Turn: 2, Color: White, Line: []int{2}}}
	history := []TechniqueRecord{{ID: TechniqueCapture, Count: 1, LastShown: 2}}
	if got := SelectTechnique(lessons, history, Black); got == nil || got.ID != TechniqueEscape {
		t.Fatal("unseen not prioritized")
	}
	history = append(history, TechniqueRecord{ID: TechniqueEscape, Count: 1, LastShown: 1})
	if got := SelectTechnique(lessons, history, Black); got.ID != TechniqueEscape {
		t.Fatal("old lesson not prioritized")
	}
	got := SelectTechnique(lessons, nil, Black)
	if got.ID != TechniqueCapture {
		t.Fatal("player not prioritized")
	}
	got.Line[0] = 9
	if !reflect.DeepEqual(lessons[0].Line, []int{1}) {
		t.Fatal("selection aliases source")
	}
}
