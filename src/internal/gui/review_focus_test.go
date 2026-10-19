// 作成日: 2026-10-19。拡大図の連・共有呼吸点・前後局面・所有コピーを検証する。
package gui

import (
	"image"
	"reflect"
	"slices"
	"testing"

	"github.com/Tremendous1192/NobiGo/internal/app"
)

// TestFocusMatchesCombinedAdvice は取り・攻撃・救出・連結の対象が本文から欠落しないことを検証する。
func TestFocusMatchesCombinedAdvice(t *testing.T) {
	for _, tc := range []struct {
		points []int
		labels []string
	}{
		{[]int{2, 1, 8, 7, 0}, []string{"着手した連", "取られた連", "アタリにした連"}},
		{[]int{0, 1, 14, 48, 7}, []string{"着手した連", "助けた連", "つないだ連"}},
	} {
		c, before, after := adviceFixture(t, app.Japanese, tc.points)
		advice := reviewAdvice(c, len(tc.points), before, after)
		for _, label := range tc.labels {
			if !slices.ContainsFunc(advice.focus, func(f focusTarget) bool { return f.label == label }) {
				t.Fatalf("missing target %s", label)
			}
		}
		d := newReviewDiagram(before, after, advice.focus)
		if d.showBefore || d.point != tc.points[len(tc.points)-1] {
			t.Fatal("wrong initial focus")
		}
		for i, target := range d.targets {
			d.selectTarget(i)
			d.showBefore = true
			pre, preOK := d.selectedGroup()
			d.showBefore = false
			_, postOK := d.selectedGroup()
			if target.label == "取られた連" && (!preOK || len(pre.stones) != 1 || postOK) {
				t.Fatal("captured target lost before/after meaning")
			}
		}
	}
}

// TestDiagramSharesUniqueLiberties は同じ連の全石が重複のない呼吸点集合を共有することを検証する。
func TestDiagramSharesUniqueLiberties(t *testing.T) {
	b, err := app.NewBoard(7, app.Japanese)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []int{0, 1, 7, 8} {
		b.Cells[p] = app.Black
	}
	b.Cells[3] = app.White
	d := newReviewDiagram(nil, b, nil)
	for _, stone := range []int{0, 1, 7, 8} {
		if !d.selectStone(stone) {
			t.Fatal("stone not selectable")
		}
		group, ok := d.selectedGroup()
		got := slices.Clone(group.liberties)
		slices.Sort(got)
		if !ok || len(group.stones) != 4 || !reflect.DeepEqual(got, []int{2, 9, 14, 15}) {
			t.Fatalf("wrong liberties: %v", got)
		}
	}
	white := d.after.groups[d.after.groupAt[3]]
	if !slices.Contains(white.liberties, 2) {
		t.Fatal("opposing groups cannot share an empty point")
	}
	if d.selectStone(2) || d.selectStone(-1) || d.selectStone(49) {
		t.Fatal("selected empty or outside point")
	}
}

// TestDiagramOwnsSnapshots は開いた図が入力盤面・番号・対象リストの後続変更に影響されないことを検証する。
func TestDiagramOwnsSnapshots(t *testing.T) {
	c, before, after := adviceFixture(t, app.Japanese, []int{0, 1, 7})
	a := reviewAdvice(c, 3, before, after)
	d := newReviewDiagram(before, after, a.focus)
	after.Cells[7] = app.Empty
	after.Moves[2].Point = -1
	before.Cells[0] = app.Empty
	a.focus[0].point = 48
	if d.after.cells[7] != app.Black || d.after.numbers[7] != 3 || d.before.cells[0] != app.Black || d.targets[0].point != 7 {
		t.Fatal("snapshot retained mutable input")
	}
}

// TestDiagramViewportAndOutsideCounts は全路数・盤端と図外の連や呼吸点を検証する。
func TestDiagramViewportAndOutsideCounts(t *testing.T) {
	for _, size := range []int{7, 9, 11, 13, 15, 19} {
		b, err := app.NewBoard(size, app.Chinese)
		if err != nil {
			t.Fatal(err)
		}
		for p := 0; p < size; p++ {
			b.Cells[p] = app.Black
		}
		d := newReviewDiagram(nil, b, []focusTarget{{point: 0}})
		for _, point := range []int{0, size - 1, size * (size - 1), size*size - 1, size * size / 2} {
			d.center = point
			r := d.viewport()
			if r.Dx() != 5 || r.Dy() != 5 || !r.In(image.Rect(0, 0, size, size)) || !image.Pt(point%size, point/size).In(r) {
				t.Fatalf("wrong viewport: %v", r)
			}
		}
		d.center = 0
		stones, libs := d.outsideCounts()
		if stones != size-5 || libs != size-5 {
			t.Fatalf("outside counts=%d/%d", stones, libs)
		}
	}
}

// TestFocusPassAndInitialPosition はパス・初期局面に架空の着手対象を作らないことを検証する。
func TestFocusPassAndInitialPosition(t *testing.T) {
	c, before, after := adviceFixture(t, app.Jungo, []int{0, -1})
	for _, turn := range []int{0, 2} {
		a := reviewAdvice(c, turn, before, after)
		if len(a.focus) != 0 {
			t.Fatal("invented move focus")
		}
	}
	d := newReviewDiagram(before, after, nil)
	if _, ok := d.selectedGroup(); ok {
		t.Fatal("pass selected a group")
	}
	if newReviewDiagram(nil, nil, nil) != nil {
		t.Fatal("missing board created a diagram")
	}
}
