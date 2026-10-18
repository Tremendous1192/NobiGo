// 作成日: 2026-10-18。色分けの初回描画時刻と局面切替の計測を検証する。実時間の待機は使わない。
package gui

import (
	"strings"
	"testing"
	"time"

	"github.com/Tremendous1192/PlayGo/engine"
	"github.com/Tremendous1192/PlayGo/internal/app"
)

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
