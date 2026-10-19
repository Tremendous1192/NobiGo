// 作成日: 2026-10-18。対局状態はGUI側の単一所有者が更新し、解析だけを有限キューの作業へ委ねる。
package app

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/Tremendous1192/PlayGo/engine"
)

// CPUResponseLimit は操作確定からCPU着手の描画完了までの受入上限。
const CPUResponseLimit = 7500 * time.Millisecond

type Phase uint8

const (
	Idle Phase = iota
	Playing
	Scoring
	Finished
	Review
	Summary
)

// Analyzer の実装はcontext取消で速やかに戻る。Controllerは同時に一度だけ呼び出す。
type Analyzer interface {
	Analyze(context.Context, engine.Position) (engine.Analysis, error)
}

// progressAnalyzer は途中通知に対応する解析器だけが実装する。通知は解析作業内で呼び、配列は以後変更しない。
type progressAnalyzer interface {
	AnalyzeProgress(context.Context, engine.Position, func(engine.Analysis)) (engine.Analysis, error)
}
type analysisJob struct {
	ctx      context.Context
	id       uint64
	turn     int
	position engine.Position
}
type analysisResult struct {
	id       uint64
	turn     int
	analysis engine.Analysis
	err      error
}

// Controller はUIスレッド専用。Closeだけは停止完了を待ち、解析器自体のCloseは生成側が担当する。
type Controller struct {
	Phase           Phase
	Board           *Board
	Player          Color
	Profile         Profile
	Settings        Settings
	Strength        float64
	Evaluations     []*engine.Analysis
	Score           Score
	Err             error
	Busy            bool
	UndoCount       int
	LastCPUTime     time.Duration
	CPUDeadlineMiss bool
	SummarySession  *SummarySession

	store           *Store
	rng             *rand.Rand
	gameID          string
	jobs            chan analysisJob
	results         chan analysisResult
	progress        chan analysisResult
	stop            context.CancelFunc
	done            chan struct{}
	cancel          context.CancelFunc
	generation      uint64
	cpubegan        time.Time
	cpuPresented    int
	reviewTurn      int
	failedTurn      int
	reviewRequested map[int]bool // 表示中の前後2局面だけを再解析し、失敗・未対応時の自動再送を防ぐ。
}

// NewController は1つの解析作業を開始する。呼出し元はCloseして終了を待つ。
func NewController(ctx context.Context, backend Analyzer, store *Store, rng *rand.Rand) (*Controller, error) {
	if backend == nil || store == nil || rng == nil {
		return nil, errors.New("missing controller dependency")
	}
	life, stop := context.WithCancel(ctx)
	c := &Controller{store: store, rng: rng, jobs: make(chan analysisJob, 1), results: make(chan analysisResult, 1), progress: make(chan analysisResult, 1), done: make(chan struct{}), stop: stop}
	// この作業だけがbackendを呼ぶ。1件の待機要求は更新時に置換し、送信待ちは取消で解除する。
	go c.work(life, backend)
	return c, nil
}

