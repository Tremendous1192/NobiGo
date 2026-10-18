// 作成日: 2026-10-18。採点画面の連単位修正と純碁の変更禁止を検証する。
package gui

import (
	"github.com/Tremendous1192/PlayGo/internal/app"
	"testing"
)

// TestScoreMarkTransitions は死石の再選択で解除され、セキとの矛盾が残らないことを検証する。
func TestScoreMarkTransitions(t *testing.T) {
	b, _ := app.NewBoard(7, app.Japanese)
	b.Cells[0] = app.Black
	b.Cells[1] = app.Black
	b.Cells[48] = app.White
	g := &window{controller: &app.Controller{Board: b}}
	g.prepareScore()
	g.toggleMark(0)
	if !g.scorePreview.DeadMask[0] || !g.scorePreview.DeadMask[1] {
		t.Fatal("group was not selected")
	}
	g.toggleMark(1)
	if g.scorePreview.DeadMask[0] || g.scorePreview.DeadMask[1] {
		t.Fatal("group did not clear")
	}
	g.markSeki = true
	g.toggleMark(0)
	g.markSeki = false
	g.toggleMark(1)
	if g.scoreError != "" || len(g.seki) != 0 {
		t.Fatal("stale seki mark", g.scoreError)
	}
}

// TestJungoCannotMarkDead は純碁の画面操作で石数が変わらないことを検証する。
func TestJungoCannotMarkDead(t *testing.T) {
	b, _ := app.NewBoard(7, app.Jungo)
	b.Cells[0] = app.Black
	g := &window{controller: &app.Controller{Board: b}}
	g.prepareScore()
	g.toggleMark(0)
	if len(g.dead) != 0 || g.scorePreview.Black != 1 {
		t.Fatal("jungo mark accepted")
	}
}
