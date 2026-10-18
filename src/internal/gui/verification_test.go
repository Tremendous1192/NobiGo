// 作成日: 2026-10-18。色分けの初回描画時刻と局面切替の計測を検証する。実時間の待機は使わない。
package gui

import (
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
