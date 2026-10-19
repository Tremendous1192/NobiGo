// 作成日: 2026-10-19。感想戦の概要を4ページで表示する。確定得点とAIの推定を区別し、盤面・戦績を変更しない。
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

// summaryStageText は実着手の集計を根拠に、段階ごとの狙いを推定として説明する。
func summaryStageText(stage app.SummaryStage, name string) string {
	if stage.CPUMoves == 0 {
		return name + "：説明できるCPUの着手がありません。"
	}
	aim := "一つの狙いに絞れる材料はありません。"
	switch {
	case stage.Saved > 0:
		aim = "危険な石を助け、自分の形を保とうとしたと考えられます。"
	case stage.Captured > 0:
		aim = "相手の石を取り、周囲で打ちやすくする狙いが考えられます。"
	case stage.Connected > 0:
		aim = "離れた石をつなぎ、分断されにくくする狙いが考えられます。"
	case stage.EdgeMoves*2 >= stage.CPUMoves:
		aim = "盤の端を利用して、隅や辺から形を作る狙いが考えられます。"
	}
	return fmt.Sprintf("%s（%d～%d手目）\nCPU %d手／隅・辺 %d手／取り %d個／救出 %d個／連結 %d組。\n%s", name, stage.From, stage.To, stage.CPUMoves, stage.EdgeMoves, stage.Captured, stage.Saved, stage.Connected, aim)
}

// summaryPageContent は各ページの文章と根拠となる手を返す。根拠のない好手・戦略は補わない。
func summaryPageContent(c *app.Controller, page int) (title, body string, move *app.SummaryMove) {
	s := c.SummarySession.Data
	switch page {
	case 0:
		title = "結果と一局のハイライト"
		body = summaryResult(c) + fmt.Sprintf("\n\n全%d手。実際の取り石は黒%d個、白%d個でした。", len(c.Board.Moves), c.Board.Captured[app.Black], c.Board.Captured[app.White])
		move = s.Highlight
		if move != nil {
			body += fmt.Sprintf("\n解析済みの範囲で目差の変動が最大だったのは%d手目です。着手した側の予測目差は%+.1f目、勝率は%+.1fポイント変わりました。", move.Turn, move.Swing, move.WinChange*100)
		} else {
			body += "\nハイライトを選べる前後のAI評価がまだ揃っていません。"
		}
	case 1:
		title = "CPUの戦略（推定）"
		body = "手数を三分割した区間です。実際の着手の傾向から振り返ります。\n\n"
		for i, name := range []string{"序盤", "中盤", "終盤"} {
			body += summaryStageText(s.Stages[i], name) + "\n\n"
		}
	case 2:
		title = "あなたの良かった場面"
		move = s.GoodMove
		if move == nil {
			body = "好手として紹介できるAI評価と盤面の根拠が揃っていません。これは、良い手がなかったという意味ではありません。"
		} else {
			body = fmt.Sprintf("%d手目の%s。推定損失は%.1f目でした。", move.Turn, adviceVertex(move.Point, c.Board.Size), move.Loss)
			if move.Captured+move.Saved+move.Connected > 0 {
				body += fmt.Sprintf("\n取り%d個・救出%d個・連結%d組の効果がありました。", move.Captured, move.Saved, move.Connected)
			}
			if move.BestCandidate {
				body += "\nAIの有力候補と同じ手を選べました。"
			}
			body += "\n\n次局も、打つ前に取れる石・助ける石・つながる石を確認してみましょう。"
		}
	case 3:
		title = "次の一局で試すこと"
		move = s.Improvement
		if move == nil {
			body = "具体的な改善場面を示すだけの解析と合法な比較候補が揃っていません。\n\n次局は打つ前に、自分の石が取られそうか、相手の石を取れるかを順に確認しましょう。"
		} else {
			body = fmt.Sprintf("%d手目の%sを振り返ります。推定損失は%.1f目でした。\n着手前の盤面で、比較候補の%sも確認してみましょう。", move.Turn, adviceVertex(move.Point, c.Board.Size), move.Loss, adviceVertex(move.Alternative, c.Board.Size))
			before, err := c.Snapshot(move.Turn - 1)
			after, afterErr := c.Snapshot(move.Turn)
			if err == nil && afterErr == nil {
				body += "\n\n" + reviewAdvice(c, move.Turn, before, after).summary
			}
			body += "\n\n比較候補の先の結果は確定していません。周囲の石の逃げ道を比べることから始めましょう。"
		}
	}
	return
}

