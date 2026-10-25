// 作成日: 2026-10-19。更新日: 2026-10-24。概要の文章と対象石を4ページで表示する。確定得点とAIの推定を区別し、盤面・戦績を変更しない。
package gui

import (
	"fmt"
	"image"
	"math"
	"strings"

	"github.com/Tremendous1192/NobiGo/internal/app"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// summaryDialog はページ・本文・独立盤面を所有する。再解析完了時だけ内容を更新する。
type summaryDialog struct {
	page, cachedPage          int
	ready                     bool
	source                    *app.SummarySession
	title, body, boardCaption string
	lines                     []string
	scroll, maxScroll         float64
	board                     *app.Board
	canvas                    *ebiten.Image
	move                      *app.SummaryMove
	marked                    []int
	alternativeOK             bool
	lesson                    *app.TechniqueLesson
	techniqueStep             int
	cachedTechniqueStep       int
	techniquePresented        bool
	saveAttempted, saveFailed bool
	close, retry              func()
}

// newSummaryDialog は1ページ目を選択し、画面固有の終了・再試行操作を保持する。
func newSummaryDialog(close, retry func()) *summaryDialog {
	return &summaryDialog{cachedPage: -1, close: close, retry: retry}
}

// changePage は4ページの範囲を守り、移動後に本文を先頭へ戻す。
func (d *summaryDialog) changePage(delta int) { d.page = max(0, min(3, d.page+delta)); d.scroll = 0 }

// scrollSummary は本文の端を越えずにスクロールする。
func (d *summaryDialog) scrollSummary(delta float64) {
	d.scroll = math.Max(0, math.Min(d.maxScroll, d.scroll+delta))
}

// summaryResult は採点で確定した結果だけを表示する。
func summaryResult(c *app.Controller) string {
	s := c.Score
	if s.NoResult {
		return "無勝負です。棋力調整には使用しません。"
	}
	unit := "目"
	if c.Board.Rule == app.Jungo {
		unit = "石"
	}
	result := "引き分け"
	if s.Winner != app.Empty {
		actor := "CPU"
		if s.Winner == c.Player {
			actor = "あなた"
		}
		result = fmt.Sprintf("%sの%.1f%s勝ち", actor, s.Margin, unit)
	}
	return fmt.Sprintf("%s\n確定得点：黒 %.1f ／ 白 %.1f%s", result, s.Black, s.White, unit)
}

// refreshSummaryPage はページ移動・再試行・解析完了時だけ文章と盤面を作り直す。
func (g *window) refreshSummaryPage(d *summaryDialog) {
	c := g.controller
	s := c.SummarySession
	if d.cachedPage == d.page && d.source == s && d.ready == s.Ready {
		if d.page == 3 && d.cachedTechniqueStep != d.techniqueStep {
			refreshTechniqueBoard(c, d)
			d.cachedTechniqueStep = d.techniqueStep
		}
		return
	}
	d.cachedPage, d.source, d.ready = d.page, s, s.Ready
	d.title, d.body, d.move = summaryPageContent(c, d.page)
	width := 510
	if d.page == 1 {
		width = 1020
	}
	d.lines = g.wrapLines(strings.TrimSpace(d.body), float64(width), 20)
	d.maxScroll = math.Max(0, float64(len(d.lines)*30-450))
	d.scrollSummary(0)
	d.board = nil
	d.marked = nil
	d.alternativeOK = false
	d.lesson = nil
	if d.page == 3 {
		d.lesson = c.ChooseTechnique()
		refreshTechniqueBoard(c, d)
		d.cachedTechniqueStep = d.techniqueStep
		return
	}
	if d.move != nil {
		stage := "着手後"
		scene, err := makeSummaryScene(c, d.move)
		if err != nil {
			d.boardCaption = "対象局面を再現できません。"
			return
		}
		d.board = scene.after
		if d.page == 3 {
			stage = "着手前"
			d.board = scene.before
		}
		d.marked = summaryMarkedPoints(scene, d.move, d.page == 3)
		d.alternativeOK = d.page == 3 && scene.alternative != nil
		d.boardCaption = fmt.Sprintf("%d手目の%s", d.move.Turn, stage)
	}
}

// updateSummaryInput は概要の本文にだけホイール・移動キーを適用する。
func (g *window) updateSummaryInput(d *summaryDialog) {
	x, y := ebiten.CursorPosition()
	right := 610
	if d.page == 1 {
		right = 1115
	}
	if image.Pt(x, y).In(image.Rect(65, 185, right, 640)) {
		_, wheel := ebiten.Wheel()
		d.scrollSummary(-wheel * 90)
	}
	for _, key := range []struct {
		key   ebiten.Key
		delta float64
	}{{ebiten.KeyArrowUp, -30}, {ebiten.KeyArrowDown, 30}, {ebiten.KeyPageUp, -420}, {ebiten.KeyPageDown, 420}, {ebiten.KeyHome, -d.maxScroll}, {ebiten.KeyEnd, d.maxScroll}} {
		if repeated(key.key) {
			d.scrollSummary(key.delta)
		}
	}
}

// drawSummaryDialog は詳細と同じ外枠に本文・盤面を表示し、最前面の操作だけを受け付ける。
func (g *window) drawSummaryDialog(screen *ebiten.Image, d *summaryDialog) {
	g.refreshSummaryPage(d)
	s := g.controller.SummarySession
	g.overlay(screen)
	rect(screen, image.Rect(40, 50, 1140, 755), panel)
	g.label(screen, fmt.Sprintf("感想戦（概要） %d / 4　%s", d.page+1, d.title), 65, 72, 27, foreground)
	status := fmt.Sprintf("AI評価済み %d / %d手 · 解析済みの範囲で選定", s.Data.Evaluated, len(g.controller.Board.Moves))
	if !s.Ready {
		status = fmt.Sprintf("追加解析中 %d / %d局面 · ページ移動・終了できます", s.Completed, s.Total)
	} else if s.Err != nil {
		status = "追加解析を完了できませんでした。取得済みの範囲を表示しています。"
	}
	if d.page == 3 && s.Ready && s.Err == nil {
		status = "今回の棋譜から選んだ教材 · 紹介は1局につき1種類"
	}
	g.label(screen, status, 65, 125, 17, muted)
	hint := "空き＝ひとまとまりの石の上下左右に接する空いた交点。"
	if g.controller.Board.Rule == app.Jungo {
		hint += " 純碁のAI評価は近似。得点は最後の石数です。"
	}
	g.label(screen, hint, 65, 153, 14, muted)
	width := 530
	if d.page == 1 {
		width = 1050
	}
	if d.canvas == nil || d.canvas.Bounds().Dx() != width {
		d.canvas = ebiten.NewImage(width, 450)
	}
	d.canvas.Fill(tint(panel))
	for i, line := range d.lines {
		y := float64(i*30) - d.scroll
		if y > -30 && y < 450 {
			g.label(d.canvas, line, 0, y, 20, foreground)
		}
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(65, 185)
	screen.DrawImage(d.canvas, op)
	if d.maxScroll > 0 {
		g.label(screen, "本文：ホイール・↑↓でスクロール", 65, 645, 14, muted)
	}
	if d.board != nil {
		g.drawSummaryBoard(screen, d)
	} else if d.page != 1 {
		g.label(screen, "対象盤面はありません", 720, 365, 20, muted)
	}
	g.button(screen, image.Rect(65, 695, 250, 737), "前のページ", d.page > 0, func() { d.changePage(-1) })
	g.button(screen, image.Rect(270, 695, 350, 737), "上へ", d.scroll > 0, func() { d.scrollSummary(-120) })
	g.button(screen, image.Rect(360, 695, 440, 737), "下へ", d.scroll < d.maxScroll, func() { d.scrollSummary(120) })
	if s.Ready && s.Err != nil {
		if !d.saveFailed {
			g.button(screen, image.Rect(460, 695, 645, 737), "解析を再試行", d.retry != nil, d.retry)
		}
	}
	if d.saveFailed {
		g.label(screen, "紹介履歴を保存できません。再試行できます。", 65, 670, 16, warning)
		g.button(screen, image.Rect(460, 695, 645, 737), "履歴を再保存", true, func() { d.saveAttempted = false })
	}
	g.button(screen, image.Rect(680, 695, 865, 737), "閉じる（Esc）", d.close != nil, d.close)
	next := "次のページ"
	if d.page == 3 {
		next = "感想戦終了"
	}
	g.button(screen, image.Rect(885, 695, 1110, 737), next, true, func() {
		if d.page == 3 {
			if d.close != nil {
				d.close()
			}
		} else {
			d.changePage(1)
		}
	})
	if d.page == 3 && d.lesson != nil && d.board != nil {
		d.techniquePresented = true
	}
}

// drawSummaryBoard は選定した独立局面を縮小表示する。改善ページの比較候補は着手前の盤面へ重ねる。
func (g *window) drawSummaryBoard(screen *ebiten.Image, d *summaryDialog) {
	b := d.board
	r := image.Rect(655, 225, 1105, 610)
	left, top, step := overviewGeometry(r.Inset(34), b.Size)
	g.label(screen, d.boardCaption, 655, 190, 21, foreground)
	rect(screen, r, 0xdab47b)
	for i := 0; i < b.Size; i++ {
		x, y := left+float64(i)*step, top+float64(i)*step
		vector.StrokeLine(screen, float32(x), float32(top), float32(x), float32(top+float64(b.Size-1)*step), 1, tint(0x654d32), true)
		vector.StrokeLine(screen, float32(left), float32(y), float32(left+float64(b.Size-1)*step), float32(y), 1, tint(0x654d32), true)
		column := strings.TrimRight(adviceVertex(i, b.Size), "0123456789")
		g.label(screen, column, x-4, float64(r.Min.Y)+2, 13, 0x302619)
		g.label(screen, fmt.Sprint(b.Size-i), left-step*.5-22, y-9, 13, 0x302619)
	}
	for point, stone := range b.Cells {
		if stone == app.Empty {
			continue
		}
		x, y := left+float64(point%b.Size)*step, top+float64(point/b.Size)*step
		color := uint32(0x18222d)
		if stone == app.White {
			color = 0xf1f2eb
		}
		vector.DrawFilledCircle(screen, float32(x), float32(y), float32(step*.42), tint(color), true)
	}
	if d.lesson != nil {
		g.drawTechniqueMarks(screen, d, left, top, step)
		return
	}
	stoneMark, captureMark := false, false
	for _, point := range d.marked {
		x, y := left+float64(point%b.Size)*step, top+float64(point/b.Size)*step
		radius := step * .47
		if b.Cells[point] == app.Empty {
			captureMark = true
			radius *= .5
			vector.StrokeLine(screen, float32(x-radius), float32(y-radius), float32(x+radius), float32(y+radius), 2, tint(0x805b08), true)
			vector.StrokeLine(screen, float32(x-radius), float32(y+radius), float32(x+radius), float32(y-radius), 2, tint(0x805b08), true)
		} else {
			stoneMark = true
			vector.StrokeRect(screen, float32(x-radius), float32(y-radius), float32(radius*2), float32(radius*2), 2, tint(0x805b08), true)
		}
	}
	for i, point := range []int{d.move.Point, d.move.Alternative} {
		if point < 0 || i == 1 && !d.alternativeOK {
			continue
		}
		x, y := left+float64(point%b.Size)*step, top+float64(point/b.Size)*step
		color := uint32(0xd64032)
		if i == 1 {
			color = 0x125bb4
		}
		vector.StrokeCircle(screen, float32(x), float32(y), float32(step*.46), 3, tint(color), true)
		letter := []string{"A", "B"}[i]
		ink := color
		if b.Cells[point] == app.Black {
			ink = 0xffffff
		} else if b.Cells[point] == app.White {
			ink = 0x18222d
		}
		g.label(screen, letter, x-4, y-9, 13, ink)
	}
	caption := "赤い印A：実際の手 " + adviceVertex(d.move.Point, b.Size)
	if d.move.Point < 0 {
		caption = "実際の手：パス（盤上の印なし）"
	}
	if d.alternativeOK {
		if d.move.Alternative < 0 {
			caption += "\nAI候補：パス（盤上の印なし）"
		} else {
			caption += "\n青い印B：AI候補 " + adviceVertex(d.move.Alternative, b.Size)
		}
	}
	var marks []string
	if stoneMark {
		marks = append(marks, "枠：説明する石")
	}
	if captureMark {
		marks = append(marks, "×：取った石の場所")
	}
	if len(marks) > 0 {
		caption += "\n" + strings.Join(marks, "　")
	}
	g.label(screen, caption, 655, 618, 15, muted)
}
