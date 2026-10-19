// 作成日: 2026-10-19。概要4ページの根拠・盤面・スクロール・背面操作の境界を検証する。
package gui

import (
	"bytes"
	"image"
	"strings"
	"testing"

	"github.com/Tremendous1192/NobiGo/engine"
	"github.com/Tremendous1192/NobiGo/internal/app"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// summaryWindow は白黒双方に優劣のある合成局面を表示用に準備する。
func summaryWindow(t *testing.T, rule app.Rule, player app.Color) *window {
	t.Helper()
	c, _, _ := adviceFixture(t, rule, []int{16, 0, 24, 1, 32, 2})
	c.Player = player
	for i, v := range []float64{0, 0, 0, -4, -2, -2, -1} {
		c.Evaluations[i] = &engine.Analysis{ScoreLead: v, Winrate: .5, Moves: []engine.Candidate{{Point: 40, Visits: 32}}}
	}
	c.Evaluations[0].Moves[0].Point = 16
	data, err := app.BuildGameSummary(c.Board, c.Player, c.Evaluations)
	if err != nil {
		t.Fatal(err)
	}
	c.SummarySession = &app.SummarySession{Data: data, Ready: true}
	c.Score = app.Score{Black: 10, White: 8, Margin: 2, Winner: app.Black}
	source, err := text.NewGoTextFaceSource(bytes.NewReader(fontData))
	if err != nil {
		t.Fatal(err)
	}
	return &window{controller: c, face: &text.GoTextFace{Source: source}}
}

// TestSummaryPagesUseEvidence は各ページが対応する手・確定得点・推定戦略を表示することを検証する。
func TestSummaryPagesUseEvidence(t *testing.T) {
	for _, rule := range []app.Rule{app.Japanese, app.Chinese, app.Jungo} {
		g := summaryWindow(t, rule, app.Black)
		for page := 0; page < 4; page++ {
			d := newSummaryDialog(nil, nil)
			d.page = page
			g.refreshSummaryPage(d)
			if d.title == "" || d.body == "" || len(d.lines) == 0 {
				t.Fatal("missing page")
			}
			if page == 1 {
				if !strings.Contains(d.body, "序盤") || !strings.Contains(d.title, "推定") {
					t.Fatal("missing stage evidence")
				}
				continue
			}
			if d.board == nil {
				t.Fatal("missing selected board")
			}
			want := d.move.Turn
			if page == 3 {
				want--
			}
			if len(d.board.Moves) != want {
				t.Fatal("wrong board stage")
			}
		}
		g.controller.Score.NoResult = true
		if !strings.Contains(summaryResult(g.controller), "無勝負") {
			t.Fatal("no result lost")
		}
	}
}

// TestSummaryImprovementUsesCurrentAdvice は概要4ページ目が通常感想戦と同じ手の平易な要約を使うことを検証する。
func TestSummaryImprovementUsesCurrentAdvice(t *testing.T) {
	for _, rule := range []app.Rule{app.Japanese, app.Chinese, app.Jungo} {
		for _, player := range []app.Color{app.Black, app.White} {
			g := summaryWindow(t, rule, player)
			c := g.controller
			_, body, move := summaryPageContent(c, 3)
			if move == nil {
				t.Fatal("fixture has no improvement")
			}
			before, err := c.Snapshot(move.Turn - 1)
			if err != nil {
				t.Fatal(err)
			}
			after, err := c.Snapshot(move.Turn)
			if err != nil {
				t.Fatal(err)
			}
			advice := reviewAdvice(c, move.Turn, before, after)
			if !strings.Contains(body, advice.summary) || !strings.Contains(advice.summary, "あなたの") || !strings.Contains(body, "結果は確定していません") {
				t.Fatalf("overview advice differs or lost uncertainty: %s", body)
			}
			d := newSummaryDialog(nil, nil)
			d.page = 3
			g.refreshSummaryPage(d)
			if d.board == nil || len(d.board.Moves) != move.Turn-1 || d.body != body {
				t.Fatal("overview paired advice with the wrong board")
			}
		}
	}
}

// TestSummaryUnknownIsNotBadPlay は根拠不足を悪手やゼロ損失と表現せず、純碁でも確定得点を使うことを検証する。
func TestSummaryUnknownIsNotBadPlay(t *testing.T) {
	g := summaryWindow(t, app.Jungo, app.White)
	g.controller.SummarySession.Data = app.GameSummary{}
	_, body, m := summaryPageContent(g.controller, 2)
	if m != nil || !strings.Contains(body, "良い手がなかったという意味ではありません") {
		t.Fatal("missing evidence was judged")
	}
	if result := summaryResult(g.controller); !strings.Contains(result, "CPU") || !strings.Contains(result, "石") || !strings.Contains(result, "10.0") {
		t.Fatal(result)
	}
}

// TestSummaryNavigationBlocksBackground はページ境界・最終終了・背面遮断・本文の端を検証する。
func TestSummaryNavigationBlocksBackground(t *testing.T) {
	g := summaryWindow(t, app.Japanese, app.Black)
	closed, background := false, false
	d := newSummaryDialog(func() { closed = true }, nil)
	d.changePage(-1)
	if d.page != 0 {
		t.Fatal("negative page")
	}
	g.actions = []hit{{image.Rect(0, 0, 1180, 800), func() { background = true }}}
	screen := ebiten.NewImage(1180, 800)
	g.drawSummaryDialog(screen, d)
	for _, action := range g.actions {
		if action.rect.Min.Y < 695 {
			t.Fatal("background action survived")
		}
	}
	d.maxScroll = 100
	d.scrollSummary(1000)
	if d.scroll != 100 {
		t.Fatal("scroll beyond end")
	}
	d.scrollSummary(-1000)
	if d.scroll != 0 {
		t.Fatal("scroll before start")
	}
	d.changePage(9)
	g.drawSummaryDialog(screen, d)
	for _, action := range g.actions {
		if image.Pt(1000, 715).In(action.rect) {
			action.action()
		}
	}
	if !closed || background || d.page != 3 {
		t.Fatal("summary exit failed")
	}
}

// TestSummaryRefreshAfterAnalysis は解析完了時だけ不足内容を更新し、同じページで盤面を再生成しないことを検証する。
func TestSummaryRefreshAfterAnalysis(t *testing.T) {
	g := summaryWindow(t, app.Japanese, app.Black)
	s := g.controller.SummarySession
	s.Ready = false
	d := newSummaryDialog(nil, nil)
	g.refreshSummaryPage(d)
	board := d.board
	g.refreshSummaryPage(d)
	if d.board != board {
		t.Fatal("board rebuilt every frame")
	}
	s.Ready = true
	g.refreshSummaryPage(d)
	if !d.ready || d.board == board {
		t.Fatal("analysis completion not reflected")
	}
}

// TestSummaryPreviewScenarios は全概要例が実データを使わず、初回描画で消えず、各路数の盤面を維持することを検証する。
func TestSummaryPreviewScenarios(t *testing.T) {
	for _, mode := range summaryPreviewModes() {
		t.Run(mode, func(t *testing.T) {
			g := summaryWindow(t, app.Japanese, app.Black)
			g.previewMode = true
			if err := g.preparePreview(mode); err != nil {
				t.Fatal(err)
			}
			g.syncPhase()
			if g.summary == nil || g.store != nil || g.engine != nil || g.autoplay {
				t.Fatal("preview lost isolation")
			}
			g.drawSummaryDialog(ebiten.NewImage(1180, 800), g.summary)
			if mode == "summary-4" && (g.summary.board == nil || len(g.summary.board.Moves) != 2) {
				t.Fatal("improvement preview has wrong position")
			}
			if mode == "summary-bottom" && g.summary.scroll != g.summary.maxScroll {
				t.Fatal("long text lost end")
			}
			if g.previewMinimum != (mode == "summary-min") {
				t.Fatal("minimum preview mismatch")
			}
		})
	}
	if err := (&window{}).preparePreview("summary-unknown"); err == nil {
		t.Fatal("unknown preview accepted")
	}
}

// TestSummaryBoardMargins は全路数の四隅で石と強調輪が木目領域からはみ出さないことを検証する。
func TestSummaryBoardMargins(t *testing.T) {
	r := image.Rect(655, 225, 1105, 610)
	for _, size := range []int{7, 9, 11, 13, 15, 19} {
		x, y, step := overviewGeometry(r.Inset(14), size)
		radius := step*.46 + 1.5
		if x-radius < float64(r.Min.X) || y-radius < float64(r.Min.Y) || x+float64(size-1)*step+radius > float64(r.Max.X) || y+float64(size-1)*step+radius > float64(r.Max.Y) {
			t.Fatal("board marker outside wooden area", size)
		}
	}
}
