// 作成日: 2026-10-18。色分けの初回描画時刻と局面切替の計測を検証する。実時間の待機は使わない。
package gui

import (
	"strings"
	"testing"
	"time"

	"github.com/Tremendous1192/NobiGo/engine"
	"github.com/Tremendous1192/NobiGo/internal/app"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// TestHeaderPreviewScenarios はルール別棋力・全路数・黒白・長い名前・最小画面を保存や実解析なしで検証する。
func TestHeaderPreviewScenarios(t *testing.T) {
	for _, tc := range []struct {
		mode   string
		size   int
		rule   app.Rule
		player app.Color
		rank   string
	}{
		{"game-header-7", 7, app.Japanese, app.Black, "棋力30級"},
		{"game-header-9", 9, app.Japanese, app.Black, "棋力30級"},
		{"game-header-11", 11, app.Japanese, app.Black, "棋力30級"},
		{"game-header-13", 13, app.Japanese, app.Black, "棋力30級"},
		{"game-header-15", 15, app.Japanese, app.Black, "棋力30級"},
		{"game-header-19", 19, app.Japanese, app.Black, "棋力30級"},
		{"game-header-chinese", 13, app.Chinese, app.White, "棋力1段"},
		{"game-header-jungo", 7, app.Jungo, app.Black, "棋力9段"},
		{"game-header-long", 19, app.Japanese, app.Black, "棋力30級"},
		{"game-header-min", 19, app.Japanese, app.Black, "棋力30級"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			g := &window{previewMode: true}
			if err := g.preparePreview(tc.mode); err != nil {
				t.Fatal(err)
			}
			c := g.controller
			if g.store != nil || g.engine != nil || c.Phase != app.Playing || c.Board.Passes >= 2 {
				t.Fatal("preview used real resources or ended the game")
			}
			if c.Board.Size != tc.size || c.Board.Rule != tc.rule || c.Player != tc.player || !strings.Contains(gameHeaderText(c), tc.rank) {
				t.Fatal("wrong header fixture")
			}
			if g.previewMinimum != (tc.mode == "game-header-min") {
				t.Fatal("wrong window size")
			}
			if (tc.mode == "game-header-long" || tc.mode == "game-header-min") && len([]rune(c.Profile.Name)) != 32 {
				t.Fatal("long name does not cover the limit")
			}
		})
	}
}

// TestOwnershipTimingTracksCurrentBoard は最初の描画だけを計測し、旧局面の評価や再描画を数えないことを検証する。
func TestOwnershipTimingTracksCurrentBoard(t *testing.T) {
	b, err := app.NewBoard(9, app.Japanese)
	if err != nil {
		t.Fatal(err)
	}
	c := &app.Controller{Board: b, Evaluations: make([]*engine.Analysis, 1)}
	g := &window{controller: c, verification: &verification{displayedTurn: -1}}
	start := time.Unix(100, 0)
	g.recordVerificationDraw(start)
	if g.verification.ownershipShown {
		t.Fatal("missing analysis counted as displayed")
	}
	c.Evaluations[0] = &engine.Analysis{Ownership: make([]float64, 81), Partial: true}
	g.recordVerificationDraw(start.Add(350 * time.Millisecond))
	g.recordVerificationDraw(start.Add(time.Second))
	if !g.verification.ownershipShown || g.verification.ownershipMS != 350 {
		t.Fatal("first display time changed")
	}
	if err := b.Play(0); err != nil {
		t.Fatal(err)
	}
	c.Evaluations = append(c.Evaluations, nil)
	g.recordVerificationDraw(start.Add(2 * time.Second))
	if g.verification.ownershipShown || g.verification.ownershipMS != 0 {
		t.Fatal("old board ownership was reused")
	}
	c.Evaluations[1] = &engine.Analysis{Ownership: make([]float64, 80)}
	g.recordVerificationDraw(start.Add(2100 * time.Millisecond))
	if g.verification.ownershipShown {
		t.Fatal("invalid ownership length accepted")
	}
	c.Evaluations[1] = &engine.Analysis{Ownership: make([]float64, 81)}
	g.recordVerificationDraw(start.Add(2250 * time.Millisecond))
	if g.verification.ownershipMS != 250 {
		t.Fatal("new board timing includes old board")
	}
}

