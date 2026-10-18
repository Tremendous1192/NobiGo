// 作成日: 2026-10-18。解析失敗後に再起動できるエンジンを提供する。同時呼出しは禁止。
package engine

import (
	"context"
	"errors"
)

// Managed は起動作業または対局作業が順番に所有する。最後の作業終了後にCloseする。
type Managed struct {
	Root   string
	client *Client
}

// Prepare はモデルを準備する。ctxは起動だけを制限し、成功後の寿命はCloseで終了する。
func (m *Managed) Prepare(ctx context.Context) error {
	if m.client != nil {
		select {
		case <-m.client.stop:
			if err := m.Close(); err != nil {
				return err
			}
		default:
			return nil
		}
	}
	life, cancel := context.WithCancel(context.Background())
	c, err := startRoot(ctx, life, cancel, m.Root)
	if err == nil {
		m.client = c
	}
	return err
}

// Analyze は要求を直列実行し、失敗したプロセスを回収する。次の要求で再起動する。
func (m *Managed) Analyze(ctx context.Context, p Position) (Analysis, error) {
	return m.AnalyzeProgress(ctx, p, nil)
}

// AnalyzeProgress は途中通知付きの解析を直列実行する。通知の非待機・読取り専用条件はClientと同じ。
func (m *Managed) AnalyzeProgress(ctx context.Context, p Position, notify func(Analysis)) (Analysis, error) {
	if err := m.Prepare(ctx); err != nil {
		return Analysis{}, err
	}
	a, err := m.client.AnalyzeProgress(ctx, p, notify)
	// 通常の待った・背景解析取消は、プロセスを再利用して次の着手を遅らせない。
	closed := false
	select {
	case <-m.client.stop:
		closed = true
	default:
	}
	if err != nil && (!errors.Is(err, context.Canceled) || closed) {
		err = errors.Join(err, m.Close())
	}
	return a, err
}

// Close は解析作業が終了してから呼ぶ。再度Prepareすることもできる。
func (m *Managed) Close() error {
	if m.client == nil {
		return nil
	}
	err := m.client.Close()
	m.client = nil
	return err
}
