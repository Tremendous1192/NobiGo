// 作成日: 2026-10-19。AIの根拠・範囲制限・前後局面・読取り専用の境界を検証する。
package gui

import (
	"context"
	"image"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Tremendous1192/PlayGo/engine"
	"github.com/Tremendous1192/PlayGo/internal/app"
)

// TestRealReviewReading は明示したローカルAIの応答を合法再生し、盤面の推定へ取り込めることを検証する。
func TestRealReviewReading(t *testing.T) {
	root := os.Getenv("PLAYGO_ENGINE_ROOT")
	if root == "" {
		t.Skip("set PLAYGO_ENGINE_ROOT to opt into local engine checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	client, err := engine.Start(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	b := tacticsBoard(t)
	position := app.EnginePosition(b)
	position.Review = true
	a, err := client.Analyze(ctx, position)
	if err != nil {
		t.Fatal(err)
	}
	p := newDiagramPosition(b)
	r := analyzeTactics(&p, &a)
	if !a.LocalRead || len(r.life) != len(p.groups) || r.variations == 0 {
		for _, m := range a.Moves[:min(8, len(a.Moves))] {
			t.Logf("point=%d visits=%d pv=%v pvVisits=%v", m.Point, m.Visits, m.PV, m.PVVisits)
		}
		t.Fatalf("no usable real reading: partial=%v variations=%d", a.Partial, r.variations)
	}
	t.Logf("real reading: %d variations, %d plies", r.variations, r.depth)
}

// tacticsBoard は黒の離れた2連と白の手番を持つ合法局面を作る。
func tacticsBoard(t *testing.T) *app.Board {
	t.Helper()
	b, err := app.NewBoard(7, app.Japanese)
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range []int{8, 48, 10} {
		if err = b.Play(point); err != nil {
			t.Fatal(err)
		}
	}
	return b
}

// tacticsAnalysis は明示された探索変化だけを持つ合成データを返す。
func tacticsAnalysis(p *diagramPosition, pvs ...[]int) *engine.Analysis {
	ownership := make([]float64, len(p.cells))
	for i, color := range p.cells {
		if color == app.Black {
			ownership[i] = .9
		}
		if color == app.White {
			ownership[i] = -.9
		}
	}
	a := &engine.Analysis{LocalRead: true, Ownership: ownership}
	for _, pv := range pvs {
		visits := make([]int, len(pv))
		for i := range visits {
			visits[i] = 4
		}
		a.Moves = append(a.Moves, engine.Candidate{Point: pv[0], Visits: 8, PV: pv, PVVisits: visits, Ownership: slices.Clone(ownership)})
	}
	return a
}

// TestTacticsConnectionEvidence は共有点への攻めの成功・未確認・低探索・違法を区別し、棋譜を変えないことを検証する。
func TestTacticsConnectionEvidence(t *testing.T) {
	b := tacticsBoard(t)
	p := newDiagramPosition(b)
	before := slices.Clone(b.Cells)
	for _, tc := range []struct {
		name  string
		pvs   [][]int
		want  readStrength
		count int
	}{
		{"unread", nil, readUnknown, 0},
		{"cut", [][]int{{9, 15}}, readRisk, 1},
		{"one-reconnect", [][]int{{9, 15, 0, 16, 1, 17}}, readUnknown, 1},
		{"two-reconnect", [][]int{{9, 15, 0, 16, 1, 17}, {0, 15, 9, 16, 1, 17}}, readStrong, 2},
		{"unrelated", [][]int{{0, 15}}, readUnknown, 1},
		{"illegal", [][]int{{9, 8}}, readUnknown, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := analyzeTactics(&p, tacticsAnalysis(&p, tc.pvs...))
			if len(r.links) != 1 || r.links[0].strength != tc.want || r.variations != tc.count {
				t.Fatalf("read=%+v", r)
			}
		})
	}
	a := tacticsAnalysis(&p, []int{9, 15})
	a.Moves[0].PVVisits[1] = 1
	if r := analyzeTactics(&p, a); r.variations != 0 || r.links[0].strength != readUnknown {
		t.Fatal("single-visit reading trusted")
	}
	a.Partial = true
	if r := analyzeTactics(&p, a); r.life[0] != readUnknown {
		t.Fatal("partial reading trusted")
	}
	if !slices.Equal(b.Cells, before) || len(b.Moves) != 3 || !slices.Equal(p.cells, before) {
		t.Fatal("analysis mutated source")
	}
}

// TestSurvivalRequiresUsableReading は所有予測が強くても低探索・違法・単一変化を安定表示しないことを検証する。
func TestSurvivalRequiresUsableReading(t *testing.T) {
	p := newDiagramPosition(tacticsBoard(t))
	for _, tc := range []struct {
		name string
		pvs  [][]int
		low  bool
		want readStrength
	}{
		{"missing", nil, false, readUnknown},
		{"one", [][]int{{0, 15}}, false, readUnknown},
		{"low", [][]int{{0, 15}, {1, 16}}, true, readUnknown},
		{"illegal", [][]int{{0, 8}, {1, 10}}, false, readUnknown},
		{"two", [][]int{{0, 15}, {1, 16}}, false, readStrong},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := tacticsAnalysis(&p, tc.pvs...)
			if tc.low {
				for i := range a.Moves {
					a.Moves[i].PVVisits[1] = 1
				}
			}
			got := analyzeTactics(&p, a)
			for _, life := range got.life {
				if life != tc.want {
					t.Fatalf("survival=%v want=%v", life, tc.want)
				}
			}
		})
	}
}