// TestSummaryAcceptanceCoverage は実測対象に全3ルール・7路と19路の概要と次局が含まれることを検証する。
func TestSummaryAcceptanceCoverage(t *testing.T) {
	v := newVerification("unused.json")
	if len(v.SummaryCases) != 6 || len(v.Cases) != 96 {
		t.Fatal("acceptance coverage changed")
	}
	for i, item := range v.SummaryCases {
		if item.Rule != []app.Rule{app.Japanese, app.Chinese, app.Jungo}[i/2] || item.Size != []int{7, 19}[i%2] {
			t.Fatal("wrong summary condition")
		}
	}
}

// TestDisplayPreviewBoundaries は二軸・純碁・欠損と部分図の全路数・前後・図外・パス・初期局面を検証する。
func TestDisplayPreviewBoundaries(t *testing.T) {
	for _, mode := range []string{"game-white-jungo", "game-extremes", "game-gap", "review-focus-moved", "review-focus-before", "review-detail-before", "review-detail-outside", "review-detail-pass", "review-detail-empty",
		"review-detail-7", "review-detail-9", "review-detail-11", "review-detail-13", "review-detail-15", "review-detail-19"} {
		t.Run(mode, func(t *testing.T) {
			g := &window{previewMode: true}
			if err := g.preparePreview(mode); err != nil {
				t.Fatal(err)
			}
			if g.store != nil || g.engine != nil || g.autoplay {
				t.Fatal("preview accessed real resources")
			}
			if strings.HasPrefix(mode, "review-focus") {
				if g.advice != nil || g.currentReviewFocus() == nil || g.reviewFocus.showBefore != (mode == "review-focus-before") {
					t.Fatal("closed detail lost focus or stage")
				}
				if mode == "review-focus-moved" && g.reviewFocus.center == g.reviewFocus.point {
					t.Fatal("moved focus was reset")
				}
				return
			}
			if strings.HasPrefix(mode, "review-detail") {
				if g.advice == nil || g.advice.diagram == nil {
					t.Fatal("missing diagram")
				}
				d := g.advice.diagram
				if mode == "review-detail-before" && (!d.showBefore || d.targets[d.target].label != "取られた石") {
					t.Fatal("wrong before target")
				}
				if mode == "review-detail-outside" {
					s, l := d.outsideCounts()
					if s != 14 || l != 14 {
						t.Fatalf("wrong outside example: %d/%d", s, l)
					}
				}
				if (mode == "review-detail-pass" || mode == "review-detail-empty") && len(d.targets) != 0 {
					t.Fatal("invented focus")
				}
				return
			}
			c := g.controller
			if c.Phase != app.Playing || c.Board.Passes >= 2 {
				t.Fatal("game fixture ended")
			}
			if mode == "game-white-jungo" && (c.Player != app.White || c.Board.Rule != app.Jungo) {
				t.Fatal("wrong perspective or rule")
			}
			if mode == "game-extremes" {
				if loss, ok := c.Loss(len(c.Board.Moves)); !ok || loss != 40000 {
					t.Fatal("extreme loss missing")
				}
			}
			if mode == "game-gap" && (c.Evaluations[7] == nil || c.Evaluations[8] != nil || c.Evaluations[12] != nil || c.Evaluations[13] == nil) {
				t.Fatal("gap fixture missing")
			}
		})
	}
}

// TestAIPreviewEvidence は合成例の接続評価・全路盤・エンジン分離を検証する。
func TestAIPreviewEvidence(t *testing.T) {
	for _, mode := range []string{"review-ai", "review-ai-19", "review-detail-ai", "review-detail-ai-risk"} {
		g := &window{previewMode: true}
		if err := g.preparePreview(mode); err != nil {
			t.Fatal(err)
		}
		d := g.currentReviewFocus()
		g.refreshTactics(&d.after, g.reviewTurn)
		if g.engine != nil || g.store != nil || d.after.read.variations == 0 {
			t.Fatal("preview has no isolated reading")
		}
		found := false
		want := readStrong
		if mode == "review-detail-ai-risk" {
			want = readRisk
		}
		for _, link := range d.after.read.links {
			if link.strength == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s has no expected connection", mode)
		}
	}
}

