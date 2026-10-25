// 作成日: 2026-10-25。8教材の合法棋譜と失敗状態を使う画面確認。実プロフィールとエンジンには接続しない。
package gui

import (
	"context"
	"errors"
	"strings"

	"github.com/Tremendous1192/NobiGo/engine"
	"github.com/Tremendous1192/NobiGo/internal/app"
)

// prepareTechniquePreview は技術の成立を実際に検査した合成棋譜を表示する。
func (g *window) prepareTechniquePreview(id app.TechniqueID) error {
	end := strings.HasSuffix(string(id), "-end")
	id = app.TechniqueID(strings.TrimSuffix(string(id), "-end"))
	requested := id
	if id == "repeat" || id == "save-error" || id == "wait" || id == "none" {
		id = app.TechniqueCapture
	}
	black, white, point := []int{}, []int{}, 0
	switch id {
	case app.TechniqueCapture:
		black, white, point = []int{1, 7, 9}, []int{8, 44, 46}, 15
	case app.TechniqueEscape:
		black, white, point = []int{8, 45, 47}, []int{1, 7, 9}, 15
	case app.TechniqueConnect:
		black, white, point = []int{7, 9}, []int{45, 47}, 8
	case app.TechniqueAtari:
		black, white, point = []int{1, 7}, []int{8, 47}, 9
	case app.TechniqueDoubleAtari:
		black, white, point = []int{1, 7, 3, 11}, []int{8, 10, 45, 47}, 9
	case app.TechniqueLadder:
		black, white, point = []int{1, 7, 14}, []int{8, 3, 4}, 9
	case app.TechniqueNet:
		black, white, point = []int{1, 2, 7, 10, 14, 22}, []int{8, 45, 47, 43, 41, 39}, 16
	case app.TechniqueSnapback:
		black, white, point = []int{2, 9, 14, 15}, []int{1, 8, 48, 47}, 0
	default:
		return errors.New("unknown technique preview")
	}
	b, err := app.NewBoard(7, app.Japanese)
	if err != nil {
		return err
	}
	for i, p := range black {
		if err = b.Play(p); err != nil {
			return err
		}
		if err = b.Play(white[i]); err != nil {
			return err
		}
	}
	turn := len(b.Moves) + 1
	lessons := app.BasicTechniques(b, point, turn)
	lessons = append(lessons, app.ReadTechniques(context.Background(), b, point, turn)...)
	var chosen *app.TechniqueLesson
	for _, m := range lessons {
		if m.ID == id {
			copy := m
			chosen = &copy
			break
		}
	}
	if chosen == nil {
		return errors.New("technique preview lacks evidence")
	}
	for _, p := range []int{point, -1, -1} {
		if err = b.Play(p); err != nil {
			return err
		}
	}
	c := &app.Controller{Phase: app.Summary, Board: b, Player: app.Black, Settings: app.Settings{Rule: app.Japanese, Size: 7}, Evaluations: make([]*engine.Analysis, len(b.Moves)+1)}
	data, err := app.BuildGameSummary(b, c.Player, c.Evaluations)
	if err != nil {
		return err
	}
	c.SummarySession = &app.SummarySession{Data: data, Ready: true, Lessons: []app.TechniqueLesson{*chosen}}
	if requested == "none" {
		c.SummarySession.Lessons = nil
	}
	if requested == "wait" {
		c.SummarySession.Ready = false
	}
	if requested == "repeat" {
		c.Profile.Techniques = []app.TechniqueRecord{{ID: id, Count: 1, LastShown: 1}}
	}
	g.controller = c
	g.shownPhase = app.Summary
	g.summary = newSummaryDialog(func() {}, func() {})
	g.summary.page = 3
	if end {
		g.summary.techniqueStep = len(chosen.Line)
	}
	g.summary.saveFailed = requested == "save-error"
	return nil
}
