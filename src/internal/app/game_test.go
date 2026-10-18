// 作成日: 2026-10-18。取消・世代・CPU先手・採点保存の境界を同期チャネルで検証する。
package app

import (
	"context"
	"errors"
	"math/rand/v2"
	"runtime"
	"testing"
	"time"

	"github.com/Tremendous1192/PlayGo/engine"
)

type fakeQuery struct {
	p         engine.Position
	reply     chan engine.Analysis
	cancelled chan struct{}
}
type gameFake struct{ queries chan fakeQuery }

// Analyze は応答か取消の一方で終了する試験用解析器。
func (f *gameFake) Analyze(ctx context.Context, p engine.Position) (engine.Analysis, error) {
	q := fakeQuery{p, make(chan engine.Analysis, 1), make(chan struct{})}
	select {
	case f.queries <- q:
	case <-ctx.Done():
		return engine.Analysis{}, ctx.Err()
	}
	select {
	case a := <-q.reply:
		return a, nil
	case <-ctx.Done():
		close(q.cancelled)
		return engine.Analysis{}, ctx.Err()
	}
}

// setupGame は試験専用ディレクトリと所有される作業を生成する。
func setupGame(t *testing.T, side int) (*Controller, *gameFake) {
	t.Helper()
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Create("試験")
	if err != nil {
		t.Fatal(err)
	}
	f := &gameFake{queries: make(chan fakeQuery, 8)}
	c, err := NewController(context.Background(), f, s, rand.New(rand.NewPCG(1, 2)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	settings := DefaultSettings()
	settings.Side = side
	if err = c.Start(p, settings); err != nil {
		t.Fatal(err)
	}
	return c, f
}

// takeQuery は要求受理の明示通知を有限時間で待つ。
func takeQuery(t *testing.T, f *gameFake) fakeQuery {
	t.Helper()
	select {
	case q := <-f.queries:
		return q
	case <-time.After(2 * time.Second):
		t.Fatal("query not delivered")
		return fakeQuery{}
	}
}

// pollUntil は作業の完了条件を待つ。時間は停止不良の上限検出だけに使う。
func pollUntil(t *testing.T, c *Controller, ready func() bool) {
	t.Helper()
	limit := time.Now().Add(2 * time.Second)
	for !ready() {
		c.Poll()
		if time.Now().After(limit) {
			t.Fatal("result not applied")
		}
		runtime.Gosched()
	}
}

// replyMove は指定着手だけを返す確定解析を作る。
func replyMove(q fakeQuery, point int) {
	q.reply <- engine.Analysis{ScoreLead: 2, Winrate: .6, Moves: []engine.Candidate{{Point: point, Visits: 64}}, Ownership: make([]float64, q.p.Size*q.p.Size)}
}

// TestControllerUndoAndStale は旧局面の解析が待った後に適用されないことを検証する。
func TestControllerUndoAndStale(t *testing.T) {
	c, f := setupGame(t, 0)
	initial := takeQuery(t, f)
	if err := c.Play(0); err != nil {
		t.Fatal(err)
	}
	select {
	case <-initial.cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("background not cancelled")
	}
	cpu := takeQuery(t, f)
	if !c.Undo() {
		t.Fatal("undo failed")
	}
	replyMove(cpu, 1)
	preview := takeQuery(t, f)
	replyMove(preview, 2)
	pollUntil(t, c, func() bool { return !c.Busy })
	if len(c.Board.Moves) != 0 || c.Board.ToMove != Black || c.UndoCount != 1 {
		t.Fatal("stale CPU move applied")
	}
	c.Abort()
	p, err := c.store.Load(c.Profile.ID)
	if err != nil || p.GamesPlayed != 0 {
		t.Fatal("abort trained", err)
	}
}

// TestControllerWhiteOpening はCPU先手と人の着手前への待ったを検証する。
func TestControllerWhiteOpening(t *testing.T) {
	c, f := setupGame(t, 1)
	replyMove(takeQuery(t, f), 0)
	pollUntil(t, c, func() bool { return len(c.Board.Moves) == 1 })
	if c.Player != White || c.Undo() {
		t.Fatal("opening undo")
	}
	if err := c.Play(1); err != nil {
		t.Fatal(err)
	}
	if !c.Undo() || len(c.Board.Moves) != 1 || c.Board.ToMove != White {
		t.Fatal("white undo")
	}
}

// TestControllerCompletionOnce は連続パス後の保存が一度だけ行われることを検証する。
func TestControllerCompletionOnce(t *testing.T) {
	c, f := setupGame(t, 0)
	initial := takeQuery(t, f)
	replyMove(initial, 0)
	pollUntil(t, c, func() bool { return !c.Busy })
	if err := c.Play(-1); err != nil {
		t.Fatal(err)
	}
	replyMove(takeQuery(t, f), -1)
	pollUntil(t, c, func() bool { return c.Phase == Scoring })
	if err := c.Complete(nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Complete(nil, nil); err != nil {
		t.Fatal(err)
	}
	p, err := c.store.Load(c.Profile.ID)
	if err != nil || p.GamesPlayed != 1 || len(p.Results) != 1 {
		t.Fatal("completion duplicated", err)
	}
	if snapshot, err := c.Snapshot(1); err != nil || len(snapshot.Moves) != 1 {
		t.Fatal("snapshot", err)
	}
}

// TestControllerErrorDelivery は取消を伴う失敗でもBusyを解除して再試行できることを検証する。
func TestControllerErrorDelivery(t *testing.T) {
	c, f := setupGame(t, 0)
	takeQuery(t, f)
	c.cancel()
	pollUntil(t, c, func() bool { return !c.Busy })
	if !errors.Is(c.Err, context.Canceled) {
		t.Fatal(c.Err)
	}
	c.Retry()
	replyMove(takeQuery(t, f), 1)
	pollUntil(t, c, func() bool { return !c.Busy })
	if c.Err != nil {
		t.Fatal(c.Err)
	}
}

// TestReviewNeverTrainsOrMutatesGame は補完解析と独立盤面の変更が棋譜・戦績を変更しないことを検証する。
func TestReviewNeverTrainsOrMutatesGame(t *testing.T) {
	c, f := setupGame(t, 0)
	replyMove(takeQuery(t, f), 0)
	pollUntil(t, c, func() bool { return !c.Busy })
	if err := c.Play(-1); err != nil {
		t.Fatal(err)
	}
	replyMove(takeQuery(t, f), -1)
	pollUntil(t, c, func() bool { return c.Phase == Scoring })
	if err := c.Complete(nil, nil); err != nil {
		t.Fatal(err)
	}
	revision := c.Profile.Revision
	c.Evaluations[0] = nil
	b, err := c.ReviewAt(0)
	if err != nil {
		t.Fatal(err)
	}
	if err = b.Play(0); err != nil {
		t.Fatal(err)
	}
	replyMove(takeQuery(t, f), 1)
	pollUntil(t, c, func() bool { return !c.Busy })
	if len(c.Board.Moves) != 2 || c.Board.Cells[0] != Empty || c.Phase != Review || c.Evaluations[0] == nil {
		t.Fatal("review modified game")
	}
	c.EndReview()
	p, err := c.store.Load(c.Profile.ID)
	if err != nil || p.Revision != revision || p.GamesPlayed != 1 || c.Phase != Finished {
		t.Fatal("review changed profile", err)
	}
}