// TestAdviceClarityPreviews は平易な説明と図が同じ局面を使い、要約3行と詳細の全文が描画幅に収まることを検証する。
func TestAdviceClarityPreviews(t *testing.T) {
	for _, tc := range []struct {
		mode, want string
		rule       app.Rule
		turn       int
	}{
		{"review-detail-rescue", "1か所から3か所", app.Japanese, 5},
		{"review-detail-atari", "1か所に減り、アタリ", app.Japanese, 3},
		{"review-detail-jungo-empty", "空きだけでは得点になりません", app.Jungo, 0},
		{"review-detail-jungo-pass", "実際に取る必要", app.Jungo, 2},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			g := summaryWindow(t, app.Japanese, app.Black)
			g.previewMode = true
			if err := g.preparePreview(tc.mode); err != nil {
				t.Fatal(err)
			}
			g.syncPhase()
			if g.advice == nil || g.store != nil || g.engine != nil || g.autoplay || g.reviewTurn != tc.turn || g.reviewBoard.Rule != tc.rule {
				t.Fatal("clarity preview has wrong position or accessed real resources")
			}
			if !strings.Contains(g.advice.body, tc.want) {
				t.Fatalf("missing lesson: %s", g.advice.body)
			}
			a := reviewAdvice(g.controller, tc.turn, g.reviewBefore, g.reviewBoard)
			if g.advice.body != a.detail || len(g.wrapLines(a.summary, 445, 16)) > 3 {
				t.Fatal("preview differs from actual advice or summary exceeds three lines")
			}
			g.drawAdviceDialog(ebiten.NewImage(1180, 800))
			if strings.Join(g.advice.lines, "") != strings.ReplaceAll(a.detail, "\n", "") {
				t.Fatal("detail lost text during wrapping")
			}
			g.face.Size = adviceFontSize
			for _, line := range g.advice.lines {
				if width, _ := text.Measure(line, g.face, adviceLineHeight); width > adviceWidth-16 {
					t.Fatal("detail exceeded viewport")
				}
			}
		})
	}
}

// TestAdvicePreviewScenarios は表示用局面の手番・複合効果・未解析・長文・最小サイズが再現できることを検証する。
func TestAdvicePreviewScenarios(t *testing.T) {
	for _, mode := range []string{"review-detail", "review-detail-cpu", "review-detail-combined", "review-detail-wait", "review-detail-error", "review-detail-long", "review-detail-bottom", "review-detail-min"} {
		t.Run(mode, func(t *testing.T) {
			g := &window{previewMode: true}
			if err := g.preparePreview(mode); err != nil {
				t.Fatal(err)
			}
			if g.advice == nil || g.store != nil || g.autoplay || g.controller.Phase != app.Review {
				t.Fatal("preview lost isolation or modal")
			}
			g.syncPhase()
			if g.advice == nil {
				t.Fatal("first draw would dismiss preview")
			}
			body := g.advice.body
			if (mode == "review-detail-cpu") != strings.Contains(body, "CPUの狙い（推定）") {
				t.Fatal("wrong preview mover")
			}
			if mode == "review-detail-combined" && (!strings.Contains(body, "石を1個") || !strings.Contains(body, "アタリ")) {
				t.Fatal("missing combined effects")
			}
			if mode == "review-detail-wait" || mode == "review-detail-error" {
				if g.advice.status == "" || strings.Contains(body, "比較候補") {
					t.Fatal("missing analysis shown as available")
				}
			}
			if mode == "review-detail-long" || mode == "review-detail-bottom" {
				if !strings.HasSuffix(body, "末尾まで確認できました。") {
					t.Fatal("long text has no end marker")
				}
			}
			if g.previewMinimum != (mode == "review-detail-min") {
				t.Fatal("wrong preview size")
			}
		})
	}
	if err := (&window{}).preparePreview("review-detail-invalid"); err == nil {
		t.Fatal("unknown advice preview accepted")
	}
}
