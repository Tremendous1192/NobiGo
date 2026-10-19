// 作成日: 2026-10-18。明示されたローカルエンジンでJSON連携を確認する。通常テストでは実行しない。
package engine

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestRealEngine は実エンジンの起動・3ルールの解析・終了を検証する。
func TestRealEngine(t *testing.T) {
	root := os.Getenv("PLAYGO_ENGINE_ROOT")
	if root == "" {
		t.Skip("set PLAYGO_ENGINE_ROOT to opt into local engine checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	c, err := Start(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, rule := range []string{"japanese", "chinese", "stone-scoring"} {
		started := time.Now()
		a, err := c.Analyze(ctx, Position{Size: 7, Rules: rule, Komi: 0})
		if err != nil {
			t.Fatal(rule, err)
		}
		if len(a.Ownership) != 49 || len(a.Policy) != 50 || len(a.Moves) == 0 {
			t.Fatal("incomplete analysis")
		}
		normalMS := time.Since(started).Milliseconds()
		started = time.Now()
		review, err := c.Analyze(ctx, Position{Size: 7, Rules: rule, Komi: 0, Review: true})
		if err != nil || !review.LocalRead || len(review.Moves) == 0 || len(review.Moves[0].PV) == 0 || len(review.Moves[0].Ownership) != 49 {
			t.Fatalf("review data missing: %v", err)
		}
		t.Logf("%s normal=%dms review=%dms partial=%v", rule, normalMS, time.Since(started).Milliseconds(), a.Partial)
	}
}