// refreshSummaryPage はページ移動・再試行・解析完了時だけ文章と盤面を作り直す。
func (g *window) refreshSummaryPage(d *summaryDialog) {
	c := g.controller
	s := c.SummarySession
	if d.cachedPage == d.page && d.source == s && d.ready == s.Ready {
		return
	}
	d.cachedPage, d.source, d.ready = d.page, s, s.Ready
	d.title, d.body, d.move = summaryPageContent(c, d.page)
	d.lines = g.wrapLines(strings.TrimSpace(d.body), 510, 20)
	d.maxScroll = math.Max(0, float64(len(d.lines)*30-450))
	d.scrollSummary(0)
	d.board = nil
	if d.move != nil {
		turn := d.move.Turn
		stage := "着手後"
		if d.page == 3 {
			turn--
			stage = "着手前"
		}
		b, err := c.Snapshot(turn)
		if err == nil {
			d.board = b
			d.boardCaption = fmt.Sprintf("%d手目の%s", d.move.Turn, stage)
		} else {
			d.boardCaption = "対象局面を再現できません。"
		}
	}
}

// updateSummaryInput は概要の本文にだけホイール・移動キーを適用する。
func (g *window) updateSummaryInput(d *summaryDialog) {
	x, y := ebiten.CursorPosition()
	if image.Pt(x, y).In(image.Rect(65, 185, 610, 640)) {
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
	g.label(screen, status, 65, 125, 17, muted)
	if g.controller.Board.Rule == app.Jungo {
		g.label(screen, "途中のAI評価は近似。確定得点は盤上の石数です。", 65, 153, 14, warning)
	}
	if d.canvas == nil {
		d.canvas = ebiten.NewImage(530, 450)
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
	} else {
		g.label(screen, "対象盤面はありません", 720, 365, 20, muted)
	}
	g.button(screen, image.Rect(65, 695, 250, 737), "前のページ", d.page > 0, func() { d.changePage(-1) })
	g.button(screen, image.Rect(270, 695, 350, 737), "上へ", d.scroll > 0, func() { d.scrollSummary(-120) })
	g.button(screen, image.Rect(360, 695, 440, 737), "下へ", d.scroll < d.maxScroll, func() { d.scrollSummary(120) })
	if s.Ready && s.Err != nil {
		g.button(screen, image.Rect(460, 695, 645, 737), "解析を再試行", d.retry != nil, d.retry)
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
}

// drawSummaryBoard は選定した独立局面を縮小表示する。改善ページの比較候補は着手前の盤面へ重ねる。
func (g *window) drawSummaryBoard(screen *ebiten.Image, d *summaryDialog) {
	b := d.board
	r := image.Rect(655, 225, 1105, 610)
	left, top, step := overviewGeometry(r.Inset(14), b.Size)
	g.label(screen, d.boardCaption, 655, 190, 21, foreground)
	rect(screen, r, 0xdab47b)
	for i := 0; i < b.Size; i++ {
		x, y := left+float64(i)*step, top+float64(i)*step
		vector.StrokeLine(screen, float32(x), float32(top), float32(x), float32(top+float64(b.Size-1)*step), 1, tint(0x654d32), true)
		vector.StrokeLine(screen, float32(left), float32(y), float32(left+float64(b.Size-1)*step), float32(y), 1, tint(0x654d32), true)
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
	for i, point := range []int{d.move.Point, d.move.Alternative} {
		if point < 0 || i == 1 && (d.page != 3 || !d.move.HasAlternative) {
			continue
		}
		x, y := left+float64(point%b.Size)*step, top+float64(point/b.Size)*step
		color := uint32(0xd64032)
		if i == 1 {
			color = 0x125bb4
		}
		vector.StrokeCircle(screen, float32(x), float32(y), float32(step*.46), 3, tint(color), true)
	}
	caption := "赤い輪：実際の着手 " + adviceVertex(d.move.Point, b.Size)
	if d.page == 3 {
		caption += "\n青い輪：比較候補 " + adviceVertex(d.move.Alternative, b.Size)
	}
	g.label(screen, caption, 655, 622, 16, muted)
}
