// 作成日: 2026-10-18。盤面の交点と画面外の対応を検証する。
package gui

import (
	"github.com/Tremendous1192/PlayGo/internal/app"
	"math"
	"strings"
	"testing"
)

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
