// 作成日: 2026-10-18。感想戦は保存済み対局を変更せず再生する。助言は盤面と解析結果だけから作る。
package gui

import (
	"fmt"
	"image"
	"time"

	"github.com/Tremendous1192/PlayGo/engine"
	"github.com/Tremendous1192/PlayGo/internal/app"
	"github.com/hajimehoshi/ebiten/v2"
)

// setReview は再生位置を更新し、必要な局面解析を要求する。
func (g *window) setReview(turn int) {
	b, err := g.controller.ReviewAt(turn)
	if err != nil {
		g.notice = "この局面を再生できません。結果画面へ戻ってください。"
		g.autoplay = false
		return
	}
	g.reviewBoard = b
	g.reviewTurn = turn
	g.notice = ""
	g.nextReplay = time.Now().Add(1200 * time.Millisecond)
}

// drawReview は元の棋譜・戦績を変えずに再生する。
func (g *window) drawReview(screen *ebiten.Image) {
	c := g.controller
	b := g.reviewBoard
	if b == nil {
		g.setReview(0)
		b = g.reviewBoard
		if b == nil {
			return
		}
	}
	g.label(screen, "感想戦", 35, 25, 34, foreground)
	g.label(screen, fmt.Sprintf("%s · %d路 · %d / %d手", ruleName(b.Rule), b.Size, g.reviewTurn, len(c.Board.Moves)), 230, 36, 21, muted)
	status := "一手ずつ振り返り、次の対局のヒントを見つけましょう。"
	if c.Busy {
		status = "この局面を解析しています…（戦績は変更しません）"
	}
	g.label(screen, status, 38, 82, 19, accent)
	a := g.currentAnalysis(g.reviewTurn)
	if g.reviewTurn == len(c.Board.Moves) && a == nil {
		a = scoreOwnership(c.Score)
	}
	g.drawBoard(screen, b, a, nil, nil, false)
	g.drawSidebar(screen, b, g.reviewTurn)
	g.button(screen, image.Rect(35, 741, 215, 788), "一手戻る", g.reviewTurn > 0, func() { g.autoplay = false; g.setReview(g.reviewTurn - 1) })
	playLabel := "自動再生"
	if g.autoplay {
		playLabel = "再生を停止"
	}
	g.button(screen, image.Rect(229, 741, 409, 788), playLabel, true, func() {
		if g.reviewTurn == len(c.Board.Moves) {
			g.setReview(0)
		}
		g.autoplay = !g.autoplay
		g.nextReplay = time.Now().Add(1200 * time.Millisecond)
	})
	g.button(screen, image.Rect(423, 741, 635, 788), "一手進める", g.reviewTurn < len(c.Board.Moves), func() { g.autoplay = false; g.setReview(g.reviewTurn + 1) })
	if c.Err != nil {
		g.button(screen, image.Rect(660, 741, 865, 788), "解析を再試行", true, func() { g.setReview(g.reviewTurn) })
	}
	g.button(screen, image.Rect(880, 741, 1145, 788), "感想戦を終える", true, func() { g.autoplay = false; c.EndReview() })
}

// reviewAdvice は候補手と推定損失を示す。数値が欠けている場合は断定しない。
func reviewAdvice(c *app.Controller, turn int) string {
	if turn == 0 {
		return "最初の局面です。隅・辺・中央では、\n囲むために必要な石数が違います。"
	}
	move := c.Board.Moves[turn-1]
	if move.Point < 0 {
		return "パスの前に、取れる石と打ち残しを確認しましょう。\n純碁では石を盤上に増やすことが得点につながります。"
	}
	before := c.Evaluations[turn-1]
	if before == nil || len(before.Moves) == 0 {
		return "この手の前の局面へ戻ると解析を補えます。\n石の連の呼吸点を数えて比較しましょう。"
	}
	best := before.Moves[0].Point
	if best == move.Point {
		return "この手はエンジンの第一候補と一致しました。\n石のつながりと次の狙いを確かめましょう。"
	}
	vertex, err := engine.Vertex(best, c.Board.Size)
	if err != nil {
		return "候補手の座標を取得できませんでした。"
	}
	if best < 0 {
		vertex = "パス"
	}
	text := "比較する候補：" + vertex + "。前の局面で狙いを考えてみましょう。"
	if loss, ok := c.Loss(turn); ok {
		text += fmt.Sprintf("\nこの手の推定損失は %.1f目です。", loss)
	}
	if c.Board.Rule == app.Jungo {
		text += "\n純碁の助言は近似評価に基づきます。"
	}
	return text
}
