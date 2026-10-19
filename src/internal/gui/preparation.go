// 作成日: 2026-10-19。起動準備の所有権・取消・取得と読込みの期限を管理する。
package gui

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/Tremendous1192/NobiGo/engine"
	"github.com/hajimehoshi/ebiten/v2"
)

type preparationStage int

const (
	preparationChecking preparationStage = iota
	preparationDownloading
	preparationVerifying
	preparationLoading
)

type preparationResult struct {
	err               error
	modelMS, engineMS int64
	downloaded        bool
}

type preparationMeasurement struct {
	ModelMS    int64 `json:"modelPreparationMilliseconds"`
	EngineMS   int64 `json:"engineLoadMilliseconds"`
	TotalMS    int64 `json:"totalMilliseconds"`
	Downloaded bool  `json:"downloaded"`
	Passed     bool  `json:"passed"`
}

// runPreparation はモデル準備に3分、エンジン読込みに独立した30秒を与える。
func runPreparation(ctx context.Context, root string, repair bool, report func(preparationStage), prepare func(context.Context, string, bool, func(engine.ModelStage)) error, load func(context.Context) error) preparationResult {
	started := time.Now()
	modelCtx, modelCancel := context.WithTimeout(ctx, 3*time.Minute)
	downloaded := false
	err := prepare(modelCtx, root, repair, func(stage engine.ModelStage) {
		downloaded = downloaded || stage == engine.ModelDownloading
		report(preparationStage(stage))
	})
	modelCancel()
	r := preparationResult{err: err, modelMS: time.Since(started).Milliseconds(), downloaded: downloaded}
	if r.err != nil {
		return r
	}
	if r.err = ctx.Err(); r.err != nil {
		return r
	}
	report(preparationLoading)
	engineCtx, engineCancel := context.WithTimeout(ctx, 30*time.Second)
	defer engineCancel()
	started = time.Now()
	r.err = load(engineCtx)
	r.engineMS = time.Since(started).Milliseconds()
	return r
}

// startPreparation は作業を一つだけ開始する。GUIが所有し、取消後はengineDoneを待ってからエンジンを閉じる。
func (g *window) startPreparation(prepare func(context.Context, string, bool, func(engine.ModelStage)) error, load func(context.Context) error) {
	if g.engineLoading {
		return
	}
	if g.engineCancel != nil {
		g.engineCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	g.engineCancel = cancel
	g.engineLoading = true
	g.engineError = nil
	g.preparationStage = preparationChecking
	g.engineBegan = time.Now()
	// 段階は最大4件、完了は1件。GUIが受け取らず終了しても作業を待機させない。
	g.preparationUpdates = make(chan preparationStage, 4)
	g.ready = make(chan preparationResult, 1)
	g.engineDone = make(chan struct{})
	updates, ready, done := g.preparationUpdates, g.ready, g.engineDone
	root, repair := g.engine.Root, g.modelRepair
	go func() {
		defer cancel()
		r := runPreparation(ctx, root, repair, func(stage preparationStage) {
			select {
			case updates <- stage:
			default:
			}
		}, prepare, load)
		// 完了通知より先に所有権を返し、再試行と前の作業が重ならないようにする。
		close(done)
		ready <- r
	}()
}

// cancelPreparation は作業へ取消を通知する。完了受信までは再試行を無効にする。
func (g *window) cancelPreparation() {
	if g.engineCancel != nil {
		g.engineCancel()
	}
}

// finishPreparationReport は検査専用の起動結果を書き、実プロフィールに触れず終了する。
func (g *window) finishPreparationReport(r preparationResult) error {
	data, err := json.MarshalIndent(preparationMeasurement{r.modelMS, r.engineMS, time.Since(g.engineBegan).Milliseconds(), r.downloaded, r.err == nil}, "", "  ")
	if err == nil {
		err = os.WriteFile(g.preparationReport, append(data, '\n'), 0600)
	}
	if err = errors.Join(r.err, err); err != nil {
		return err
	}
	return ebiten.Termination
}