// TestSurvivalPredictionAndCapture は白黒の符号・未確定・実際の捕獲を所有予測より優先することを検証する。
func TestSurvivalPredictionAndCapture(t *testing.T) {
	for _, color := range []app.Color{app.Black, app.White} {
		for _, tc := range []struct {
			value float64
			want  readStrength
		}{{.9, readStrong}, {-.9, readRisk}, {.2, readUnknown}} {
			value := tc.value
			if color == app.White {
				value = -value
			}
			if got := ownershipLife([]int{0}, color, []float64{value}); got != tc.want {
				t.Fatal("ownership perspective")
			}
		}
	}
	b, err := app.NewBoard(7, app.Japanese)
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range []int{0, 1, 48} {
		if err = b.Play(point); err != nil {
			t.Fatal(err)
		}
	}
	p := newDiagramPosition(b)
	a := tacticsAnalysis(&p, []int{7, 47})
	r := analyzeTactics(&p, a)
	if r.life[p.groupAt[0]] != readRisk {
		t.Fatal("captured stone marked safe")
	}
}

// TestConnectionsStayInsideFocus は全路数でつながりの線が着目範囲内に収まることを検証する。
func TestConnectionsStayInsideFocus(t *testing.T) {
	for _, size := range []int{7, 9, 11, 13, 15, 19} {
		b, err := fixtureBoard(size, app.Japanese, size*3)
		if err != nil {
			t.Fatal(err)
		}
		p := newDiagramPosition(b)
		for center := range p.cells {
			d := newReviewDiagram(nil, b, nil)
			d.center = center
			view := d.viewport()
			for _, link := range sharedLinks(&p) {
				a, bridge, c, ok := visibleLink(&p, view, link)
				if ok {
					for _, point := range []int{a, bridge, c} {
						if !image.Pt(point%size, point/size).In(view) {
							t.Fatal("link escaped crop")
						}
					}
				}
			}
		}
	}

}

// TestTacticsRefreshUsesMatchingStage は前後の異なる解析と欠損を混同せず、同じ結果を再利用することを検証する。
func TestTacticsRefreshUsesMatchingStage(t *testing.T) {
	b := tacticsBoard(t)
	before, err := app.Replay(b.Size, b.Rule, b.Moves, 2)
	if err != nil {
		t.Fatal(err)
	}
	d := newReviewDiagram(before, b, nil)
	a := tacticsAnalysis(&d.after, []int{9, 15})
	g := &window{controller: &app.Controller{Evaluations: []*engine.Analysis{nil, nil, nil, a}}}
	g.refreshTactics(&d.after, 3)
	want := d.after.read
	g.refreshTactics(&d.after, 3)
	if !reflect.DeepEqual(want, d.after.read) {
		t.Fatal("cached read changed")
	}
	g.refreshTactics(&d.before, 2)
	if d.before.readSource != nil || d.before.read.variations != 0 {
		t.Fatal("after result used for before")
	}
	g.controller.Evaluations[3] = nil
	g.refreshTactics(&d.after, 3)
	if d.after.read.life[0] != readUnknown {
		t.Fatal("stale AI survived missing evaluation")
	}
}
