// 作成日: 2026-10-18。感想戦の詳細を拡大・スクロール表示する。表示と操作の所有者はGUIスレッドのみ。
package gui

import (
	"fmt"
	"image"
	"math"

	"github.com/Tremendous1192/PlayGo/internal/app"
	"github.com/hajimehoshi/ebiten/v2"
)

const (
	adviceWidth      = 800
	adviceHeight     = 450
	adviceFontSize   = 22
	adviceLineHeight = 33
)

// adviceDialog は開いた時点の文章を所有する。解析到着で読んでいる行が動かないよう本文を固定する。
type adviceDialog struct {
	turn              int
	body, status      string
	lines             []string
	scroll, maxScroll float64
}

// openReviewAdvice は現在表示中の手を固定し、自動再生を停止して詳細を開く。
func (g *window) openReviewAdvice() {
	c := g.controller
	if c == nil || c.Phase != app.Review || g.reviewBoard == nil {
		return
	}
	g.dismiss()
	g.autoplay = false
	d := &adviceDialog{turn: g.reviewTurn, body: reviewAdvice(c, g.reviewTurn, g.reviewBefore, g.reviewBoard).detail}
	if c.Err != nil {
		d.status = "解析失敗：盤面の事実を表示しています。閉じて再試行できます。"
	} else if c.Busy {
		d.status = "解析中：比較候補は解析完了後に開き直すと確認できます。"
	}
	g.advice = d
}

// scrollAdvice は本文の上下端でスクロールを止める。
func (g *window) scrollAdvice(delta float64) {
	if g.advice == nil {
		return
	}
	d := g.advice
	d.scroll = math.Max(0, math.Min(d.maxScroll, d.scroll+delta))
}

// updateAdviceInput は閲覧中のホイールとキー操作だけを本文へ適用する。
func (g *window) updateAdviceInput() {
	x, y := ebiten.CursorPosition()
	if image.Pt(x, y).In(image.Rect(150, 65, 1030, 740)) {
		_, wheel := ebiten.Wheel()
		g.scrollAdvice(-wheel * adviceLineHeight * 3)
	}
	for _, item := range []struct {
		key   ebiten.Key
		delta float64
	}{
		{ebiten.KeyArrowUp, -adviceLineHeight}, {ebiten.KeyArrowDown, adviceLineHeight},
		{ebiten.KeyPageUp, -(adviceHeight - adviceLineHeight)}, {ebiten.KeyPageDown, adviceHeight - adviceLineHeight},
		{ebiten.KeyHome, -g.advice.maxScroll}, {ebiten.KeyEnd, g.advice.maxScroll},
	} {
		if repeated(item.key) {
			g.scrollAdvice(item.delta)
		}
	}
}

// drawAdviceDialog は全文を一定サイズの描画領域へ切り取り、背面の操作を遮断する。
func (g *window) drawAdviceDialog(screen *ebiten.Image) {
	d := g.advice
	g.overlay(screen)
	rect(screen, image.Rect(150, 65, 1030, 740), panel)
	g.label(screen, fmt.Sprintf("%d手目のアドバイス", d.turn), 180, 85, 28, foreground)
	g.label(screen, "自動再生は停止中です。ホイール・↑↓・PageUp / PageDownで読めます。", 180, 130, 16, muted)
	if d.lines == nil {
		d.lines = g.wrapLines(d.body, adviceWidth-16, adviceFontSize)
		d.maxScroll = math.Max(0, float64(len(d.lines)*adviceLineHeight+16-adviceHeight))
		g.scrollAdvice(0)
	}
	if g.adviceCanvas == nil {
		g.adviceCanvas = ebiten.NewImage(adviceWidth, adviceHeight)
	}
	g.adviceCanvas.Fill(tint(panel))
	for i, line := range d.lines {
		y := 8 + float64(i*adviceLineHeight) - d.scroll
		if y > -adviceLineHeight && y < adviceHeight {
			g.label(g.adviceCanvas, line, 4, y, adviceFontSize, foreground)
		}
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(180, 175)
	screen.DrawImage(g.adviceCanvas, op)
	if d.maxScroll > 0 {
		rect(screen, image.Rect(1000, 175, 1005, 625), surface)
		height := max(25, int(float64(adviceHeight*adviceHeight)/(adviceHeight+d.maxScroll)))
		y := 175 + int(d.scroll/d.maxScroll*float64(adviceHeight-height))
		rect(screen, image.Rect(1000, y, 1005, y+height), accent)
	}
	g.label(screen, d.status, 180, 637, 15, warning)
	g.button(screen, image.Rect(180, 677, 320, 720), "上へ", d.scroll > 0, func() { g.scrollAdvice(-adviceLineHeight * 3) })
	g.button(screen, image.Rect(335, 677, 475, 720), "下へ", d.scroll < d.maxScroll, func() { g.scrollAdvice(adviceLineHeight * 3) })
	g.button(screen, image.Rect(810, 677, 1005, 720), "閉じる（Esc）", true, g.dismiss)
}
