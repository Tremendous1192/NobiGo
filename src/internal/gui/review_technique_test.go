// 作成日: 2026-10-25。教材8種類の表示・合法な手順再生・描画後保存・保存失敗の再試行を検証する。
package gui

import (
	"context"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Tremendous1192/NobiGo/engine"
	"github.com/Tremendous1192/NobiGo/internal/app"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestTechniqueDiagrams は全教材の手順を往復しても実棋譜・本文・履歴が変わらないことを検証する。
func TestTechniqueDiagrams(t *testing.T) {
	for _, id := range app.TechniqueIDs() {
		t.Run(string(id), func(t *testing.T) {
			g := summaryWindow(t, app.Japanese, app.Black)
			g.previewMode = true
			if err := g.prepareTechniquePreview(id); err != nil {
				t.Fatal(err)
			}
			d := g.summary
			c := g.controller
			original := slices.Clone(c.Board.Cells)
			g.refreshSummaryPage(d)
			if d.lesson == nil || d.lesson.ID != id || d.techniquePresented || !strings.Contains(d.body, "仮想手順") {
				t.Fatal("missing or prematurely presented lesson")
			}
			body := d.body
			screen := ebiten.NewImage(1180, 800)
			for step := 0; step <= len(d.lesson.Line); step++ {
				g.drawSummaryDialog(screen, d)
				if d.board == nil || len(d.board.Moves) != d.lesson.Turn-1+step || !d.techniquePresented || d.body != body {
					t.Fatal("wrong technique stage", step)
				}
				d.changeTechniqueStep(1)
			}
			d.changeTechniqueStep(-100)
			g.drawSummaryDialog(screen, d)
			g.updateTechniqueSave()
			if d.techniqueStep != 0 || !slices.Equal(original, c.Board.Cells) || len(c.Profile.Techniques) != 0 {
				t.Fatal("preview changed source")
			}
		})
	}
}

type techniqueTestAnalyzer struct{}

// Analyze は教材保存テストで実エンジンを使わず、取消まで待つ。
func (techniqueTestAnalyzer) Analyze(ctx context.Context, _ engine.Position) (engine.Analysis, error) {
	<-ctx.Done()
	return engine.Analysis{}, ctx.Err()
}

// TestTechniqueDisplayedSave は描画前不変・描画後保存・明示再試行・一覧同期・重複防止を検証する。
func TestTechniqueDisplayedSave(t *testing.T) {
	g := summaryWindow(t, app.Japanese, app.Black)
	if err := g.prepareTechniquePreview(app.TechniqueCapture); err != nil {
		t.Fatal(err)
	}
	fixture := g.controller
	s, err := app.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Create("Display fixture")
	if err != nil {
		t.Fatal(err)
	}
	c, err := app.NewController(context.Background(), techniqueTestAnalyzer{}, s, rand.New(rand.NewPCG(1, 2)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	if err = c.Start(p, app.Settings{Rule: app.Japanese, Size: 7}); err != nil {
		t.Fatal(err)
	}
	c.Abort()
	c.Board = fixture.Board
	c.SummarySession = fixture.SummarySession
	c.Phase = app.Summary
	g.controller = c
	g.store = s
	g.profiles = []app.Profile{c.Profile}
	revision := c.Profile.Revision
	g.refreshSummaryPage(g.summary)
	g.updateTechniqueSave()
	if c.Profile.Revision != revision {
		t.Fatal("saved before drawing")
	}
	lock := filepath.Join(s.Dir, "store.lock")
	if err = os.WriteFile(lock, nil, 0600); err != nil {
		t.Fatal(err)
	}
	g.drawSummaryDialog(ebiten.NewImage(1180, 800), g.summary)
	g.updateTechniqueSave()
	if !g.summary.saveFailed || c.Profile.Revision != revision {
		t.Fatal("failed save hidden")
	}
	if err = os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	g.updateTechniqueSave()
	if c.Profile.Revision != revision {
		t.Fatal("automatic retry")
	}
	g.summary.saveAttempted = false
	g.updateTechniqueSave()
	if g.summary.saveFailed || c.Profile.Revision != revision+1 || g.profiles[0].Revision != c.Profile.Revision || len(c.Profile.Techniques) != 1 {
		t.Fatal("retry or profile sync failed")
	}
	g.summary.saveAttempted = false
	g.updateTechniqueSave()
	if c.Profile.Revision != revision+1 || c.Profile.Techniques[0].Count != 1 {
		t.Fatal("duplicate history")
	}
}
