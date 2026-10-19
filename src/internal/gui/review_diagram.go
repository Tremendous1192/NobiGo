// 作成日: 2026-10-19。固定した感想戦局面の部分図と石のつながりを描く。操作はGUI所有の図だけを変更する。
package gui

import (
	"fmt"
	"image"
	"math"
	"slices"

	"github.com/Tremendous1192/NobiGo/internal/app"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// diagramGeometry は縦横比を保った部分図の交点間隔と左上の位置を返す。
func diagramGeometry(r, view image.Rectangle) (x, y, step float64) {
	step = math.Min(float64(r.Dx()-96)/float64(max(1, view.Dx()-1)), float64(r.Dy()-96)/float64(max(1, view.Dy()-1)))
	x = float64(r.Min.X) + (float64(r.Dx())-step*float64(view.Dx()-1))/2
	y = float64(r.Min.Y) + (float64(r.Dy())-step*float64(view.Dy()-1))/2
	return
}

// diagramPoint は部分図のクリックを元の盤面の交点へ変換する。
func diagramPoint(x, y int, r, view image.Rectangle, size int) int {
	left, top, step := diagramGeometry(r, view)
	fx, fy := (float64(x)-left)/step, (float64(y)-top)/step
	ix, iy := int(math.Round(fx)), int(math.Round(fy))
	if ix < 0 || iy < 0 || ix >= view.Dx() || iy >= view.Dy() || math.Abs(fx-float64(ix)) > .48 || math.Abs(fy-float64(iy)) > .48 {
		return -1
	}
	return (iy+view.Min.Y)*size + ix + view.Min.X
}

// drawReviewDiagram は説明対象・前後切替・移動と、選択した連の石数を表示する。
func (g *window) drawReviewDiagram(screen *ebiten.Image, d *reviewDiagram) {
	p := d.position()
	turn := g.reviewTurn
	if d.showBefore {
		turn--
	}
	g.refreshTactics(p, turn)
	title := "部分図：石を選んで確認"
	if len(d.targets) > 0 {
		target := d.targets[d.target]
		title = fmt.Sprintf("%s %s（%d/%d）", target.label, adviceVertex(target.point, p.size), d.target+1, len(d.targets))
	}
	g.label(screen, g.fit(title, 460, 18), 650, 154, 18, foreground)
	g.button(screen, image.Rect(650, 185, 778, 223), "前の対象", d.target > 0, func() { d.selectTarget(d.target - 1) })
	g.button(screen, image.Rect(786, 185, 914, 223), "次の対象", d.target+1 < len(d.targets), func() { d.selectTarget(d.target + 1) })
	switchLabel := "着手前へ"
	if d.showBefore {
		switchLabel = "着手後へ"
	}
	g.button(screen, image.Rect(922, 185, 1110, 223), switchLabel, d.before.size > 0, func() { d.showBefore = !d.showBefore })
	r := image.Rect(650, 230, 930, 595)
	g.drawDiagramBoard(screen, d, r)
	g.drawDiagramOverview(screen, d, image.Rect(940, 300, 1110, 470))
	group, ok := d.selectedGroup()
	status := "石を選ぶと、同じ連を囲みます。"
	if ok {
		status = fmt.Sprintf("%s %s：連%d石", colorName(p.cells[d.point]), adviceVertex(d.point, p.size), len(group.stones))
	} else if d.point >= 0 {
		status = adviceVertex(d.point, p.size) + "は空点です。前後を切り替えて確認。"
	}
	g.label(screen, g.fit(status, 460, 17), 650, 602, 17, foreground)
	outStones, _ := d.outsideCounts()
	caption := "着手後"
	if d.showBefore {
		caption = "着手前"
	}
	if outStones > 0 {
		caption += fmt.Sprintf(" · 図外の同じ連：%d石（総数に含む）", outStones)
	}
	g.label(screen, caption, 650, 632, 14, warning)
	view := d.viewport()
	for i, item := range []struct {
		label   string
		dx, dy  int
		enabled bool
	}{
		{"←", -2, 0, view.Min.X > 0}, {"↑", 0, -2, view.Min.Y > 0}, {"↓", 0, 2, view.Max.Y < p.size}, {"→", 2, 0, view.Max.X < p.size},
	} {
		x := 650 + i*84
		g.button(screen, image.Rect(x, 661, x+76, 699), item.label, item.enabled, func() { d.pan(item.dx, item.dy) })
	}
	g.button(screen, image.Rect(986, 661, 1110, 699), "選択へ", d.point >= 0, func() { d.center = d.point })
}

// overviewGeometry は全体盤の交点を正方形へ配置し、端の石と範囲枠の余白を確保する。
func overviewGeometry(r image.Rectangle, size int) (left, top, step float64) {
	step = float64(min(r.Dx(), r.Dy())-32) / float64(size-1)
	left = float64(r.Min.X) + (float64(r.Dx())-step*float64(size-1))/2
	top = float64(r.Min.Y) + (float64(r.Dy())-step*float64(size-1))/2
	return
}

// overviewFocusBounds は部分図に含まれる交点を、石の半径を含めた全体盤上の枠へ変換する。
func overviewFocusBounds(r, view image.Rectangle, size int) image.Rectangle {
	left, top, step := overviewGeometry(r, size)
	return focusBounds(view, left, top, step)
}

// focusBounds は同じ盤上範囲を各盤面の交点間隔へ写し、石の周囲を半路分含める。
func focusBounds(view image.Rectangle, left, top, step float64) image.Rectangle {
	return image.Rect(
		int(math.Floor(left+(float64(view.Min.X)-.5)*step)),
		int(math.Floor(top+(float64(view.Min.Y)-.5)*step)),
		int(math.Ceil(left+(float64(view.Max.X)-.5)*step)),
		int(math.Ceil(top+(float64(view.Max.Y)-.5)*step)),
	)
}

// reviewFocusBounds は通常盤の表示範囲を返す。7路などで半路が盤の余白を超える場合だけ盤内へ収める。
func reviewFocusBounds(view image.Rectangle, size int) image.Rectangle {
	return focusBounds(view, 70, 155, 530/float64(size-1)).Intersect(image.Rect(38, 123, 632, 717))
}

// drawFocusFrame は領域の色分けや石の色に埋もれない二重の範囲枠を描く。
func drawFocusFrame(screen *ebiten.Image, focus image.Rectangle) {
	for _, stroke := range []struct {
		width float32
		color uint32
	}{{5, 0xffffff}, {3, 0x0d526a}} {
		vector.StrokeRect(screen, float32(focus.Min.X), float32(focus.Min.Y), float32(focus.Dx()), float32(focus.Dy()), stroke.width, tint(stroke.color), true)
	}
}

// drawReviewFocus は詳細を開く前と閉じた後に、同じ部分図の範囲を現在の感想戦盤面へ示す。
func (g *window) drawReviewFocus(screen *ebiten.Image) {
	if g.advice != nil {
		return
	}
	if d := g.currentReviewFocus(); d != nil {
		g.refreshTactics(&d.after, g.reviewTurn)
		step := 530 / float64(g.reviewBoard.Size-1)
		bounds := reviewFocusBounds(d.viewport(), g.reviewBoard.Size)
		g.drawTactics(screen.SubImage(bounds).(*ebiten.Image), &d.after, d.viewport(), 70, 155, step, min(step*.45, 28))
		drawFocusFrame(screen, bounds)
		legendY := 586.0
		if g.tab != 2 {
			legendY = 610
		}
		g.drawTacticsLegend(screen, &d.after, 685, legendY)
		g.label(screen, "枠：詳細の拡大範囲", 880, 85, 15, muted)
	}
}

// drawDiagramOverview は部分図と同じ前後局面を全体表示し、現在の表示範囲を枠で囲む。
func (g *window) drawDiagramOverview(screen *ebiten.Image, d *reviewDiagram, r image.Rectangle) {
	p := d.position()
	left, top, step := overviewGeometry(r, p.size)
	g.label(screen, "全体盤", float64(r.Min.X), float64(r.Min.Y-30), 17, foreground)
	rect(screen, r, 0xdab47b)
	endX, endY := float32(left+float64(p.size-1)*step), float32(top+float64(p.size-1)*step)
	for i := 0; i < p.size; i++ {
		x, y := float32(left+float64(i)*step), float32(top+float64(i)*step)
		vector.StrokeLine(screen, x, float32(top), x, endY, 1, tint(0x654d32), true)
		vector.StrokeLine(screen, float32(left), y, endX, y, 1, tint(0x654d32), true)
	}
	for point, color := range p.cells {
		if color == app.Empty {
			continue
		}
		stone := uint32(0x18222d)
		if color == app.White {
			stone = 0xf1f2eb
		}
		x, y := float32(left+float64(point%p.size)*step), float32(top+float64(point/p.size)*step)
		vector.DrawFilledCircle(screen, x, y, float32(step*.42), tint(stone), true)
	}
	focus := overviewFocusBounds(r, d.viewport(), p.size)
	drawFocusFrame(screen, focus)
	g.label(screen, "枠内を左に拡大", float64(r.Min.X), float64(r.Max.Y+12), 15, foreground)
	stage := "着手後"
	if d.showBefore {
		stage = "着手前"
	}
	g.label(screen, stage, float64(r.Min.X), float64(r.Max.Y+38), 14, muted)
	g.label(screen, fmt.Sprintf("AI %d変化・最大%d手", p.read.variations, p.read.depth), float64(r.Min.X), float64(r.Max.Y+62), 12, muted)
	g.label(screen, "共有呼吸点への攻めを\n読めた範囲で連絡評価", float64(r.Min.X), float64(r.Max.Y+82), 12, muted)
}

// drawDiagramBoard は選択連・着手番号と着目範囲内のつながりを部分図へ重ねる。
func (g *window) drawDiagramBoard(screen *ebiten.Image, d *reviewDiagram, r image.Rectangle) {
	p, view := d.position(), d.viewport()
	left, top, step := diagramGeometry(r, view)
	rect(screen, r, 0xdab47b)
	for x := 0; x < view.Dx(); x++ {
		px := float32(left + float64(x)*step)
		vector.StrokeLine(screen, px, float32(top), px, float32(top+float64(view.Dy()-1)*step), 1, tint(0x654d32), true)
		g.label(screen, string("ABCDEFGHJKLMNOPQRST"[view.Min.X+x]), float64(px)-5, float64(r.Max.Y-16), 12, 0x654d32)
	}
	for y := 0; y < view.Dy(); y++ {
		py := float32(top + float64(y)*step)
		vector.StrokeLine(screen, float32(left), py, float32(left+float64(view.Dx()-1)*step), py, 1, tint(0x654d32), true)
		g.label(screen, fmt.Sprint(p.size-view.Min.Y-y), float64(r.Min.X+8), float64(py)-9, 12, 0x654d32)
	}
	group, _ := d.selectedGroup()
	for y := view.Min.Y; y < view.Max.Y; y++ {
		for x := view.Min.X; x < view.Max.X; x++ {
			point := y*p.size + x
			px, py := float32(left+float64(x-view.Min.X)*step), float32(top+float64(y-view.Min.Y)*step)
			color := p.cells[point]
			if color == app.Empty {
				continue
			}
			radius := float32(step * .46)
			stone := uint32(0x18222d)
			if color == app.White {
				stone = 0xf1f2eb
			}
			vector.DrawFilledCircle(screen, px, py, radius, tint(stone), true)
			if slices.Contains(group.stones, point) {
				vector.StrokeCircle(screen, px, py, radius+2, 3, tint(0xa95d08), true)
			}
			if number := p.numbers[point]; number > 0 {
				g.drawStoneNumber(screen, number, float64(px), float64(py), float64(radius), color)
			}
		}
	}
	g.drawTactics(screen.SubImage(r).(*ebiten.Image), p, view, left-float64(view.Min.X)*step, top-float64(view.Min.Y)*step, step, step*.46)
	g.actions = append(g.actions, hit{rect: r, action: func() {
		x, y := ebiten.CursorPosition()
		d.selectStone(diagramPoint(x, y, r, d.viewport(), d.position().size))
	}})
}
