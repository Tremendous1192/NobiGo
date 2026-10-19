// 作成日: 2026-10-18。死石・セキを確認してから得点を確定する。純碁では石を除去しない。
package gui

import (
	"fmt"
	"image"
	"math"
	"time"

	"github.com/Tremendous1192/PlayGo/engine"
	"github.com/Tremendous1192/PlayGo/internal/app"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// prepareScore は同一盤面の直近解析から死石候補を用意する。提案は手動で解除できる。
func (g *window) prepareScore() {
	c := g.controller
	b := c.Board
	var ownership []float64
	for i := len(b.Moves); i >= 0; i-- {
		if a := g.currentAnalysis(i); a != nil {
			ownership = a.Ownership
			break
		}
		if i == 0 || b.Moves[i-1].Point != -1 {
			break
		}
	}
	g.dead = app.SuggestDead(b, ownership)
	g.seki = make(map[int]bool)
	g.markSeki = false
	g.notice = ""
	g.scoreError = ""
	g.updateScore()
}

// updateScore は修正中の印からプレビューを計算する。
func (g *window) updateScore() {
	s, err := app.ScoreBoard(g.controller.Board, g.dead, g.seki)
	if err != nil {
		g.scoreError = "死石とセキの指定が矛盾しています。印を解除してください。"
		return
	}
	g.scoreError = ""
	g.scorePreview = s
}

// toggleMark は連全体の死石・セキ指定を切り替える。
func (g *window) toggleMark(point int) {
	b := g.controller.Board
	if point < 0 || point >= len(b.Cells) || b.Cells[point] == app.Empty || b.Rule == app.Jungo {
		return
	}
	stones, _ := b.Group(point)
	if g.markSeki {
		if g.scorePreview.DeadMask[point] {
			return
		}
		value := !g.scorePreview.SekiMask[point]
		for _, p := range stones {
			delete(g.seki, p)
		}
		g.seki[stones[0]] = value
	} else {
		value := !g.scorePreview.DeadMask[point]
		for _, p := range stones {
			delete(g.dead, p)
			delete(g.seki, p)
		}
		if value {
			g.dead[stones[0]] = true
		}
	}
	g.updateScore()
}

// scoreOwnership は確定候補の領域を描画用の黒視点に変換する。
func scoreOwnership(s app.Score) *engine.Analysis {
	a := &engine.Analysis{Ownership: make([]float64, len(s.Owner))}
	for p, c := range s.Owner {
		if c == app.Black {
			a.Ownership[p] = 1
		} else if c == app.White {
			a.Ownership[p] = -1
		}
	}
	return a
}

// drawScoring は印の意味と点数内訳を示し、明示確認で保存する。
func (g *window) drawScoring(screen *ebiten.Image) {
	c := g.controller
	b := c.Board
	s := g.scorePreview
	g.label(screen, "採点を確認", 35, 25, 34, foreground)
	caption := "死石候補に赤い×を表示しています。石の連をクリックして修正できます。"
	if b.Rule == app.Jungo {
		caption = "純碁：盤上の石をそのまま数えます。死石の自動除去は行いません。"
	}
	if b.NoResult {
		caption = "同一局面の反復による無勝負です。棋力調整には使用しません。"
	}
	captionColor := muted
	if cpuPassed(b, c.Player) {
		caption = "CPUがパスし、連続パスで終局しました。点数と盤上の印を確認してください。"
		captionColor = warning
	}
	g.label(screen, caption, 38, 82, 19, captionColor)
	g.drawBoard(screen, b, scoreOwnership(s), s.DeadMask, s.SekiMask, false)
	if b.Rule != app.Jungo && !b.NoResult {
		g.actions = append(g.actions, hit{image.Rect(42, 127, 629, 714), func() { x, y := ebiten.CursorPosition(); g.toggleMark(boardPoint(x, y, b.Size)) }})
	}
	rect(screen, image.Rect(660, 120, 1145, 720), panel)
	g.label(screen, ruleName(b.Rule), 685, 145, 25, foreground)
	g.label(screen, fmt.Sprintf("黒  %.1f   /   白  %.1f", s.Black, s.White), 685, 197, 29, accent)
	rows := fmt.Sprintf("　　　　　 黒　　 白\n盤上の石　 %d　　 %d\n地　　　　 %d　　 %d\n取り石　　 %d　　 %d\n死石　　　 %d　　 %d\nコミ　　　 0　　 %.1f", s.Stones[app.Black], s.Stones[app.White], s.Territory[app.Black], s.Territory[app.White], s.Captures[app.Black], s.Captures[app.White], s.Dead[app.Black], s.Dead[app.White], s.Komi)
	g.label(screen, rows, 685, 265, 20, foreground)
	explanation := "日本：地 ＋ 取り石 ＋ コミ\nセキの眼は地に含めません。"
	if b.Rule == app.Chinese {
		explanation = "中国：盤上の石 ＋ 地 ＋ コミ\n取り石は点数に加算しません。"
	}
	if b.Rule == app.Jungo {
		explanation = "純碁：盤上の石数だけで勝敗を決めます。\n同数なら引き分けです。"
	}
	g.label(screen, explanation, 685, 502, 18, muted)
	if b.Rule != app.Jungo && !b.NoResult {
		mode := "死石を修正中"
		if g.markSeki {
			mode = "セキを修正中"
		}
		g.button(screen, image.Rect(685, 580, 1120, 624), mode+"（切替）", b.Rule == app.Japanese, func() { g.markSeki = !g.markSeki })
		g.label(screen, "赤×：死石　緑の輪：セキ候補\n候補は確定判定ではありません。", 685, 640, 17, warning)
	}
	g.button(screen, image.Rect(35, 741, 290, 788), "待ったで対局へ戻る", true, func() { c.Undo() })
	g.button(screen, image.Rect(805, 741, 1145, 788), "この点数で確定する", g.scoreError == "", func() {
		if err := c.Complete(g.dead, g.seki); err != nil {
			g.scoreError = "保存できません。他のPlayGoを閉じて再試行してください。"
			return
		}
		g.updateSelectedProfile()
	})
	if g.scoreError != "" {
		g.label(screen, g.fit(g.scoreError, 760, 17), 330, 754, 17, warning)
		g.button(screen, image.Rect(805, 687, 1120, 729), "保存を再試行", true, func() { g.scoreError = "" })
	}
}

// updateSelectedProfile は保存成功後のRevisionを起動画面へ反映する。
func (g *window) updateSelectedProfile() {
	for i, p := range g.profiles {
		if p.ID == g.controller.Profile.ID {
			g.profiles[i] = g.controller.Profile
			g.selected = i
			break
		}
	}
}

// nextGame は完了したプロフィールで次局を始める。
func (g *window) nextGame() error {
	c := g.controller
	if err := c.Start(c.Profile, c.Settings); err != nil {
		g.notice = "次の対局を開始できません。保存先と他の起動中アプリを確認してください。"
		return err
	}
	g.updateSelectedProfile()
	g.notice = ""
	return nil
}

// advanceResult は計数・勝利演出が終わった後に一度だけ次の行動を提示する。
func (g *window) advanceResult(now time.Time) {
	if g.controller.Phase == app.Finished && g.resultPromptPending && g.dialog == nil && now.Sub(g.animationStart) >= 900*time.Millisecond {
		g.openResultChoices()
	}
}

// openResultChoices は保存済みの結果から次の行動だけを選び、再保存しない。
func (g *window) openResultChoices() {
	if g.controller == nil || g.controller.Phase != app.Finished {
		return
	}
	g.resultPromptPending = false
	g.dialog = &dialog{
		title: "次にどうしますか？",
		body:  "一局おつかれさまでした。振り返りも、次の一局も、あなたのペースで。",
		choices: []dialogChoice{
			{title: "感想戦で振り返る", choose: func() error { g.autoplay = false; return g.setReview(0) }},
			{title: "感想戦（概要）", choose: g.openSummary},
			{title: "次のゲームを行う", choose: g.nextGame},
			{title: "最初の画面に戻る", choose: func() error { g.controller.Abort(); g.notice = ""; return nil }},
		},
	}
}

// openSummary は解析を準備して概要へ移り、通常感想戦の再生を停止する。
func (g *window) openSummary() error {
	if err := g.controller.StartSummary(); err != nil {
		return err
	}
	g.autoplay = false
	g.dialog, g.menu, g.advice = nil, nil, nil
	g.syncPhase()
	g.summary = newSummaryDialog(g.finishSummary, func() {
		if err := g.controller.StartSummary(); err != nil {
			g.notice = "概要の解析を開始できません。閉じて再度お試しください。"
		}
	})
	return nil
}

// finishSummary は補完解析を停止し、再保存・演出の再開始なしで行動選択へ戻す。
func (g *window) finishSummary() {
	g.autoplay = false
	g.summary = nil
	g.controller.EndSummary()
	g.syncPhase()
	g.openResultChoices()
}

// finishReview は補完解析と再生を停止して行動選択へ戻る。採点演出と結果保存は繰り返さない。
func (g *window) finishReview() {
	g.autoplay = false
	g.controller.EndReview()
	g.syncPhase()
	g.openResultChoices()
}

// drawResult は計数と勝利演出を描き、閉じた行動選択も再度開けるようにする。
func (g *window) drawResult(screen *ebiten.Image) {
	c := g.controller
	s := c.Score
	progress := math.Min(1, time.Since(g.animationStart).Seconds()/.9)
	g.label(screen, "対局結果", 45, 32, 35, foreground)
	g.drawBoard(screen, c.Board, scoreOwnership(s), s.DeadMask, s.SekiMask, false)
	rect(screen, image.Rect(660, 120, 1145, 720), panel)
	message := "この一局が、次の一歩になります。"
	if s.Winner == c.Player {
		message = "おめでとう！ あなたの勝ちです。"
	} else if s.Winner == app.Empty {
		message = "引き分けです。良い勝負でした。"
	}
	if s.NoResult {
		message = "無勝負です。棋力は変更しません。"
	}
	g.label(screen, fmt.Sprintf("黒 %.1f  /  白 %.1f", s.Black*progress, s.White*progress), 685, 153, 30, accent)
	g.label(screen, g.wrap(message, 430, 23, 2), 685, 218, 23, foreground)
	result := fmt.Sprintf("%sの %.1f目勝ち", colorName(s.Winner), s.Margin)
	if c.Board.Rule == app.Jungo {
		result = fmt.Sprintf("%sの %.0f石勝ち", colorName(s.Winner), s.Margin)
	}
	if s.Winner == app.Empty {
		result = "同点"
	}
	if s.NoResult {
		result = "反復無勝負"
	}
	g.label(screen, result, 685, 300, 22, muted)
	if s.Winner == c.Player && progress < 1 {
		for i := 0; i < 16; i++ {
			x := float32(690 + i*27)
			y := float32(350 + 80*math.Sin(float64(i)+progress*6))
			vector.DrawFilledCircle(screen, x, y, 4, tint(accent), true)
		}
	}
	g.label(screen, "次の一局も、あなたのペースで。", 685, 394, 22, foreground)
	g.button(screen, image.Rect(685, 447, 1120, 505), "次の行動を選ぶ", progress >= 1, g.openResultChoices)
	if g.notice != "" {
		g.label(screen, g.wrap(g.notice, 440, 16, 2), 685, 665, 16, warning)
	}
	g.label(screen, "確定した結果だけを、次局の難易度調整に使用します。", 38, 755, 19, muted)
}
