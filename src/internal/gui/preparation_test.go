// 作成日: 2026-10-19。初回準備の期限分離・取消・単一所有と表示例を検証する。
package gui

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Tremendous1192/NobiGo/engine"
)

// TestPreparationStagesAndDeadlines はモデル取得後に独立した期限でエンジンを読み込むことを検証する。
func TestPreparationStagesAndDeadlines(t *testing.T) {
	var stages []preparationStage
	var modelDeadline time.Time
	r := runPreparation(context.Background(), "fixture", true, func(s preparationStage) { stages = append(stages, s) }, func(ctx context.Context, root string, repair bool, notify func(engine.ModelStage)) error {
		modelDeadline, _ = ctx.Deadline()
		if root != "fixture" || !repair || time.Until(modelDeadline) < 2*time.Minute {
			t.Fatal("wrong model preparation contract")
		}
		notify(engine.ModelChecking)
		notify(engine.ModelDownloading)
		notify(engine.ModelVerifying)
		return nil
	}, func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 30*time.Second || !deadline.Before(modelDeadline) {
			t.Fatal("engine deadline was not separated")
		}
		return nil
	})
	if r.err != nil || !slices.Equal(stages, []preparationStage{preparationChecking, preparationDownloading, preparationVerifying, preparationLoading}) {
		t.Fatalf("unexpected preparation: %+v %v", r, stages)
	}
}

// TestPreparationFailureBlocksEngine は取得失敗・取消の後にエンジンを開始しないことを検証する。
func TestPreparationFailureBlocksEngine(t *testing.T) {
	for _, failure := range []error{engine.ErrModelInvalid, context.Canceled, errors.New("network failed")} {
		r := runPreparation(context.Background(), "fixture", false, func(preparationStage) {}, func(context.Context, string, bool, func(engine.ModelStage)) error { return failure }, func(context.Context) error { t.Error("engine loaded after failed preparation"); return nil })
		if !errors.Is(r.err, failure) || r.engineMS != 0 {
			t.Fatalf("failure lost: %v", r.err)
		}
	}
}

// TestPreparationCancelAndRetry は作業の重複・再試行への旧通知混入を防ぎ、取消完了を待てることを検証する。
func TestPreparationCancelAndRetry(t *testing.T) {
	g := &window{engine: &engine.Managed{Root: "fixture"}}
	started := make(chan struct{})
	g.startPreparation(func(ctx context.Context, _ string, _ bool, report func(engine.ModelStage)) error {
		report(engine.ModelDownloading)
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}, func(context.Context) error { t.Error("engine started after cancellation"); return nil })
	<-started
	oldReady, oldDone := g.ready, g.engineDone
	g.startPreparation(nil, nil)
	if oldReady != g.ready || oldDone != g.engineDone {
		t.Fatal("duplicate preparation started")
	}
	g.cancelPreparation()
	<-oldDone
	if r := <-oldReady; !errors.Is(r.err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", r.err)
	}
	g.engineLoading = false
	g.startPreparation(func(context.Context, string, bool, func(engine.ModelStage)) error { return nil }, func(context.Context) error { return nil })
	<-g.engineDone
	if r := <-g.ready; r.err != nil || oldReady == g.ready {
		t.Fatal("retry reused failed preparation")
	}
}

// TestStartupPreparationPreviews は準備中・失敗・修復・取消の画面例が実資源に触れないことを検証する。
func TestStartupPreparationPreviews(t *testing.T) {
	for _, mode := range []string{"startup-check", "startup-download", "startup-verify", "startup-load", "startup-error", "startup-repair", "startup-cancel"} {
		g := &window{previewMode: true}
		if err := g.preparePreview(mode); err != nil {
			t.Fatal(err)
		}
		if g.store != nil || g.engine != nil || g.engineDone != nil {
			t.Fatal("preview accessed real resources")
		}
		if !g.engineLoading && g.engineError == nil {
			t.Fatal("missing preparation status")
		}
	}
	if err := (&window{}).preparePreview("startup-unknown"); err == nil {
		t.Fatal("unknown startup mode accepted")
	}
}