// work は要求ごとのcontextを守り、結果の所有権をUIへ渡す。
func (c *Controller) work(ctx context.Context, backend Analyzer) {
	defer close(c.done)
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-c.jobs:
			var a engine.Analysis
			err := job.ctx.Err()
			if err == nil {
				if progressive, ok := backend.(progressAnalyzer); ok {
					a, err = progressive.AnalyzeProgress(job.ctx, job.position, func(a engine.Analysis) { c.publishProgress(job, a) })
				} else {
					a, err = backend.Analyze(job.ctx, job.position)
				}
			}
			r := analysisResult{job.id, job.turn, a, err}
			// 期限切れもUIへ通知する。古い未読結果だけを捨て、Busyが残ることを防ぐ。
			select {
			case c.results <- r:
			default:
				select {
				case <-c.results:
				default:
				}
				select {
				case c.results <- r:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

// publishProgress は最新の途中通知1件だけを保持する。作業が単独送信し、UIが受信し、終了時にもcloseしない。
func (c *Controller) publishProgress(job analysisJob, a engine.Analysis) {
	r := analysisResult{id: job.id, turn: job.turn, analysis: a}
	// 描画待ちでCPU着手の確定応答を遅らせないよう、途中通知は別キューで古い値を置換する。
	select {
	case <-c.progress:
	default:
	}
	select {
	case c.progress <- r:
	default:
	}
}

// invalidate は保留要求・結果を無効にし、進行中の解析を取り消す。
func (c *Controller) invalidate() {
	c.generation++
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.Busy = false
	select {
	case <-c.jobs:
	default:
	}
}

// Close は解析を取り消して唯一の作業の終了を待つ。UI所有者が最後に呼ぶ。
func (c *Controller) Close() { c.invalidate(); c.stop(); <-c.done }

// request は待ち時間も含む6秒の期限を設定する。残り1.5秒を取消・着手選択・描画に確保する。
func (c *Controller) request() {
	c.requestPosition(c.Board, len(c.Board.Moves))
}

// requestPosition は指定した再生局面の解析も、対局と同じ有限キューへ送る。
func (c *Controller) requestPosition(b *Board, turn int) {
	c.invalidate()
	limit := 6 * time.Second
	if c.Phase == Summary && c.SummarySession != nil {
		limit = min(limit, time.Until(c.SummarySession.deadline))
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	c.cancel = cancel
	c.Busy = true
	c.Err = nil
	position := EnginePosition(b)
	position.Review = c.Phase == Review
	c.jobs <- analysisJob{ctx, c.generation, turn, position}
}

// Start は設定保存の成功後に新規対局を開始し、強度を固定する。
func (c *Controller) Start(p Profile, s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := p.Validate(); err != nil {
		return err
	}
	b, err := NewBoard(s.Size, s.Rule)
	if err != nil {
		return err
	}
	id, err := NewID()
	if err != nil {
		return err
	}
	p.Last = s
	if err = c.store.Save(&p); err != nil {
		return err
	}
	c.invalidate()
	c.Profile = p
	c.Settings = s
	c.Board = b
	c.gameID = id
	c.Strength = Strength(p, s)
	c.Player = Black
	if s.Side == 1 || s.Side == 2 && c.rng.IntN(2) == 1 {
		c.Player = White
	}
	c.Evaluations = make([]*engine.Analysis, 1)
	c.UndoCount = 0
	c.Score = Score{}
	c.SummarySession = nil
	c.Phase = Playing
	c.LastCPUTime = 0
	c.CPUDeadlineMiss = false
	c.cpuPresented = 0
	c.Err = nil
	if c.Player == White {
		c.cpubegan = time.Now()
	}
	c.request()
	return nil
}

// Play はプレイヤーの合法着手を受理し、必要ならCPU解析を要求する。
func (c *Controller) Play(point int) error {
	if c.Phase != Playing || c.Board.ToMove != c.Player {
		return errors.New("not player turn")
	}
	started := time.Now()
	if err := c.Board.Play(point); err != nil {
		return err
	}
	c.invalidate()
	c.Err = nil
	c.Evaluations = append(c.Evaluations, nil)
	if c.endPosition() {
		return nil
	}
	c.cpubegan = started
	c.request()
	return nil
}

// endPosition は連続パスまたは反復無勝負を採点状態へ移す。
func (c *Controller) endPosition() bool {
	if c.Board.Passes >= 2 || c.Board.NoResult {
		c.Phase = Scoring
		return true
	}
	return false
}

// matchesAnalysis は現在の対局または再生要求に属する結果だけを受理する。
func (c *Controller) matchesAnalysis(r analysisResult) bool {
	return r.id == c.generation && c.Board != nil && r.turn >= 0 && r.turn < len(c.Evaluations) &&
		(c.Phase == Review || c.Phase == Summary || c.Phase == Playing && r.turn == len(c.Board.Moves))
}

// Poll は確定応答を優先し、なければ途中評価だけを更新する。途中通知では着手せずBusyも解除しない。
func (c *Controller) Poll() {
	if c.expireSummary(time.Now()) {
		return
	}
	select {
	case r := <-c.results:
		if !c.matchesAnalysis(r) {
			return
		}
		c.Busy = false
		if c.cancel != nil {
			c.cancel()
			c.cancel = nil
		}
		if r.err != nil {
			if c.Phase == Summary {
				c.finishSummary(r.err)
				return
			}
			c.Err = r.err
			c.failedTurn = r.turn
			return
		}
		c.Evaluations[r.turn] = &r.analysis
		if c.Phase == Summary {
			c.SummarySession.Completed++
			c.requestSummaryAnalysis()
			return
		}
		if c.Phase == Review {
			c.requestReviewAnalysis()
			return
		}
		if c.Phase != Playing || c.Board.ToMove == c.Player {
			return
		}
		point, err := SelectMove(c.Board, r.analysis, c.Strength, c.rng)
		if err != nil {
			c.Err = err
			return
		}
		if err = c.Board.Play(point); err != nil {
			c.Err = err
			return
		}
		c.LastCPUTime = time.Since(c.cpubegan)
		c.Evaluations = append(c.Evaluations, nil)
		if !c.endPosition() {
			c.request()
		}
	default:
		select {
		case r := <-c.progress:
			if c.Busy && c.matchesAnalysis(r) {
				c.Evaluations[r.turn] = &r.analysis
			}
		default:
		}
	}
}

// Retry は現在局面の解析を再試行する。CPU手番なら7.5秒の計測も開始し直す。
func (c *Controller) Retry() {
	if c.Board == nil || c.Phase != Playing {
		return
	}
	if c.Board.ToMove != c.Player {
		c.cpubegan = time.Now()
	}
	c.request()
}

// Undo は最後の人の着手前へ戻し、取り消した解析値も除去する。
func (c *Controller) Undo() bool {
	if c.Phase != Playing && c.Phase != Scoring {
		return false
	}
	if !c.Board.UndoPlayer(c.Player) {
		return false
	}
	c.invalidate()
	c.Phase = Playing
	c.Err = nil
	c.UndoCount++
	c.Evaluations = c.Evaluations[:len(c.Board.Moves)+1]
	c.cpuPresented = min(c.cpuPresented, len(c.Board.Moves))
	c.request()
	return true
}

// Abort は結果登録をせず起動画面へ戻す。開始時の設定保存は取り消さない。
func (c *Controller) Abort() { c.invalidate(); c.Phase = Idle; c.Err = nil }

// Loss は連続する解析が揃った着手の推定損失を返す。未解析をゼロとして表示しない。
func (c *Controller) Loss(turn int) (float64, bool) {
	if c.Board == nil || turn < 1 || turn >= len(c.Evaluations) || c.Evaluations[turn-1] == nil || c.Evaluations[turn] == nil {
		return 0, false
	}
	loss := c.Evaluations[turn-1].ScoreLead - c.Evaluations[turn].ScoreLead
	if c.Board.Moves[turn-1].Color == White {
		loss = -loss
	}
	return math.Max(0, loss), true
}

// Complete は確定採点を1回だけ保存する。失敗時には採点状態を維持して再試行できる。
func (c *Controller) Complete(dead, seki map[int]bool) error {
	if c.Phase == Finished || c.Phase == Review || c.Phase == Summary {
		return nil
	}
	if c.Phase != Scoring {
		return errors.New("game is not ready for scoring")
	}
	score, err := ScoreBoard(c.Board, dead, seki)
	if err != nil {
		return err
	}
	loss, count := 0.0, 0
	for i, m := range c.Board.Moves {
		if m.Color == c.Player {
			if n, ok := c.Loss(i + 1); ok {
				loss += n
				count++
			}
		}
	}
	if count > 0 {
		loss /= float64(count)
	}
	g := GameResult{ID: c.gameID, Settings: c.Settings, Player: c.Player, BlackScore: score.Black, WhiteScore: score.White, NoResult: score.NoResult, MoveCount: len(c.Board.Moves), UndoCount: c.UndoCount, MeanLoss: loss, FinishedAt: time.Now().UTC().Format(time.RFC3339)}
	p, err := CompletedProfile(c.Profile, g)
	if err != nil {
		return err
	}
	if err = c.store.Save(&p); err != nil {
		return err
	}
	c.invalidate()
	c.Profile = p
	c.Score = score
	c.Phase = Finished
	c.Err = nil
	return nil
}

// Snapshot は感想戦用に指定手までの盤面を独立して再現する。
func (c *Controller) Snapshot(turn int) (*Board, error) {
	if c.Board == nil {
		return nil, errors.New("no game to review")
	}
	return Replay(c.Board.Size, c.Board.Rule, slices.Clone(c.Board.Moves), turn)
}

// ReviewAt は指定手の独立盤面を返し、現在と直前の不足解析を順に補う。プロフィールへは反映しない。
func (c *Controller) ReviewAt(turn int) (*Board, error) {
	if c.Phase != Finished && c.Phase != Review {
		return nil, errors.New("game is not finished")
	}
	b, err := c.Snapshot(turn)
	if err != nil {
		return nil, err
	}
	if c.Phase == Review && c.Err != nil && c.failedTurn >= 0 && c.failedTurn < len(c.Evaluations) {
		c.Evaluations[c.failedTurn] = nil
	}
	c.invalidate()
	c.Phase = Review
	c.Err = nil
	c.releaseReviewRead(turn)
	c.reviewTurn = turn
	c.reviewRequested = make(map[int]bool, 2)
	c.requestReviewAnalysis()
	return b, nil
}

// releaseReviewRead は候補別の大きな予測配列を表示前後の2局面に限定し、履歴の目差・勝率は残す。
func (c *Controller) releaseReviewRead(turn int) {
	for i, a := range c.Evaluations {
		if a == nil || !a.LocalRead || i == turn || i == turn-1 {
			continue
		}
		copy := *a
		copy.LocalRead = false
		copy.Moves = slices.Clone(a.Moves)
		for j := range copy.Moves {
			copy.Moves[j].PV, copy.Moves[j].PVVisits, copy.Moves[j].Ownership = nil, nil, nil
		}
		c.Evaluations[i] = &copy
	}
}

// requestReviewAnalysis は表示局面を優先し、その直前だけを補完する。連続パス・無勝負の終局は確定採点を使う。
func (c *Controller) requestReviewAnalysis() {
	for _, turn := range []int{c.reviewTurn, c.reviewTurn - 1} {
		if turn < 0 || c.reviewRequested[turn] || c.Evaluations[turn] != nil && c.Evaluations[turn].LocalRead {
			continue
		}
		b, err := c.Snapshot(turn)
		if err != nil {
			c.Err = err
			c.failedTurn = turn
			return
		}
		if b.Passes >= 2 || b.NoResult {
			continue
		}
		c.reviewRequested[turn] = true
		c.requestPosition(b, turn)
		return
	}
}

// EndReview は補完解析を取り消して、保存済みの結果画面へ戻る。
func (c *Controller) EndReview() {
	if c.Phase == Review {
		c.invalidate()
		c.Phase = Finished
		c.Err = nil
	}
}

// RecordDisplayed はCPU着手を描画した時点までの経過時間を記録する。描画所有者が各フレームに呼ぶ。
func (c *Controller) RecordDisplayed() { c.recordDisplayedAt(time.Now()) }

// recordDisplayedAt は描画時刻で受入上限を判定し、同じ着手を二重計測しない。
func (c *Controller) recordDisplayedAt(now time.Time) {
	if c.Board == nil || len(c.Board.Moves) == 0 || c.Phase == Review || c.Phase == Summary {
		return
	}
	turn := len(c.Board.Moves)
	if turn <= c.cpuPresented || c.Board.Moves[turn-1].Color == c.Player {
		return
	}
	c.cpuPresented = turn
	c.LastCPUTime = now.Sub(c.cpubegan)
	if c.LastCPUTime > CPUResponseLimit {
		c.CPUDeadlineMiss = true
	}
}
