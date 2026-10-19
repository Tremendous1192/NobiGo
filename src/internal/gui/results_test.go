// 作成日: 2026-10-18。採点修正、終了後の選択、感想戦復帰、保存失敗からの再試行を検証する。
package gui

import (
	"context"
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tremendous1192/PlayGo/engine"
	"github.com/Tremendous1192/PlayGo/internal/app"
)

// TestScoreMarkTransitions は死石の再選択で解除され、セキとの矛盾が残らないことを検証する。
func TestScoreMarkTransitions(t *testing.T) {
	b, _ := app.NewBoard(7, app.Japanese)
	b.Cells[0] = app.Black
	b.Cells[1] = app.Black
	b.Cells[48] = app.White
	g := &window{controller: &app.Controller{Board: b}}
	g.prepareScore()
	g.toggleMark(0)
	if !g.scorePreview.DeadMask[0] || !g.scorePreview.DeadMask[1] {
		t.Fatal("group was not selected")
	}
	g.toggleMark(1)
	if g.scorePreview.DeadMask[0] || g.scorePreview.DeadMask[1] {
		t.Fatal("group did not clear")
	}
	g.markSeki = true
	g.toggleMark(0)
	g.markSeki = false
	g.toggleMark(1)
	if g.scoreError != "" || len(g.seki) != 0 {
		t.Fatal("stale seki mark", g.scoreError)
	}
}

type waitingAnalyzer struct{}

// Analyze は取消まで待つだけで外部プロセスを起動しない。Controller.Closeが完了を待つ。
func (waitingAnalyzer) Analyze(ctx context.Context, _ engine.Position) (engine.Analysis, error) {
	<-ctx.Done()
	return engine.Analysis{}, ctx.Err()
}

