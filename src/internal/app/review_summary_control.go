// 作成日: 2026-10-19。概要の不足解析を既存の単一作業へ順次送る。状態はGUIスレッドが所有し、保存・対局操作は行わない。
package app

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/Tremendous1192/NobiGo/engine"
)

const MaxSummaryPositions = 12
const SummaryTimeLimit = 60 * time.Second

// SummarySession は一回の概要解析の進捗と確定データを保持する。Ready時には追加要求を送らない。
type SummarySession struct {
	Data             GameSummary
	Ready            bool
	Completed, Total int
	Err              error
	Elapsed          time.Duration
	pending          []int
	began, deadline  time.Time
}

// summaryPositions は各段階の代表局面と効果のある手を優先し、欠損する前後局面を上限以内に重複なく選ぶ。
func summaryPositions(b *Board, data GameSummary, evaluations []*engine.Analysis) []int {
	var turns []int
	add := func(turn int) {
		if turn < 0 || turn > len(b.Moves) || len(turns) >= MaxSummaryPositions || slices.Contains(turns, turn) {
			return
		}
		if turn == len(b.Moves) && (b.Passes >= 2 || b.NoResult) {
			return
		}
		if turn < len(evaluations) && summaryEvaluation(evaluations[turn]) {
			return
		}
		turns = append(turns, turn)
	}
	for _, stage := range data.Stages {
		if stage.From > 0 {
			turn := (stage.From + stage.To) / 2
			add(turn - 1)
			add(turn)
		}
	}
	candidates := slices.Clone(data.Moves)
	slices.SortStableFunc(candidates, func(a, b SummaryMove) int { return summaryEffect(&b) - summaryEffect(&a) })
	for _, move := range candidates {
		add(move.Turn - 1)
		add(move.Turn)
	}
	return turns
}

// StartSummary は完了対局の概要を準備する。再実行は明示操作だけで行い、前の解析を取り消す。
func (c *Controller) StartSummary() error {
	if c.Phase != Finished && c.Phase != Review && c.Phase != Summary {
		return errors.New("game is not finished")
	}
	data, err := BuildGameSummary(c.Board, c.Player, c.Evaluations)
	if err != nil {
		return err
	}
	c.invalidate()
	c.releaseReviewRead(-2)
	c.Phase = Summary
	c.Err = nil
	now := time.Now()
	turns := summaryPositions(c.Board, data, c.Evaluations)
	c.SummarySession = &SummarySession{Data: data, Total: len(turns), pending: turns, began: now, deadline: now.Add(SummaryTimeLimit)}
	c.requestSummaryAnalysis()
	return nil
}

// requestSummaryAnalysis は1件ずつ要求し、完了・期限・再生失敗で停止する。通常対局用の軽い解析を使う。
func (c *Controller) requestSummaryAnalysis() {
	s := c.SummarySession
	if c.Phase != Summary || s == nil || s.Ready {
		return
	}
	if c.expireSummary(time.Now()) {
		return
	}
	if len(s.pending) == 0 {
		c.finishSummary(nil)
		return
	}
	turn := s.pending[0]
	s.pending = s.pending[1:]
	b, err := c.Snapshot(turn)
	if err != nil {
		c.finishSummary(err)
		return
	}
	c.requestPosition(b, turn)
}

// expireSummary は画面更新から総期限を確認し、待機中の解析も取り消す。
func (c *Controller) expireSummary(now time.Time) bool {
	s := c.SummarySession
	if c.Phase == Summary && s != nil && !s.Ready && !now.Before(s.deadline) {
		c.finishSummary(context.DeadlineExceeded)
		return true
	}
	return false
}

// finishSummary は取得できた根拠を固定し、失敗しても確定した対局結果は維持する。
func (c *Controller) finishSummary(cause error) {
	c.invalidate()
	s := c.SummarySession
	data, err := BuildGameSummary(c.Board, c.Player, c.Evaluations)
	if err == nil {
		s.Data = data
	}
	s.Ready = true
	s.Err = errors.Join(cause, err)
	s.Elapsed = time.Since(s.began)
	s.pending = nil
}

// EndSummary は解析を取り消し、再保存せずに終了後の画面へ戻る。
func (c *Controller) EndSummary() {
	if c.Phase == Summary {
		c.invalidate()
		c.Phase = Finished
		c.Err = nil
	}
}
