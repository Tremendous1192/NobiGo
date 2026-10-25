// 作成日: 2026-10-25。実際の合法棋譜からの教材抽出と中止時の入力不変を検証する。
package app

import (
	"context"
	"errors"
	"slices"
	"testing"
)

// TestGameTechniques は棋譜途中のウッテガエシを、終局盤面を変えずに抽出する。
func TestGameTechniques(t *testing.T) {
	b, _ := NewBoard(7, Japanese)
	for _, p := range []int{2, 1, 9, 8, 14, 48, 15, 47, 0, 7, 0, -1, -1} {
		if err := b.Play(p); err != nil {
			t.Fatal(err)
		}
	}
	before := slices.Clone(b.Cells)
	lessons, err := FindGameTechniques(context.Background(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := findLesson(t, lessons, TechniqueSnapback)
	if m.Turn != 9 || m.Color != Black || !slices.Equal(m.Line, []int{0, 7, 0}) || !slices.Equal(before, b.Cells) || len(lessons) > 16 {
		t.Fatal("invalid game lesson", m)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = FindGameTechniques(ctx, b, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel ignored")
	}
}

// TestGameTechniquesNoLesson は連続パスだけの対局で教材を創作しないことを検証する。
func TestGameTechniquesNoLesson(t *testing.T) {
	b, _ := NewBoard(7, Jungo)
	if err := b.Play(-1); err != nil {
		t.Fatal(err)
	}
	if err := b.Play(-1); err != nil {
		t.Fatal(err)
	}
	lessons, err := FindGameTechniques(context.Background(), b, nil)
	if err != nil || len(lessons) != 0 {
		t.Fatal("invented lesson", lessons, err)
	}
}
