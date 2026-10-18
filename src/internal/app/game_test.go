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
	progress  chan engine.Analysis
	published chan struct{}
}

// prepareReviewTest は保存操作のない再生用の合法棋譜を用意する。
func prepareReviewTest(t *testing.T) (*Controller, *gameFake) {
	t.Helper()
	c, f := setupGame(t, 0)
	takeQuery(t, f)
	c.Abort()
	for _, point := range []int{0, 1, 2, 3} {
		if err := c.Board.Play(point); err != nil {
			t.Fatal(err)
		}
	}
	c.Evaluations = make([]*engine.Analysis, len(c.Board.Moves)+1)
	c.Phase = Finished
	return c, f
}

// TestReviewFillsCurrentThenPrevious は現在局面と直前を自動で順に補完し、結果を保存しないことを検証する。
func TestReviewFillsCurrentThenPrevious(t *testing.T) {
	c, f := prepareReviewTest(t)
	revision := c.Profile.Revision
	if _, err := c.ReviewAt(3); err != nil {
		t.Fatal(err)
	}
	current := takeQuery(t, f)
	if len(current.p.Moves) != 3 {
		t.Fatal("current board was not prioritized")
	}
	replyMove(current, 10)
	pollUntil(t, c, func() bool { return c.Evaluations[3] != nil })
	previous := takeQuery(t, f)
	if len(previous.p.Moves) != 2 || !c.Busy {
		t.Fatal("previous board was not requested")
	}
	replyMove(previous, 11)
	pollUntil(t, c, func() bool { return !c.Busy })
	if c.Evaluations[2] == nil || len(c.Board.Moves) != 4 || c.Profile.Revision != revision {
		t.Fatal("review failed or changed game/profile")
	}
	if _, ok := c.Loss(3); !ok {
		t.Fatal("loss still requires manual backtracking")
	}
}

// TestReviewNavigationCancelsSupplement は移動後に旧局面の補完が割り込まず、終了後も解析を続けないことを検証する。
func TestReviewNavigationCancelsSupplement(t *testing.T) {
	c, f := prepareReviewTest(t)
	if _, err := c.ReviewAt(3); err != nil {
		t.Fatal(err)
	}
	replyMove(takeQuery(t, f), 10)
	pollUntil(t, c, func() bool { return c.Evaluations[3] != nil })
	old := takeQuery(t, f)
	if _, err := c.ReviewAt(0); err != nil {
		t.Fatal(err)
	}
	replyMove(old, 11)
	next := takeQuery(t, f)
	if len(next.p.Moves) != 0 {
		t.Fatal("stale supplement took priority")
	}
	replyMove(next, 12)
	pollUntil(t, c, func() bool { return !c.Busy })
	if c.Evaluations[2] != nil || c.Evaluations[0] == nil {
		t.Fatal("stale review result applied")
	}
	c.EndReview()
	if c.Busy || c.Phase != Finished {
		t.Fatal("review did not stop")
	}
}

type gameFake struct{ queries chan fakeQuery }

// Analyze は応答か取消の一方で終了する試験用解析器。
func (f *gameFake) Analyze(ctx context.Context, p engine.Position) (engine.Analysis, error) {
	return f.AnalyzeProgress(ctx, p, nil)
}

// AnalyzeProgress はテストの指示で途中評価を通知し、publishedで通知完了を同期する。
func (f *gameFake) AnalyzeProgress(ctx context.Context, p engine.Position, notify func(engine.Analysis)) (engine.Analysis, error) {
	q := fakeQuery{p: p, reply: make(chan engine.Analysis, 1), cancelled: make(chan struct{}), progress: make(chan engine.Analysis, 1), published: make(chan struct{}, 1)}
	select {
	case f.queries <- q:
	case <-ctx.Done():
		return engine.Analysis{}, ctx.Err()
	}
	for {
		select {
		case a := <-q.reply:
			return a, nil
		case a := <-q.progress:
			if notify != nil {
				notify(a)
			}
			select {
			case q.published <- struct{}{}:
			case <-ctx.Done():
				close(q.cancelled)
				return engine.Analysis{}, ctx.Err()
			}
		case <-ctx.Done():
			close(q.cancelled)
			return engine.Analysis{}, ctx.Err()
		}
	}
}

// publishTestProgress は作業からの途中通知完了を待ち、UIの受信を待たないことを確認する。
func publishTestProgress(t *testing.T, q fakeQuery, score float64) {
	t.Helper()
	q.progress <- engine.Analysis{ScoreLead: score, Partial: true, Ownership: make([]float64, q.p.Size*q.p.Size)}
	select {
	case <-q.published:
	case <-time.After(2 * time.Second):
		t.Fatal("progress blocked the analyzer")
	}
}

// TestProgressDoesNotPlayAndFinalWins は途中通知で着手せず、満杯でも確定応答が失われないことを検証する。
func TestProgressDoesNotPlayAndFinalWins(t *testing.T) {
	c, f := setupGame(t, 1)
	q := takeQuery(t, f)
	publishTestProgress(t, q, 1)
	c.Poll()
	if !c.Busy || len(c.Board.Moves) != 0 || c.Evaluations[0] == nil || c.Evaluations[0].ScoreLead != 1 {
		t.Fatal("progress completed or played a move")
	}
	for i := 0; i < 100; i++ {
		publishTestProgress(t, q, float64(i))
	}
	replyMove(q, 0)
	pollUntil(t, c, func() bool { return len(c.Board.Moves) == 1 })
	if c.Evaluations[0].ScoreLead != 2 || c.Evaluations[0].Partial {
		t.Fatal("progress overwrote the final result")
	}
	c.Poll()
	if c.Evaluations[1] != nil {
		t.Fatal("old position progress applied to the new board")
	}
}

// TestProgressInvalidatedByUndo は取り消した局面の色分けが現在局面へ適用されないことを検証する。
func TestProgressInvalidatedByUndo(t *testing.T) {
	c, f := setupGame(t, 0)
	takeQuery(t, f)
	if err := c.Play(0); err != nil {
		t.Fatal(err)
	}
	cpu := takeQuery(t, f)
	publishTestProgress(t, cpu, 5)
	if !c.Undo() {
		t.Fatal("undo failed")
	}
	c.Poll()
	if len(c.Evaluations) != 1 || c.Evaluations[0] != nil {
		t.Fatal("stale progress survived undo")
	}
	preview := takeQuery(t, f)
	publishTestProgress(t, preview, 8)
	pollUntil(t, c, func() bool { return c.Evaluations[0] != nil })
	if c.Evaluations[0] == nil || c.Evaluations[0].ScoreLead != 8 || !c.Busy {
		t.Fatal("current progress missing")
	}
	replyMove(preview, 1)
	pollUntil(t, c, func() bool { return !c.Busy })
	c.Poll()
	if c.Evaluations[0].ScoreLead != 2 {
		t.Fatal("late progress replaced final background result")
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
