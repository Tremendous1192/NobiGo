// 作成日: 2026-10-18。盤面・推定領域・評価グラフと対局操作を描く。解析値と確定得点を区別する。
package gui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"

	"github.com/Tremendous1192/NobiGo/engine"
	"github.com/Tremendous1192/NobiGo/internal/app"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
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

// gameHeaderText は対局中のルール・盤サイズ・プレイヤー棋力・CPU難易度を表示順に返す。
func gameHeaderText(c *app.Controller) string {
	b := c.Board
	rank := app.RankName(c.Profile.Ranks[app.RuleIndex(b.Rule)])
	return fmt.Sprintf("%s  ·  %d路  ·  棋力%s  ·  %s", ruleName(b.Rule), b.Size, rank, app.DifficultyName(c.Settings.Difficulty))
}

// gamePlayerText は長い名前だけを省略し、プレイヤーの石色を必ず残す。
func (g *window) gamePlayerText() string {
	c := g.controller
	suffix := "（あなた：" + colorName(c.Player) + "）"
	g.face.Size = 21
	width, _ := text.Measure(suffix, g.face, 31.5)
	return g.fit(c.Profile.Name, 470-width, 21) + suffix
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
	if c.Phase == app.Scoring {
		g.drawScoring(screen)
		return
	}
	if c.Phase == app.Finished || c.Phase == app.Summary {
		g.drawResult(screen)
		return
	}
	if c.Phase == app.Review {
		g.drawReview(screen)
		return
	}
	b := c.Board
	turn := len(b.Moves)
	g.label(screen, "NobiGo", 35, 22, 32, foreground)
	g.label(screen, gameHeaderText(c), 230, 34, 20, muted)
	g.label(screen, g.gamePlayerText(), 670, 33, 21, foreground)
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
	statusColor := accent
	if cpuPassed(b, c.Player) && c.Err == nil {
		status = "CPU（" + colorName(c.Player.Other()) + "）がパスしました。あなたの番です。"
		statusColor = warning
	}
	g.label(screen, status, 38, 80, 20, statusColor)
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
			// RGBAへ未乗算のRGBを渡すと加算色になるため、透明度を持つ領域はNRGBAで合成する。
			c := color.NRGBA{R: 53, G: 125, B: 227, A: uint8(140 * math.Abs(v))}
			if v < 0 {
				c = color.NRGBA{R: 228, G: 78, B: 84, A: uint8(140 * math.Abs(v))}
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
	var numbers []int
	if g.controller.Phase == app.Review {
		numbers = stoneMoveNumbers(b)
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
			markRadius := r * .28
			if numbers != nil {
				markRadius = r * .91
			}
			vector.StrokeCircle(screen, x, y, markRadius, 2, tint(0xe9ad41), true)
		}
		if numbers != nil && numbers[p] > 0 {
			g.drawStoneNumber(screen, numbers[p], float64(x), float64(y), float64(r), c)
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

// stoneMoveNumbers は再生中の棋譜から現在盤上に残る石の着手番号を求め、保存形式を増やさない。
func stoneMoveNumbers(b *app.Board) []int {
	numbers := make([]int, len(b.Cells))
	for i, m := range b.Moves {
		if m.Point >= 0 && m.Point < len(b.Cells) && b.Cells[m.Point] == m.Color {
			numbers[m.Point] = i + 1
		}
	}
	return numbers
}

// drawStoneNumber は石の直径に合わせて数字を中央へ描き、4桁の番号も石の内側へ収める。
func (g *window) drawStoneNumber(screen *ebiten.Image, number int, x, y, radius float64, stone app.Color) {
	value := strconv.Itoa(number)
	size := math.Min(22, radius*1.1)
	g.face.Size = size
	width, _ := text.Measure(value, g.face, size*1.5)
	if width > radius*1.65 {
		size *= radius * 1.65 / width
		g.face.Size = size
		width, _ = text.Measure(value, g.face, size*1.5)
	}
	c := foreground
	if stone == app.White {
		c = background
	}
	g.label(screen, value, x-width/2, y-size*.72, size, c)
}

// drawSidebar は解析済みの値と履歴を表示する。欠損区間を補間して確定値に見せない。
func (g *window) drawSidebar(screen *ebiten.Image, b *app.Board, turn int) {
	c := g.controller
	rect(screen, image.Rect(660, 120, 1145, 720), panel)
	heading := fmt.Sprintf("%d手目  ·  %s番", turn, colorName(b.ToMove))
	headingColor := foreground
	if cpuPassed(b, c.Player) {
		heading = fmt.Sprintf("%d手目  ·  CPUがパス", turn)
		headingColor = warning
	}
	g.label(screen, heading, 680, 132, 24, headingColor)
	a := g.currentAnalysis(turn)
	legend, legendColor := "青：黒の推定領域  /  赤：白の推定領域", muted
	if a == nil {
		legend, legendColor = "解析待ち  ·  青：黒 / 赤：白", warning
	}
	if c.Phase == app.Review && turn == len(c.Board.Moves) && a == nil {
		legend, legendColor = "確定採点の領域  ·  青：黒 / 赤：白", muted
	}
	g.label(screen, legend, 680, 172, 16, legendColor)
	g.plot(screen, image.Rect(728, 230, 1090, 315), turn)
	value, note := evaluationText(c, turn, b.Rule)
	size, valueWidth := g.evaluationRowSize(value, note)
	g.label(screen, value, 680, 354, size, foreground)
	if note != "" {
		g.label(screen, note, 680+valueWidth+8, 359, 12, warning)
	}
	caption := "あなた側の予測値 · 損失は直前に打った側"
	if a != nil && a.Partial {
		caption += " · 途中結果"
	}
	g.label(screen, caption, 680, 392, 14, muted)
	tabs := []string{"情報", "コメント"}
	if c.Phase == app.Review {
		tabs = append(tabs, "アドバイス")
	}
	width := 450 / len(tabs)
	for i, title := range tabs {
		x := 680 + i*width
		g.tabButton(screen, image.Rect(x, 430, x+width-10, 470), title, g.tab == i, func() { g.tab = i })
	}
	body := fmt.Sprintf("%s / コミ %.1f\nCPU強度はこの対局中、固定です。\n棋力 %s · 専用GPU不要", ruleName(b.Rule), app.Komi(b.Rule), app.RankName(c.Profile.Ranks[app.RuleIndex(b.Rule)]))
	if c.LastCPUTime > 0 {
		body += fmt.Sprintf("\nCPU応答 %.2f秒（描画まで）", c.LastCPUTime.Seconds())
	}
	if g.tab == 1 {
		body = positionComment(b)
	}
	showAdvice := g.tab == 2 && c.Phase == app.Review
	if showAdvice {
		body = reviewAdvice(c, turn, g.reviewBefore, b).summary
	}
	if c.Err != nil && !showAdvice {
		body = "解析に失敗しました。\n「解析を再試行」でエンジンを再接続できます。"
	}
	if g.notice != "" {
		body = g.notice
	}
	if c.CPUDeadlineMiss && !showAdvice {
		body = "このPCで7.5秒を超える応答がありました。\n性能条件は未達です。再試行か中止を選べます。"
	}
	if showAdvice {
		g.label(screen, g.wrap(body, 445, 16, 3), 680, 488, 16, foreground)
		g.button(screen, image.Rect(965, 674, 1125, 710), "詳しく", true, g.openReviewAdvice)
		if c.Err != nil {
			g.label(screen, "解析失敗：盤面の事実を表示", 680, 684, 15, warning)
		} else if c.Busy {
			g.label(screen, "解析中：盤面の事実を表示", 680, 684, 15, muted)
		}
	} else {
		g.label(screen, g.wrap(body, 445, 16, 5), 680, 488, 16, muted)
	}
}

// evaluationText は欠損をゼロにせず、視点とルールに対応する1行の数値を返す。
func evaluationText(c *app.Controller, turn int, rule app.Rule) (value, note string) {
	score, win, loss := "—", "—", "—"
	if a := c.Evaluations; turn >= 0 && turn < len(a) && a[turn] != nil {
		s, w := a[turn].ScoreLead, a[turn].Winrate
		if c.Player == app.White {
			s, w = -s, 1-w
		}
		score, win = fmt.Sprintf("%+.1f", s), fmt.Sprintf("%.1f%%", w*100)
	}
	if n, ok := c.Loss(turn); ok {
		loss = fmt.Sprintf("%.1f目", n)
	}
	if rule == app.Jungo {
		note = "（近似）"
	}
	return "目差" + score + "  勝率" + win + "  直前損失" + loss, note
}

// evaluationRowSize は数値の精度を保ち、注記を含めて1行445pxに収める。
func (g *window) evaluationRowSize(value, note string) (size, width float64) {
	available := 445.0
	if note != "" {
		g.face.Size = 12
		noteWidth, _ := text.Measure(note, g.face, 18)
		available -= noteWidth + 8
	}
	g.face.Size = 18
	width, _ = text.Measure(value, g.face, 27)
	size = 18
	if width > available {
		size *= available / width
		g.face.Size = size
		width, _ = text.Measure(value, g.face, size*1.5)
	}
	return
}

type plotPoint struct {
	x, y  float64
	valid bool
}

type plotSegment struct{ from, to plotPoint }

// plotSeries はプレイヤー視点の座標列を返す。欠損位置を残し、左右軸の尺度を混ぜない。
func plotSeries(values []*engine.Analysis, player app.Color, r image.Rectangle, winrate bool) ([]plotPoint, float64) {
	bound := 10.0
	for _, a := range values {
		if a != nil && !math.IsNaN(a.ScoreLead) && !math.IsInf(a.ScoreLead, 0) {
			bound = math.Max(bound, math.Ceil(math.Abs(a.ScoreLead)))
		}
	}
	points := make([]plotPoint, len(values))
	for i, a := range values {
		if a == nil {
			continue
		}
		v := .5 + a.ScoreLead/(2*bound)
		if winrate {
			v = a.Winrate
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		if player == app.White {
			v = 1 - v
		}
		v = math.Max(0, math.Min(1, v))
		points[i] = plotPoint{
			x: float64(r.Min.X) + float64(i*r.Dx())/float64(max(1, len(values)-1)),
			y: float64(r.Max.Y) - v*float64(r.Dy()), valid: true,
		}
	}
	return points, bound
}

// plotSegments は欠損で線を切り、破線の周期を着手境界をまたいで維持する。
func plotSegments(points []plotPoint, dashed bool) []plotSegment {
	var segments []plotSegment
	phase := 0.0
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		if !a.valid || !b.valid {
			phase = 0
			continue
		}
		if !dashed {
			segments = append(segments, plotSegment{a, b})
			continue
		}
		length := math.Hypot(b.x-a.x, b.y-a.y)
		// 各着手で破線を再開すると、短い区間が続く長い棋譜では実線に見える。
		for offset := 0.0; offset < length; {
			on := phase < 6
			remaining := 10 - phase
			if on {
				remaining = 6 - phase
			}
			step := math.Min(remaining, length-offset)
			if on {
				from := plotPoint{x: a.x + (b.x-a.x)*offset/length, y: a.y + (b.y-a.y)*offset/length, valid: true}
				to := plotPoint{x: a.x + (b.x-a.x)*(offset+step)/length, y: a.y + (b.y-a.y)*(offset+step)/length, valid: true}
				segments = append(segments, plotSegment{from, to})
			}
			offset += step
			phase = math.Mod(phase+step, 10)
		}
	}
	return segments
}

// plot は目差を左軸の実線、勝率を右軸の破線として同じ85pxの領域に描く。
func (g *window) plot(screen *ebiten.Image, r image.Rectangle, turn int) {
	c := g.controller
	score, bound := plotSeries(c.Evaluations, c.Player, r, false)
	win, _ := plotSeries(c.Evaluations, c.Player, r, true)
	const winColor uint32 = 0xc4a7ff
	g.label(screen, "目差 ━", 680, float64(r.Min.Y-28), 16, accent)
	g.label(screen, "勝率 ┄", 800, float64(r.Min.Y-28), 16, winColor)
	vector.StrokeRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 1, tint(surface), false)
	middle := float64(r.Min.Y) + float64(r.Dy())/2
	vector.StrokeLine(screen, float32(r.Min.X), float32(middle), float32(r.Max.X), float32(middle), 1, tint(surface), false)
	for _, series := range []struct {
		points []plotPoint
		dashed bool
		color  uint32
	}{{score, false, accent}, {win, true, winColor}} {
		for _, segment := range plotSegments(series.points, series.dashed) {
			vector.StrokeLine(screen, float32(segment.from.x), float32(segment.from.y), float32(segment.to.x), float32(segment.to.y), 2, tint(series.color), true)
		}
		for i, p := range series.points {
			if p.valid && (i == 0 || !series.points[i-1].valid) && (i == len(series.points)-1 || !series.points[i+1].valid) {
				vector.DrawFilledCircle(screen, float32(p.x), float32(p.y), 2, tint(series.color), true)
			}
		}
	}
	count := max(1, len(c.Evaluations)-1)
	x := float32(r.Min.X) + float32(max(0, min(turn, count)))*float32(r.Dx())/float32(count)
	vector.StrokeLine(screen, x, float32(r.Min.Y), x, float32(r.Max.Y), 1, tint(warning), false)
	for i, value := range []string{fmt.Sprintf("+%.0f", bound), "0", fmt.Sprintf("−%.0f", bound)} {
		y := float64(r.Min.Y) + float64(i*r.Dy())/2 - 9
		g.label(screen, value, float64(r.Min.X-48), y, 12, accent)
		g.label(screen, []string{"100%", "50%", "0%"}[i], float64(r.Max.X+8), y, 12, winColor)
	}
	g.label(screen, "0", float64(r.Min.X), float64(r.Max.Y+4), 12, muted)
	g.label(screen, fmt.Sprintf("%d手", max(0, len(c.Evaluations)-1)), float64(r.Max.X-45), float64(r.Max.Y+4), 12, muted)
}

// cpuPassed は現在の棋譜の最終手だけを参照し、待ったや次局へ通知を持ち越さない。
func cpuPassed(b *app.Board, player app.Color) bool {
	if b == nil || len(b.Moves) == 0 {
		return false
	}
	last := b.Moves[len(b.Moves)-1]
	return last.Point == -1 && last.Color == player.Other()
}

// positionComment は盤面から検証できる短い説明だけを返す。
func positionComment(b *app.Board) string {
	if len(b.Moves) == 0 {
		return "最初は隅や辺を試してみましょう。\n石の上下左右の空点が「呼吸点」です。"
	}
	last := b.Moves[len(b.Moves)-1]
	if last.Point < 0 {
		message := "パスしました。双方が続けてパスすると終局です。"
		if b.Rule == app.Jungo {
			return message + "\n純碁では、死石も実際に取ってから終局します。"
		}
		return message + "\n取れる石と、守る必要がある石を確認しましょう。"
	}
	_, libs := b.Group(last.Point)
	if len(libs) == 1 {
		return "直前の石の連は、呼吸点が残り1つです。\n「アタリ」です。逃げる手と取る手を確認しましょう。"
	}
	return fmt.Sprintf("直前の石の連には呼吸点が%dあります。\nつながりと、相手の呼吸点にも注目しましょう。", len(libs))
}
