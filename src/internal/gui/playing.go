// 作成日: 2026-10-18。盤面・推定領域・評価グラフと対局操作を描く。解析値と確定得点を区別する。
package gui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"github.com/Tremendous1192/PlayGo/engine"
	"github.com/Tremendous1192/PlayGo/internal/app"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// ruleName はルールの表示名を返す。
func ruleName(r app.Rule) string {
	switch r {
	case app.Chinese:
		return "中国ルール"
	case app.Jungo:
		return "純碁"
	default:
		return "日本ルール"
	}
}

// colorName は手番の日本語名を返す。
func colorName(c app.Color) string {
	if c == app.Black {
		return "黒"
	}
	if c == app.White {
		return "白"
	}
	return "引き分け"
}

// currentAnalysis は指定手の解析だけを返す。前の局面の値を現在値として扱わない。
func (g *window) currentAnalysis(turn int) *engine.Analysis {
	if turn < 0 || turn >= len(g.controller.Evaluations) {
		return nil
	}
	return g.controller.Evaluations[turn]
}

// drawActive は対局画面の共通枠を描く。
func (g *window) drawActive(screen *ebiten.Image) {
	c := g.controller
	if g.shownPhase != c.Phase {
		if c.Phase == app.Scoring {
			g.prepareScore()
		}
		if c.Phase == app.Finished {
			g.animationStart = time.Now()
			g.notice = ""
		}
		g.shownPhase = c.Phase
	}
	if c.Phase == app.Scoring {
		g.drawScoring(screen)
		return
	}
	if c.Phase == app.Finished {
		g.drawResult(screen)
		return
	}
	if c.Phase == app.Review {
		g.drawReview(screen)
		return
	}
	b := c.Board
	turn := len(b.Moves)
	g.label(screen, "PlayGo", 35, 22, 32, foreground)
	g.label(screen, fmt.Sprintf("%s  ·  %d路  ·  %s", ruleName(b.Rule), b.Size, app.DifficultyName(c.Settings.Difficulty)), 230, 33, 21, muted)
	g.label(screen, g.fit(c.Profile.Name+"（あなた："+colorName(c.Player)+"）", 470, 21), 670, 33, 21, foreground)
	status := "あなたの番です。交点をクリックして打ちます。"
	if b.ToMove != c.Player {
		status = "CPUが考えています…"
	}
	if c.Phase == app.Scoring {
		status = "連続パスで終局しました。採点を確認します。"
	}
	if b.NoResult {
		status = "同じ局面が反復したため、無勝負です。"
	}
	if c.Err != nil {
		status = "解析できませんでした。再試行するか、中止してください。"
	}
	g.label(screen, status, 38, 80, 20, accent)
	a := g.currentAnalysis(turn)
	g.drawBoard(screen, b, a, nil, nil, c.Phase == app.Playing && b.ToMove == c.Player)
	g.drawSidebar(screen, b, turn)
	g.button(screen, image.Rect(35, 741, 215, 788), "パス", c.Phase == app.Playing && b.ToMove == c.Player, func() { g.play(-1) })
	g.button(screen, image.Rect(229, 741, 409, 788), "待った", c.Phase == app.Playing || c.Phase == app.Scoring, func() {
		if !c.Undo() {
			g.notice = "まだ取り消せる着手がありません。"
		} else {
			g.notice = "打ち直せます。"
		}
	})
	if c.Err != nil {
		g.button(screen, image.Rect(423, 741, 635, 788), "解析を再試行", true, c.Retry)
	}
	g.button(screen, image.Rect(880, 741, 1145, 788), "対局を中止", true, func() {
		g.dialog = &dialog{title: "対局を中止", body: "この対局の戦績・棋力調整は記録しません。", confirm: func() error { c.Abort(); return nil }}
	})
}

// play は着手失敗を盤上の案内へ変換する。
func (g *window) play(point int) {
	if err := g.controller.Play(point); err != nil {
		g.notice = "ここには置けません。空点・呼吸点・コウを確認してください。"
	} else {
		g.notice = ""
	}
}

// boardPoint は描画座標を最寄りの盤上点へ対応付ける。
func boardPoint(x, y, size int) int {
	step := 530.0 / float64(size-1)
	fx, fy := (float64(x)-70)/step, (float64(y)-155)/step
	xi, yi := int(math.Round(fx)), int(math.Round(fy))
	if xi < 0 || yi < 0 || xi >= size || yi >= size || math.Abs(fx-float64(xi)) > .48 || math.Abs(fy-float64(yi)) > .48 {
		return -2
	}
	return yi*size + xi
}