// resultWindow は一時プロフィールへ一度だけ保存した終局状態を用意する。
func resultWindow(t *testing.T) (*window, *app.Store) {
	t.Helper()
	s, err := app.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Create("result-test")
	if err != nil {
		t.Fatal(err)
	}
	c, err := app.NewController(context.Background(), waitingAnalyzer{}, s, rand.New(rand.NewPCG(1, 2)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	if err := c.Start(p, app.DefaultSettings()); err != nil {
		t.Fatal(err)
	}
	c.Abort()
	for i := 0; i < 2; i++ {
		if err := c.Board.Play(-1); err != nil {
			t.Fatal(err)
		}
	}
	c.Evaluations = []*engine.Analysis{{}, {}, {}}
	c.Phase = app.Scoring
	if err := c.Complete(nil, nil); err != nil {
		t.Fatal(err)
	}
	g := &window{controller: c, profiles: []app.Profile{c.Profile}, selected: 0, shownPhase: app.Scoring}
	g.syncPhase()
	return g, s
}

// TestResultChoicesAndReviewReturn は演出後の3択と感想戦復帰が保存・演出を重複しないことを検証する。
func TestResultChoicesAndReviewReturn(t *testing.T) {
	g, store := resultWindow(t)
	started, revision := g.animationStart, g.controller.Profile.Revision
	g.advanceResult(started.Add(899 * time.Millisecond))
	if g.dialog != nil {
		t.Fatal("choices interrupted animation")
	}
	g.advanceResult(started.Add(900 * time.Millisecond))
	if g.dialog == nil || len(g.dialog.choices) != 4 {
		t.Fatal("result choices missing")
	}
	g.chooseDialog(0)
	g.syncPhase()
	if g.controller.Phase != app.Review || g.dialog != nil || g.tab != 2 {
		t.Fatal("review entry failed")
	}
	g.autoplay = true
	g.finishReview()
	if g.autoplay || g.controller.Phase != app.Finished || g.dialog == nil || !g.animationStart.Equal(started) {
		t.Fatal("review exit restarted animation or playback")
	}
	g.dismiss()
	g.advanceResult(started.Add(10 * time.Second))
	if g.dialog != nil {
		t.Fatal("dismissed prompt reopened automatically")
	}
	g.openResultChoices()
	g.chooseDialog(3)
	p, err := store.Load(g.controller.Profile.ID)
	if err != nil || p.Revision != revision || p.GamesPlayed != 1 || g.controller.Phase != app.Idle {
		t.Fatal("navigation saved result again", err)
	}
}

// TestNextGameFailureKeepsChoices は保存失敗時に選択を保持し、復旧後に二重学習せず次局へ進むことを検証する。
func TestNextGameFailureKeepsChoices(t *testing.T) {
	g, store := resultWindow(t)
	g.openResultChoices()
	lock := filepath.Join(store.Dir, "store.lock")
	if err := os.WriteFile(lock, nil, 0600); err != nil {
		t.Fatal(err)
	}
	g.chooseDialog(2)
	if g.dialog == nil || g.dialog.problem == "" || g.controller.Phase != app.Finished {
		t.Fatal("failed next game discarded choices")
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	g.chooseDialog(2)
	g.syncPhase()
	if g.dialog != nil || g.controller.Phase != app.Playing || g.tab != 1 || g.controller.Profile.GamesPlayed != 1 {
		t.Fatal("next game failed or retrained")
	}
}

// TestSummaryChoicesNeverRetrain は概要の開閉・Esc・最終ページ・次局で二重保存や演出再開が起きないことを検証する。
func TestSummaryChoicesNeverRetrain(t *testing.T) {
	g, store := resultWindow(t)
	started, revision := g.animationStart, g.controller.Profile.Revision
	for i := 0; i < 3; i++ {
		g.openResultChoices()
		g.chooseDialog(1)
		if g.summary == nil || g.dialog != nil || g.controller.Phase != app.Summary || g.autoplay {
			t.Fatal("summary did not open")
		}
		if i == 1 {
			g.summary.changePage(3)
			g.summary.close()
		} else {
			g.dismiss()
		}
		if g.summary != nil || g.dialog == nil || g.controller.Phase != app.Finished || g.controller.Busy || !g.animationStart.Equal(started) {
			t.Fatal("summary exit did not cancel or restarted result")
		}
	}
	p, err := store.Load(g.controller.Profile.ID)
	if err != nil || p.Revision != revision || p.GamesPlayed != 1 {
		t.Fatal("summary changed profile", err)
	}
	g.chooseDialog(0)
	if g.controller.Phase != app.Review {
		t.Fatal("normal review inaccessible")
	}
	g.finishReview()
	g.chooseDialog(2)
	if g.controller.Phase != app.Playing || g.controller.SummarySession != nil || g.controller.Profile.GamesPlayed != 1 {
		t.Fatal("next game retained summary or retrained")
	}
}

// TestChoiceFailureCanRetry は失敗した操作のダイアログを閉じず、再試行を受け付けることを検証する。
func TestChoiceFailureCanRetry(t *testing.T) {
	fail := true
	g := &window{dialog: &dialog{choices: []dialogChoice{{title: "test", choose: func() error {
		if fail {
			return errors.New("test failure")
		}
		return nil
	}}}}}
	g.chooseDialog(0)
	if g.dialog == nil || g.dialog.problem == "" {
		t.Fatal("failed choice dismissed")
	}
	fail = false
	g.chooseDialog(0)
	if g.dialog != nil {
		t.Fatal("successful retry did not dismiss")
	}
}

// TestJungoCannotMarkDead は純碁の画面操作で石数が変わらないことを検証する。
func TestJungoCannotMarkDead(t *testing.T) {
	b, _ := app.NewBoard(7, app.Jungo)
	b.Cells[0] = app.Black
	g := &window{controller: &app.Controller{Board: b}}
	g.prepareScore()
	g.toggleMark(0)
	if len(g.dead) != 0 || g.scorePreview.Black != 1 {
		t.Fatal("jungo mark accepted")
	}
}
