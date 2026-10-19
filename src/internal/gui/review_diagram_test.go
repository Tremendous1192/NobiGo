// 作成日: 2026-10-19。拡大図の座標とモーダル操作の分離・局面固定を検証する。
package gui

import (
	"bytes"
	"image"
	"math"
	"testing"
	"time"

	"github.com/Tremendous1192/PlayGo/engine"
	"github.com/Tremendous1192/PlayGo/internal/app"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// TestDiagramPointAllSizes は切り出した座標が全路数で元盤面へ一致し、図外を受理しないことを検証する。
func TestDiagramPointAllSizes(t *testing.T) {
	r := image.Rect(650, 230, 930, 595)
	for _, size := range []int{7, 9, 11, 13, 15, 19} {
		b, err := app.NewBoard(size, app.Japanese)
		if err != nil {
			t.Fatal(err)
		}
		d := newReviewDiagram(nil, b, nil)
		for _, center := range []int{0, size - 1, size*size - 1, size * size / 2} {
			d.center = center
			view := d.viewport()
			left, top, step := diagramGeometry(r, view)
			for y := 0; y < view.Dy(); y++ {
				for x := 0; x < view.Dx(); x++ {
					px, py := int(math.Round(left+float64(x)*step)), int(math.Round(top+float64(y)*step))
					if got := diagramPoint(px, py, r, view, size); got != (view.Min.Y+y)*size+view.Min.X+x {
						t.Fatalf("wrong point=%d", got)
					}
				}
			}
			if diagramPoint(0, 0, r, view, size) != -1 || diagramPoint(1180, 800, r, view, size) != -1 {
				t.Fatal("outside accepted")
			}
		}
	}
}

// TestOverviewFocusBounds は全路数・隅・辺・中央で、部分図の交点だけを枠が囲み、盤内に収まることを検証する。
func TestOverviewFocusBounds(t *testing.T) {
	r := image.Rect(940, 300, 1110, 470)
	for _, size := range []int{7, 9, 11, 13, 15, 19} {
		b, err := app.NewBoard(size, app.Japanese)
		if err != nil {
			t.Fatal(err)
		}
		d := newReviewDiagram(b, b, nil)
		left, top, step := overviewGeometry(r, size)
		for center := 0; center < size*size; center++ {
			d.center = center
			view := d.viewport()
			focus := overviewFocusBounds(r, view, size)
			if !focus.In(r.Inset(2)) {
				t.Fatalf("frame outside board: size=%d center=%d frame=%v", size, center, focus)
			}
			for point := 0; point < size*size; point++ {
				x, y := point%size, point/size
				pixel := image.Pt(int(math.Round(left+float64(x)*step)), int(math.Round(top+float64(y)*step)))
				if pixel.In(focus) != image.Pt(x, y).In(view) {
					t.Fatalf("frame and crop differ: size=%d center=%d point=%d", size, center, point)
				}
			}
			d.showBefore = true
			if overviewFocusBounds(r, d.viewport(), d.position().size) != focus {
				t.Fatal("before/after changed focus")
			}
			d.showBefore = false
		}
	}
}

// TestReviewFocusBounds は通常盤でも全路数・全中心位置で部分図と同じ交点だけを囲むことを検証する。
func TestReviewFocusBounds(t *testing.T) {
	for _, size := range []int{7, 9, 11, 13, 15, 19} {
		b, err := app.NewBoard(size, app.Japanese)
		if err != nil {
			t.Fatal(err)
		}
		d := newReviewDiagram(nil, b, nil)
		step := 530 / float64(size-1)
		for center := 0; center < size*size; center++ {
			d.center = center
			view := d.viewport()
			frame := reviewFocusBounds(view, size)
			if !frame.In(image.Rect(38, 123, 632, 717)) {
				t.Fatal("focus frame exceeded main board")
			}
			for point := 0; point < size*size; point++ {
				x, y := point%size, point/size
				pixel := image.Pt(int(math.Round(70+float64(x)*step)), int(math.Round(155+float64(y)*step)))
				if pixel.In(frame) != image.Pt(x, y).In(view) {
					t.Fatalf("main frame and crop differ: size=%d center=%d point=%d", size, center, point)
				}
			}
		}
	}
}

// TestReviewFocusSurvivesDialogAndResetsOnMove は詳細の開閉で範囲を維持し、手の移動・自動再生・画面離脱では更新することを検証する。
func TestReviewFocusSurvivesDialogAndResetsOnMove(t *testing.T) {
	c, before, after := adviceFixture(t, app.Japanese, []int{2, 1, 8, 7, 0, -1, -1})
	c.Phase = app.Review
	cache := func() {
		for i := range c.Evaluations {
			c.Evaluations[i] = &engine.Analysis{LocalRead: true}
		}
	}
	cache()
	g := &window{controller: c, reviewTurn: 7, reviewBefore: before, reviewBoard: after, shownPhase: app.Review}
	if err := g.setReview(5); err != nil {
		t.Fatal(err)
	}
	d := g.currentReviewFocus()
	if d == nil || d.point != 0 || g.advice != nil {
		t.Fatal("focus unavailable before opening detail")
	}
	g.openReviewAdvice()
	if g.advice.diagram != d {
		t.Fatal("detail changed focus on open")
	}
	d.selectTarget(1)
	d.pan(2, 2)
	d.showBefore = true
	view := d.viewport()
	g.dismiss()
	if g.currentReviewFocus() != d || d.viewport() != view || g.reviewBoard.Cells[1] != app.Empty {
		t.Fatal("close lost focus or restored the before board")
	}
	g.openReviewAdvice()
	if g.advice.diagram != d || d.viewport() != view || !d.showBefore || d.target != 1 {
		t.Fatal("reopening lost diagram navigation")
	}
	cache()
	if err := g.setReview(3); err != nil {
		t.Fatal(err)
	}
	if g.advice != nil || g.currentReviewFocus() == d || g.reviewFocus.point != 8 || g.reviewFocus.showBefore {
		t.Fatal("move change retained old focus")
	}
	d = g.reviewFocus
	g.autoplay = true
	g.nextReplay = time.Time{}
	cache()
	g.advanceReview(time.Unix(1, 0))
	if g.reviewTurn != 4 || g.currentReviewFocus() == d || g.reviewFocus.point != 7 {
		t.Fatal("autoplay retained old focus")
	}
	for _, turn := range []int{0, 6} {
		cache()
		if err := g.setReview(turn); err != nil {
			t.Fatal(err)
		}
		if focus := g.currentReviewFocus(); focus == nil || len(focus.targets) != 0 || focus.viewport().Empty() {
			t.Fatal("initial or pass range differs from detail")
		}
	}
	c.Phase = app.Playing
	g.syncPhase()
	if g.reviewFocus != nil || g.currentReviewFocus() != nil {
		t.Fatal("review focus survived into playing")
	}
}

// TestDiagramControlsKeepTextAndGameFixed は対象・前後・移動が本文や元棋譜へ作用せず、閉じても再生しないことを検証する。
func TestDiagramControlsKeepTextAndGameFixed(t *testing.T) {
	source, err := text.NewGoTextFaceSource(bytes.NewReader(fontData))
	if err != nil {
		t.Fatal(err)
	}
	c, before, after := adviceFixture(t, app.Japanese, []int{2, 1, 8, 7, 0})
	c.Phase = app.Review
	g := &window{face: &text.GoTextFace{Source: source}, controller: c, reviewTurn: 5, reviewBefore: before, reviewBoard: after, autoplay: true}
	g.openReviewAdvice()
	screen := ebiten.NewImage(1180, 800)
	body := g.advice.body
	click := func(x, y int) {
		t.Helper()
		g.drawAdviceDialog(screen)
		for i := len(g.actions) - 1; i >= 0; i-- {
			h := g.actions[i]
			if image.Pt(x, y).In(h.rect) {
				h.action()
				return
			}
		}
		t.Fatalf("no control at %d,%d", x, y)
	}
	click(850, 200)
	if g.advice.diagram.targets[g.advice.diagram.target].label != "取られた連" {
		t.Fatal("wrong next target")
	}
	if _, ok := g.advice.diagram.selectedGroup(); ok {
		t.Fatal("captured stone still present")
	}
	click(1000, 200)
	if _, ok := g.advice.diagram.selectedGroup(); !ok {
		t.Fatal("before position missing captured group")
	}
	click(930, 680)
	if g.advice.diagram.viewport().Min.X == 0 {
		t.Fatal("pan did not move")
	}
	click(1040, 680)
	if g.advice.diagram.viewport().Min.X != 0 {
		t.Fatal("recenter failed")
	}
	if g.advice.body != body || g.autoplay || len(c.Board.Moves) != 5 || c.Board.Cells[1] != app.Empty {
		t.Fatal("diagram controls changed game or text")
	}
	click(1000, 720)
	if g.advice != nil || g.autoplay {
		t.Fatal("close resumed playback")
	}
}

// TestDiagramPanningPreservesSelection は移動端と選択した連の維持を検証する。
func TestDiagramPanningPreservesSelection(t *testing.T) {
	b, err := app.NewBoard(19, app.Japanese)
	if err != nil {
		t.Fatal(err)
	}
	b.Cells[0] = app.Black
	d := newReviewDiagram(nil, b, []focusTarget{{point: 0}})
	d.pan(100, 100)
	if d.viewport() != image.Rect(14, 14, 19, 19) || d.point != 0 {
		t.Fatal("pan exceeded board or changed selection")
	}
	d.pan(-100, -100)
	if d.viewport() != image.Rect(0, 0, 5, 5) || d.point != 0 {
		t.Fatal("pan start incorrect")
	}
}