// drawBoard は石と領域を描く。dead・sekiは採点確認時だけ重ねる。
func (g *window) drawBoard(screen *ebiten.Image, b *app.Board, a *engine.Analysis, dead, seki []bool, interactive bool) {
	rect(screen, image.Rect(35, 120, 635, 720), 0xdab47b)
	step := 530.0 / float64(b.Size-1)
	if a != nil && len(a.Ownership) == len(b.Cells) {
		for p, v := range a.Ownership {
			if math.Abs(v) < .2 {
				continue
			}
			c := color.RGBA{R: 53, G: 125, B: 227, A: uint8(95 * math.Abs(v))}
			if v < 0 {
				c = color.RGBA{R: 228, G: 78, B: 84, A: uint8(95 * math.Abs(v))}
			}
			x, y := 70+float64(p%b.Size)*step, 155+float64(p/b.Size)*step
			vector.DrawFilledRect(screen, float32(x-step*.48), float32(y-step*.48), float32(step*.96), float32(step*.96), c, false)
		}
	}
	for i := 0; i < b.Size; i++ {
		v := float32(float64(i) * step)
		vector.StrokeLine(screen, 70+v, 155, 70+v, 685, 1, tint(0x654d32), true)
		vector.StrokeLine(screen, 70, 155+v, 600, 155+v, 1, tint(0x654d32), true)
		g.label(screen, fmt.Sprint(b.Size-i), 42, float64(146+v), 12, 0x654d32)
		g.label(screen, string("ABCDEFGHJKLMNOPQRST"[i]), float64(65+v), 693, 12, 0x654d32)
	}
	stars := []int{b.Size / 2}
	if b.Size == 9 {
		stars = []int{2, 4, 6}
	} else if b.Size >= 13 {
		stars = []int{3, b.Size / 2, b.Size - 4}
	}
	for _, x := range stars {
		for _, y := range stars {
			vector.DrawFilledCircle(screen, float32(70+float64(x)*step), float32(155+float64(y)*step), 3, tint(0x654d32), true)
		}
	}
	for p, c := range b.Cells {
		if c == app.Empty {
			continue
		}
		x, y := float32(70+float64(p%b.Size)*step), float32(155+float64(p/b.Size)*step)
		r := float32(math.Min(step*.45, 28))
		vector.DrawFilledCircle(screen, x+2, y+3, r, color.RGBA{A: 65}, true)
		stone, light := uint32(0x18222d), uint32(0x354353)
		if c == app.White {
			stone, light = 0xf1f2eb, 0xffffff
		}
		vector.DrawFilledCircle(screen, x, y, r, tint(stone), true)
		vector.DrawFilledCircle(screen, x-r*.2, y-r*.23, r*.55, tint(light), true)
		if p < len(dead) && dead[p] {
			vector.StrokeLine(screen, x-r*.6, y-r*.6, x+r*.6, y+r*.6, 3, tint(0xee5b65), true)
			vector.StrokeLine(screen, x-r*.6, y+r*.6, x+r*.6, y-r*.6, 3, tint(0xee5b65), true)
		}
		if p < len(seki) && seki[p] {
			vector.StrokeCircle(screen, x, y, r*.75, 2, tint(0x46c9b2), true)
		}
		if len(b.Moves) > 0 && b.Moves[len(b.Moves)-1].Point == p {
			vector.StrokeCircle(screen, x, y, r*.28, 2, tint(0xe9ad41), true)
		}
	}
	if interactive {
		g.actions = append(g.actions, hit{image.Rect(42, 127, 629, 714), func() {
			x, y := ebiten.CursorPosition()
			if p := boardPoint(x, y, b.Size); p >= 0 {
				g.play(p)
			}
		}})
		x, y := ebiten.CursorPosition()
		p := boardPoint(x, y, b.Size)
		if p >= 0 && b.Legal(p) {
			vector.StrokeCircle(screen, float32(70+float64(p%b.Size)*step), float32(155+float64(p/b.Size)*step), float32(math.Min(step*.42, 27)), 2, tint(0x177b70), true)
		}
	}
}

