// 作成日: 2026-10-19。概要解析の有限性・取消・再試行・戦績不変を同期チャネルで検証する。
package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tremendous1192/NobiGo/engine"
)

// TestSummarySequentialAndReadOnly は概要が通常の単一解析を使い、結果保存や棋譜変更を行わないことを検証する。
func TestSummarySequentialAndReadOnly(t *testing.T) {
	c, f := prepareReviewTest(t)
	revision := c.Profile.Revision
	if err := c.StartSummary(); err != nil {
		t.Fatal(err)
	}
	total := c.SummarySession.Total
	for i := 0; i < total; i++ {
		q := takeQuery(t, f)
		if q.p.Review {
			t.Fatal("summary requested large local variations")
		}
		replyMove(q, 20)
		pollUntil(t, c, func() bool { return c.SummarySession.Completed == i+1 })
	}
	pollUntil(t, c, func() bool { return c.SummarySession.Ready })
	if !c.SummarySession.Ready || c.Busy || c.SummarySession.Err != nil || len(c.Board.Moves) != 4 || c.Profile.Revision != revision {
		t.Fatal("summary did not finish without mutation")
	}
	if err := c.Complete(nil, nil); err != nil || c.Profile.Revision != revision {
		t.Fatal("summary completion retrained")
	}
	c.EndSummary()
	if c.Phase != Finished || c.Busy {
		t.Fatal("summary exit failed")
	}
}

// TestSummaryCancelAndRetry は失敗を通知し、明示再試行と終了後に旧応答を破棄することを検証する。
func TestSummaryCancelAndRetry(t *testing.T) {
	c, f := prepareReviewTest(t)
	if err := c.StartSummary(); err != nil {
		t.Fatal(err)
	}
	old := takeQuery(t, f)
	c.cancel()
	pollUntil(t, c, func() bool { return c.SummarySession.Ready })
	if !errors.Is(c.SummarySession.Err, context.Canceled) {
		t.Fatal("failure hidden")
	}
	if err := c.StartSummary(); err != nil {
		t.Fatal(err)
	}
	q := takeQuery(t, f)
	c.EndSummary()
	select {
	case <-q.cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("summary cancellation blocked")
	}
	replyMove(old, 10)
	replyMove(q, 11)
	c.Poll()
	for _, a := range c.Evaluations {
		if a != nil {
			t.Fatal("old summary response applied")
		}
	}
}

// TestSummaryDeadlineAndRequestBound は追加局面数と総期限を制限し、対局中の概要開始を拒否することを検証する。
func TestSummaryDeadlineAndRequestBound(t *testing.T) {
	b, _ := NewBoard(19, Japanese)
	for i := 0; i < 40; i++ {
		if err := b.Play(i * 2); err != nil {
			t.Fatal(err)
		}
	}
	data, err := BuildGameSummary(b, Black, nil)
	if err != nil {
		t.Fatal(err)
	}
	turns := summaryPositions(b, data, nil)
	if len(turns) != MaxSummaryPositions {
		t.Fatal("request bound", len(turns))
	}
	c, f := prepareReviewTest(t)
	if err = c.StartSummary(); err != nil {
		t.Fatal(err)
	}
	takeQuery(t, f)
	if !c.expireSummary(c.SummarySession.deadline) || c.Busy || !errors.Is(c.SummarySession.Err, context.DeadlineExceeded) {
		t.Fatal("deadline did not stop summary")
	}
	c.Phase = Playing
	if err = c.StartSummary(); err == nil {
		t.Fatal("summary interrupted game")
	}
}

// TestSummaryReusesCompleteAnalysis は十分な既存解析があればエンジンへ要求せず概要を確定することを検証する。
func TestSummaryReusesCompleteAnalysis(t *testing.T) {
	c, _ := prepareReviewTest(t)
	for i := range c.Evaluations {
		c.Evaluations[i] = &engine.Analysis{Winrate: .5, Moves: []engine.Candidate{{Point: 20, Visits: 10}}}
	}
	if err := c.StartSummary(); err != nil {
		t.Fatal(err)
	}
	pollUntil(t, c, func() bool { return c.SummarySession.Ready })
	if c.Busy || c.SummarySession.Total != 0 {
		t.Fatal("complete evaluations were requested again")
	}
}
