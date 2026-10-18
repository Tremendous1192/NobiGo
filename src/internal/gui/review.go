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
// 失敗は画面へ案内して再生も停止するため、再生ボタン側では終了エラーへ変換しない。
func (g *window) setReview(turn int) error {
	var before *app.Board
	if turn > 0 {
		var err error
		before, err = g.controller.Snapshot(turn - 1)
		if err != nil {
			g.notice = "直前の局面を再生できません。結果画面へ戻ってください。"
			g.autoplay = false
			return err
		}
	}
	b, err := g.controller.ReviewAt(turn)
	if err != nil {
		g.notice = "この局面を再生できません。結果画面へ戻ってください。"
		g.autoplay = false
		return err
	}
	g.reviewBoard = b
	g.reviewBefore = before
	g.reviewTurn = turn
	g.notice = ""
	g.nextReplay = time.Now().Add(1200 * time.Millisecond)
	return nil
}

// advanceReview は必要な解析の完了を待って再生する。失敗を飛ばさず停止し、再試行の操作を残す。
func (g *window) advanceReview(now time.Time) {
	c := g.controller
	if !g.autoplay || c.Phase != app.Review {
		return
	}
	if c.Err != nil || g.reviewTurn >= len(c.Board.Moves) {
		g.autoplay = false
		return
	}
	if !c.Busy && now.After(g.nextReplay) {
		_ = g.setReview(g.reviewTurn + 1)
	}
}

// drawReview は元の棋譜・戦績を変えずに再生する。
func (g *window) drawReview(screen *ebiten.Image) {
	c := g.controller
	b := g.reviewBoard
	if b == nil {
		_ = g.setReview(0)
		b = g.reviewBoard
		if b == nil {
			return
		}
	}
	g.label(screen, "感想戦", 35, 25, 34, foreground)
	g.label(screen, fmt.Sprintf("%s · %d路 · %d / %d手", ruleName(b.Rule), b.Size, g.reviewTurn, len(c.Board.Moves)), 230, 36, 21, muted)
	status := "一手ずつ振り返り、次の対局のヒントを見つけましょう。"
	if c.Busy {
		status = "この手と直前の局面を解析しています…"
	}
	g.label(screen, status, 38, 82, 19, accent)
	a := g.currentAnalysis(g.reviewTurn)
	if g.reviewTurn == len(c.Board.Moves) && a == nil {
		a = scoreOwnership(c.Score)
	}
	g.drawBoard(screen, b, a, nil, nil, false)
	g.drawSidebar(screen, b, g.reviewTurn)
	g.button(screen, image.Rect(35, 741, 215, 788), "一手戻る", g.reviewTurn > 0, func() { g.autoplay = false; _ = g.setReview(g.reviewTurn - 1) })
	playLabel := "自動再生"
	if g.autoplay {
		playLabel = "再生を停止"
	}
	g.button(screen, image.Rect(229, 741, 409, 788), playLabel, true, func() {
		if g.reviewTurn == len(c.Board.Moves) {
			if err := g.setReview(0); err != nil {
				return
			}
		}
		g.autoplay = !g.autoplay
		g.nextReplay = time.Now().Add(1200 * time.Millisecond)
	})
	g.button(screen, image.Rect(423, 741, 635, 788), "一手進める", g.reviewTurn < len(c.Board.Moves), func() { g.autoplay = false; _ = g.setReview(g.reviewTurn + 1) })
	if c.Err != nil {
		g.button(screen, image.Rect(660, 741, 865, 788), "解析を再試行", true, func() { _ = g.setReview(g.reviewTurn) })
	}
	g.button(screen, image.Rect(880, 741, 1145, 788), "感想戦を終える", true, g.finishReview)
}

// reviewAdvice は直前に打たれた手の効果と学びを示す。前後の盤面は再生位置変更時に用意する。
func reviewAdvice(c *app.Controller, turn int, before, after *app.Board) string {
	if turn == 0 {
		return "最初の局面です。隅・辺・中央では、\n囲むために必要な石数が違います。"
	}
	if c.Board == nil || turn < 0 || turn > len(c.Board.Moves) || before == nil || after == nil {
		return "局面を準備しています。"
	}
	move := c.Board.Moves[turn-1]
	playerMove := move.Color == c.Player
	if move.Point < 0 {
		if !playerMove {
			return "CPUはパスで終局を提案しました。\n取れる石と打ち残しを確認して、続けるか決めましょう。"
		}
		if c.Board.Rule == app.Jungo {
			return "あなたはパスしました。\n純碁は盤上の石数で得点が決まります。\n取れる石と、安全に増やせる石を確認しましょう。"
		}
		return "あなたはパスしました。\n両者の連続パスで終局します。\n地を増やす手や、守る必要のある石を確認しましょう。"
	}
	effect, lesson := moveLesson(before, after, move)
	if !playerMove {
		return "CPUの狙い（推定）\n" + effect
	}
	body := "あなたの手：" + effect + "\n" + lesson
	if turn-1 < len(c.Evaluations) {
		a := c.Evaluations[turn-1]
		if loss, ok := c.Loss(turn); ok && a != nil && len(a.Moves) > 0 && loss >= 1 && a.Moves[0].Point != move.Point && before.Legal(a.Moves[0].Point) {
			vertex, err := engine.Vertex(a.Moves[0].Point, before.Size)
			if err == nil {
				if a.Moves[0].Point < 0 {
					vertex = "パス"
				}
				body += "\n比較候補：" + vertex + "。呼吸点とつながりを比べましょう。"
			}
		}
	}
	return body
}

// moveLesson は盤面で確かめられる効果を優先し、未知の読み筋を意図として断定しない。
func moveLesson(before, after *app.Board, move app.Move) (effect, lesson string) {
	taken := after.Captured[move.Color] - before.Captured[move.Color]
	_, liberties := after.Group(move.Point)
	if taken > 0 {
		return fmt.Sprintf("相手の石を%d個取りました。", taken), "取る前に、相手の最後の呼吸点を確認しましょう。"
	}
	connected := make(map[int]bool)
	rescued, attacked := false, false
	for _, point := range app.Neighbors(move.Point, before.Size) {
		stones, oldLiberties := before.Group(point)
		if len(stones) == 0 {
			continue
		}
		if before.Cells[point] == move.Color {
			root := stones[0]
			for _, p := range stones {
				root = min(root, p)
			}
			connected[root] = true
			rescued = rescued || len(oldLiberties) == 1 && len(liberties) > 1
		} else {
			_, newLiberties := after.Group(point)
			attacked = attacked || len(oldLiberties) > 1 && len(newLiberties) == 1
		}
	}
	if rescued {
		return fmt.Sprintf("アタリの石を助け、呼吸点を%dに増やしました。", len(liberties)), "逃げる手と、相手の石を取る手を比べましょう。"
	}
	if len(liberties) == 1 {
		return "この石の連は呼吸点が1つです。", "アタリです。相手に取られないか確認しましょう。"
	}
	if attacked {
		return "相手の石をアタリにしました。", "相手が逃げた後の、自分の石の安全も見ましょう。"
	}
	if len(connected) >= 2 {
		return "離れていた自分の石をつなぎました。", "つながると、一つの連として呼吸点を共有します。"
	}
	if len(connected) == 1 {
		return fmt.Sprintf("自分の石から伸び、呼吸点は%dです。", len(liberties)), "次も安全につながれるか、周りを見てみましょう。"
	}
	return fmt.Sprintf("新しい場所に石を置き、呼吸点は%dです。", len(liberties)), "近くの石とのつながりも考えてみましょう。"
}
