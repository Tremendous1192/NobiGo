// 作成日: 2026-10-18。盤面の交点と画面外の対応を検証する。
package gui

import (
	"bytes"
	"github.com/Tremendous1192/PlayGo/engine"
	"github.com/Tremendous1192/PlayGo/internal/app"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"image"
	"math"
	"strings"
	"testing"
)

// TestEvaluationRowKeepsPerspectiveAndFits は欠損・白視点・直前の着手側・純碁と許容最大値を1行で表示できることを検証する。
func TestEvaluationRowKeepsPerspectiveAndFits(t *testing.T) {
	source, err := text.NewGoTextFaceSource(bytes.NewReader(fontData))
	if err != nil {
		t.Fatal(err)
	}
	g := &window{face: &text.GoTextFace{Source: source}}
	c, _, _ := adviceFixture(t, app.Jungo, []int{0, 1})
	value, note := evaluationText(c, 2, app.Jungo)
	if value != "目差—  勝率—  直前損失—" || note != "（近似）" {
		t.Fatal("missing values or approximation changed")
	}
	c.Player = app.White
	c.Evaluations[1] = &engine.Analysis{ScoreLead: -20000, Winrate: 0}
	c.Evaluations[2] = &engine.Analysis{ScoreLead: 20000, Winrate: 1}
	value, note = evaluationText(c, 2, app.Jungo)
	if value != "目差-20000.0  勝率0.0%  直前損失40000.0目" {
		t.Fatalf("wrong perspective or loss: %s", value)
	}
	for _, player := range []app.Color{app.Black, app.White} {
		c.Player = player
		for _, rule := range []app.Rule{app.Japanese, app.Chinese, app.Jungo} {
			value, note = evaluationText(c, 2, rule)
			if (note != "") != (rule == app.Jungo) {
				t.Fatal("approximation on wrong rule")
			}
			size, width := g.evaluationRowSize(value, note)
			if size < 14 || size > 18 {
				t.Fatalf("unreadable size=%f", size)
			}
			if note != "" {
				g.face.Size = 12
				nw, _ := text.Measure(note, g.face, 18)
				width += 8 + nw
			}
			if width > 445.01 || strings.Contains(value, "\n") {
				t.Fatalf("row overflow: %f", width)
			}
		}
	}
}

// TestPlotAxesAndPerspective は両軸の上下端・中央と白視点の反転、空の履歴を検証する。
func TestPlotAxesAndPerspective(t *testing.T) {
	r := image.Rect(728, 230, 1090, 315)
	values := []*engine.Analysis{{ScoreLead: -20, Winrate: 0}, {ScoreLead: 0, Winrate: .5}, {ScoreLead: 20, Winrate: 1}}
	for _, player := range []app.Color{app.Black, app.White} {
		for _, winrate := range []bool{false, true} {
			points, bound := plotSeries(values, player, r, winrate)
			if bound != 20 || len(points) != 3 {
				t.Fatal("wrong scale")
			}
			for i, p := range points {
				v := float64(i) / 2
				if player == app.White {
					v = 1 - v
				}
				if !p.valid || p.x != float64(r.Min.X)+float64(i*r.Dx())/2 || p.y != float64(r.Max.Y)-v*85 {
					t.Fatalf("unexpected point: %+v", p)
				}
			}
		}
	}
	points, bound := plotSeries(nil, app.Black, r, false)
	if len(points) != 0 || bound != 10 || len(plotSegments(points, true)) != 0 {
		t.Fatal("empty history produced a line")
	}
}

// TestPlotMissingValuesBreakBothLines は未解析を補間せず、孤立点も座標として残すことを検証する。
func TestPlotMissingValuesBreakBothLines(t *testing.T) {
	values := []*engine.Analysis{{ScoreLead: 4, Winrate: .4}, nil, {ScoreLead: -5, Winrate: .6}}
	for _, dashed := range []bool{false, true} {
		points, _ := plotSeries(values, app.Black, image.Rect(0, 0, 100, 85), dashed)
		if points[1].valid || !points[0].valid || !points[2].valid || len(plotSegments(points, dashed)) != 0 {
			t.Fatal("missing evaluation was bridged")
		}
	}
}

// TestPlotDashesContinueAcrossDenseMoves は4096手でも実線と破線を区別でき、欠損で周期を再開することを検証する。
func TestPlotDashesContinueAcrossDenseMoves(t *testing.T) {
	values := make([]*engine.Analysis, app.MaxMoves+1)
	for i := range values {
		values[i] = &engine.Analysis{Winrate: .5}
	}
	points, _ := plotSeries(values, app.Black, image.Rect(0, 0, 100, 85), true)
	for _, dashed := range []bool{false, true} {
		length := 0.0
		for _, s := range plotSegments(points, dashed) {
			length += math.Hypot(s.to.x-s.from.x, s.to.y-s.from.y)
			if s.from.x < 0 || s.to.x > 100 {
				t.Fatal("line exceeded bounds")
			}
		}
		want := 100.0
		if dashed {
			want = 60
		}
		if math.Abs(length-want) > 1e-6 {
			t.Fatalf("painted length=%f want=%f", length, want)
		}
	}
	points = []plotPoint{{x: 0, valid: true}, {x: 7, valid: true}, {}, {x: 20, valid: true}, {x: 22, valid: true}}
	segments := plotSegments(points, true)
	if len(segments) != 2 || segments[1].from.x != 20 || segments[1].to.x != 22 {
		t.Fatal("gap retained dash phase")
	}
}