// drawSidebar は解析済みの値と履歴を表示する。欠損区間を補間して確定値に見せない。
func (g *window) drawSidebar(screen *ebiten.Image, b *app.Board, turn int) {
	c := g.controller
	rect(screen, image.Rect(660, 120, 1145, 720), panel)
	g.label(screen, fmt.Sprintf("%d手目  ·  %s番", turn, colorName(b.ToMove)), 680, 132, 24, foreground)
	g.label(screen, "青：黒の推定領域  /  赤：白の推定領域", 680, 172, 16, muted)
	g.plot(screen, image.Rect(700, 230, 1120, 315), turn, false)
	g.plot(screen, image.Rect(700, 355, 1120, 420), turn, true)
	a := g.currentAnalysis(turn)
	eval := "目差 —    勝率 —"
	if a != nil {
		score, win := a.ScoreLead, a.Winrate
		if c.Player == app.White {
			score = -score
			win = 1 - win
		}
		eval = fmt.Sprintf("目差 %+.1f    勝率 %.1f%%", score, win*100)
	}
	g.label(screen, eval, 680, 438, 24, foreground)
	lossText := "直前の損失：未解析"
	if loss, ok := c.Loss(turn); ok {
		lossText = fmt.Sprintf("直前の損失：%.1f目（推定）", loss)
	}
	g.label(screen, lossText, 680, 478, 18, muted)
	caption := "あなた側の予測値"
	if b.Rule == app.Jungo {
		caption = "純碁の途中評価は近似です"
	}
	if a != nil && a.Partial {
		caption += "・時間内の途中結果"
	}
	g.label(screen, caption, 680, 509, 15, warning)
	g.button(screen, image.Rect(680, 541, 890, 581), "情報", true, func() { g.tab = 0 })
	g.button(screen, image.Rect(904, 541, 1125, 581), "コメント", true, func() { g.tab = 1 })
	if c.Phase == app.Review {
		// 感想戦では3つの独立した選択肢を同じ列へ配置する。
		g.actions = g.actions[:len(g.actions)-2]
		rect(screen, image.Rect(680, 541, 1125, 581), panel)
		for i, title := range []string{"情報", "コメント", "アドバイス"} {
			x := 680 + i*150
			g.button(screen, image.Rect(x, 541, x+140, 581), title, true, func() { g.tab = i })
		}
	}
	body := fmt.Sprintf("%s / コミ %.1f\nCPU強度はこの対局中、固定です。\n棋力 %s · 専用GPU不要", ruleName(b.Rule), app.Komi(b.Rule), app.RankName(c.Profile.Ranks[app.RuleIndex(b.Rule)]))
	if c.LastCPUTime > 0 {
		body += fmt.Sprintf("\nCPU応答 %.2f秒（描画まで）", c.LastCPUTime.Seconds())
	}
	if g.tab == 1 {
		body = positionComment(b)
	}
	if g.tab == 2 && c.Phase == app.Review {
		body = reviewAdvice(c, turn)
	}
	if c.Err != nil {
		body = "解析に失敗しました。\n「解析を再試行」でエンジンを再接続できます。"
	}
	if g.notice != "" {
		body = g.notice
	}
	if c.CPUDeadlineMiss {
		body = "このPCで5秒を超える応答がありました。\n性能条件は未達です。再試行か中止を選べます。"
	}
	g.label(screen, g.wrap(body, 445, 17, 4), 680, 601, 17, muted)
}

// plot は人側視点の評価を描き、未解析の区間は線を切る。
func (g *window) plot(screen *ebiten.Image, r image.Rectangle, turn int, winrate bool) {
	c := g.controller
	title := "予測目差"
	bound := 10.0
	for _, a := range c.Evaluations {
		if a != nil {
			bound = math.Max(bound, math.Abs(a.ScoreLead))
		}
	}
	if winrate {
		title = "予測勝率"
	}
	g.label(screen, title, float64(r.Min.X-20), float64(r.Min.Y-28), 16, muted)
	vector.StrokeRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 1, tint(surface), false)
	vector.StrokeLine(screen, float32(r.Min.X), float32(r.Min.Y+r.Dy()/2), float32(r.Max.X), float32(r.Min.Y+r.Dy()/2), 1, tint(surface), false)
	count := max(1, len(c.Evaluations)-1)
	previous := false
	px, py := float32(0), float32(0)
	for i, a := range c.Evaluations {
		if a == nil {
			previous = false
			continue
		}
		v := .5 + a.ScoreLead/(2*bound)
		if winrate {
			v = a.Winrate
		}
		if c.Player == app.White {
			v = 1 - v
		}
		x := float32(r.Min.X) + float32(i)*float32(r.Dx())/float32(count)
		y := float32(r.Max.Y) - float32(v)*float32(r.Dy())
		if previous {
			vector.StrokeLine(screen, px, py, x, y, 2, tint(accent), true)
		}
		vector.DrawFilledCircle(screen, x, y, 2, tint(accent), true)
		px, py, previous = x, y, true
	}
	x := float32(r.Min.X) + float32(turn)*float32(r.Dx())/float32(count)
	vector.StrokeLine(screen, x, float32(r.Min.Y), x, float32(r.Max.Y), 1, tint(warning), false)
	top, bottom := fmt.Sprintf("+%.0f", bound), fmt.Sprintf("−%.0f", bound)
	if winrate {
		top, bottom = "100%", "0%"
	}
	g.label(screen, top, float64(r.Max.X-47), float64(r.Min.Y-27), 12, muted)
	g.label(screen, bottom, float64(r.Max.X-47), float64(r.Max.Y+2), 12, muted)
}

// positionComment は盤面から検証できる短い説明だけを返す。
func positionComment(b *app.Board) string {
	if len(b.Moves) == 0 {
		return "最初は隅や辺を試してみましょう。\n石の上下左右の空点が「呼吸点」です。"
	}
	last := b.Moves[len(b.Moves)-1]
	if last.Point < 0 {
		return "パスしました。双方が続けてパスすると終局です。\n純碁では、死石も実際に取ってから終局します。"
	}
	_, libs := b.Group(last.Point)
	if len(libs) == 1 {
		return "直前の石の連は、呼吸点が残り1つです。\n「アタリ」です。逃げる手と取る手を確認しましょう。"
	}
	return fmt.Sprintf("直前の石の連には呼吸点が%dあります。\nつながりと、相手の呼吸点にも注目しましょう。", len(libs))
}