// TestBoardPoint は全対応路数の四隅・中央と領域外クリックを検証する。
func TestBoardPoint(t *testing.T) {
	for _, size := range []int{7, 9, 11, 13, 15, 19} {
		for _, point := range []int{0, size - 1, size * (size - 1), size*size - 1, size * size / 2} {
			step := 530.0 / float64(size-1)
			x, y := int(math.Round(70+float64(point%size)*step)), int(math.Round(155+float64(point/size)*step))
			if got := boardPoint(x, y, size); got != point {
				t.Fatalf("size=%d point=%d got=%d", size, point, got)
			}
		}
		if boardPoint(0, 0, size) != -2 || boardPoint(700, 700, size) != -2 {
			t.Fatal("outside accepted")
		}
	}
}

// TestPassCommentUsesCurrentRule は日本・中国ルールへ純碁専用の終局説明を混ぜないことを検証する。
func TestPassCommentUsesCurrentRule(t *testing.T) {
	for _, rule := range []app.Rule{app.Japanese, app.Chinese, app.Jungo} {
		b, err := app.NewBoard(9, rule)
		if err != nil {
			t.Fatal(err)
		}
		if err := b.Play(-1); err != nil {
			t.Fatal(err)
		}
		comment := positionComment(b)
		if !strings.Contains(comment, "続けてパス") || strings.Contains(comment, "純碁") != (rule == app.Jungo) {
			t.Fatalf("wrong pass comment for %s", rule)
		}
	}
}

// TestStoneMoveNumbers はパス・取り石・再着手・再生位置ごとの生存石の番号を検証する。
func TestStoneMoveNumbers(t *testing.T) {
	b, err := app.NewBoard(7, app.Japanese)
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range []int{0, 1, -1, 7, 20, 2, -1, 0} {
		if err := b.Play(point); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ turn, corner int }{{0, 0}, {3, 1}, {4, 0}, {8, 8}, {4, 0}, {8, 8}} {
		snapshot, err := app.Replay(b.Size, b.Rule, b.Moves, tc.turn)
		if err != nil {
			t.Fatal(err)
		}
		numbers := stoneMoveNumbers(snapshot)
		if numbers[0] != tc.corner {
			t.Fatalf("turn=%d corner=%d want=%d", tc.turn, numbers[0], tc.corner)
		}
		for p, c := range snapshot.Cells {
			if c == app.Empty && numbers[p] != 0 || c == app.Black && numbers[p]%2 != 1 || c == app.White && (numbers[p] == 0 || numbers[p]%2 != 0) {
				t.Fatalf("turn=%d point=%d color=%d number=%d", tc.turn, p, c, numbers[p])
			}
		}
	}
	if len(b.Moves) != 8 || b.Cells[0] != app.White {
		t.Fatal("numbering mutated the original game")
	}
}

// TestStoneMoveNumberLimit は上限の4桁番号を切り詰めず保持することを検証する。
func TestStoneMoveNumberLimit(t *testing.T) {
	b, err := app.NewBoard(19, app.Japanese)
	if err != nil {
		t.Fatal(err)
	}
	b.Moves = make([]app.Move, app.MaxMoves)
	for i := range b.Moves {
		b.Moves[i] = app.Move{Point: -1}
	}
	b.Moves[app.MaxMoves-1] = app.Move{Color: app.White, Point: 360}
	b.Cells[360] = app.White
	if got := stoneMoveNumbers(b)[360]; got != app.MaxMoves {
		t.Fatalf("number=%d", got)
	}
}

// TestInitialTabsAndSelection は画面入場時だけ初期タブを選び、採点からの待ったで選択を保持することを検証する。
func TestInitialTabsAndSelection(t *testing.T) {
	c := &app.Controller{Phase: app.Playing}
	g := &window{controller: c}
	g.syncPhase()
	if g.tab != 1 {
		t.Fatal("game did not select comments")
	}
	g.tab = 0
	g.syncPhase()
	if g.tab != 0 {
		t.Fatal("selection was reset within game")
	}
	g.shownPhase = app.Scoring
	g.syncPhase()
	if g.tab != 0 {
		t.Fatal("undo reset selection")
	}
	c.Phase = app.Review
	g.syncPhase()
	if g.tab != 2 {
		t.Fatal("review did not select advice")
	}
	g.tab = 1
	g.syncPhase()
	if g.tab != 1 {
		t.Fatal("selection was reset within review")
	}
	c.Phase = app.Finished
	g.syncPhase()
	c.Phase = app.Review
	g.syncPhase()
	if g.tab != 2 {
		t.Fatal("review reentry did not select advice")
	}
	c.Phase = app.Idle
	g.syncPhase()
	c.Phase = app.Playing
	g.syncPhase()
	if g.tab != 1 {
		t.Fatal("new game did not select comments")
	}
}

// TestCPUPassFollowsBoard は双方の色・連続パス・待った・次局で通知が棋譜に追従することを検証する。
func TestCPUPassFollowsBoard(t *testing.T) {
	for _, player := range []app.Color{app.Black, app.White} {
		b, err := app.NewBoard(9, app.Japanese)
		if err != nil {
			t.Fatal(err)
		}
		if cpuPassed(b, player) {
			t.Fatal("empty game has pass notice")
		}
		if player == app.Black {
			if err := b.Play(0); err != nil {
				t.Fatal(err)
			}
		}
		if err := b.Play(-1); err != nil {
			t.Fatal(err)
		}
		if !cpuPassed(b, player) {
			t.Fatal("CPU pass was not announced")
		}
		if err := b.Play(-1); err != nil {
			t.Fatal(err)
		}
		if cpuPassed(b, player) {
			t.Fatal("player pass was announced as CPU pass")
		}
		if !b.Undo(2) || cpuPassed(b, player) {
			t.Fatal("undo left stale pass notice")
		}
	}
}
