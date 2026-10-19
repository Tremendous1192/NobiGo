// 作成日: 2026-10-19。概要の合成表示例と実AI受入測定。合成例は保存せず、実測は専用の一時プロフィールだけを使う。
package gui

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Tremendous1192/PlayGo/engine"
	"github.com/Tremendous1192/PlayGo/internal/app"
)

// prepareSummaryPreview は4ページ・全路数・白視点・純碁・解析失敗・長文を明示した合成データで作る。
func (g *window) prepareSummaryPreview(mode string) error {
	page, ok := map[string]int{"summary-1": 0, "summary-2": 1, "summary-3": 2, "summary-4": 3, "summary-wait": 0, "summary-error": 3, "summary-missing": 2, "summary-white": 2, "summary-jungo": 0, "summary-draw": 0, "summary-empty": 0, "summary-noresult": 0, "summary-long": 1, "summary-bottom": 1, "summary-min": 3, "summary-7": 3, "summary-9": 3, "summary-11": 3, "summary-13": 3, "summary-15": 3, "summary-19": 3}[mode]
	if !ok {
		return errors.New("unknown summary preview mode")
	}
	size := 9
	if n := map[string]int{"summary-7": 7, "summary-9": 9, "summary-11": 11, "summary-13": 13, "summary-15": 15, "summary-19": 19}[mode]; n > 0 {
		size = n
	}
	rule := app.Japanese
	if mode == "summary-jungo" || mode == "summary-draw" {
		rule = app.Jungo
	}
	b, err := app.NewBoard(size, rule)
	if err != nil {
		return err
	}
	points := []int{size*2 + 2, 0, size*3 + 3, 1, size*4 + 4, 2, -1, -1}
	if mode == "summary-draw" {
		points = []int{-1, -1}
	} else if mode == "summary-empty" {
		points = nil
	}
	for _, point := range points {
		if err = b.Play(point); err != nil {
			return err
		}
	}
	c := &app.Controller{Phase: app.Summary, Board: b, Player: app.Black, Settings: app.DefaultSettings(), Evaluations: make([]*engine.Analysis, len(points)+1)}
	c.Settings.Rule, c.Settings.Size = rule, size
	if mode == "summary-white" {
		c.Player = app.White
	}
	for i := range c.Evaluations {
		value := []float64{0, 0, 0, -4, -2, -2, -1, -1, -1}[i]
		c.Evaluations[i] = &engine.Analysis{ScoreLead: value, Winrate: .5 + value/100, Moves: []engine.Candidate{{Point: size*size - 9, Visits: 32}}}
	}
	if len(points) > 2 {
		c.Evaluations[0].Moves[0].Point = points[0]
		c.Evaluations[1].Moves[0].Point = points[1]
	}
	if mode == "summary-missing" || mode == "summary-wait" {
		c.Evaluations = make([]*engine.Analysis, len(points)+1)
	}
	data, err := app.BuildGameSummary(b, c.Player, c.Evaluations)
	if err != nil {
		return err
	}
	c.SummarySession = &app.SummarySession{Data: data, Ready: mode != "summary-wait"}
	if mode == "summary-wait" {
		c.SummarySession.Total = 8
		c.SummarySession.Completed = 2
	}
	if mode == "summary-error" {
		c.SummarySession.Err = errors.New("synthetic summary failure")
	}
	c.Score, err = app.ScoreBoard(b, nil, nil)
	if err != nil {
		return err
	}
	if mode == "summary-noresult" {
		c.Score.NoResult = true
	}
	g.controller = c
	g.shownPhase = app.Summary
	g.autoplay = false
	g.animationStart = time.Now().Add(-2 * time.Second)
	g.summary = newSummaryDialog(nil, nil)
	g.summary.page = page
	// 合成表示では状態遷移も実解析も実行せず、ボタンの有効時の外観だけを確認する。
	g.summary.close = func() {}
	g.summary.retry = func() {}
	g.previewMinimum = mode == "summary-min"
	if mode == "summary-long" || mode == "summary-bottom" {
		g.refreshSummaryPage(g.summary)
		d := g.summary
		d.body = strings.Repeat(d.body+"\n\n", 4) + "長文の末尾まで確認できました。"
		d.lines = g.wrapLines(d.body, 510, 20)
		d.maxScroll = math.Max(0, float64(len(d.lines)*30-450))
		if mode == "summary-bottom" {
			d.scroll = d.maxScroll
		}
	}
	return nil
}

// summaryPreviewModes は配布検査にも記載する固定の画面例を返す。
func summaryPreviewModes() []string {
	modes := []string{"summary-1", "summary-2", "summary-3", "summary-4", "summary-wait", "summary-error", "summary-missing", "summary-white", "summary-jungo", "summary-draw", "summary-empty", "summary-noresult", "summary-long", "summary-bottom", "summary-min"}
	for _, size := range []int{7, 9, 11, 13, 15, 19} {
		modes = append(modes, fmt.Sprintf("summary-%d", size))
	}
	return modes
}

// summaryAcceptanceCase は実CPUの概要生成と、その直後のCPU先手の描画時間を記録する。
type summaryAcceptanceCase struct {
	Rule                app.Rule `json:"rule"`
	Size                int      `json:"size"`
	Positions           int      `json:"positions"`
	EvaluatedMoves      int      `json:"evaluatedMoves"`
	Milliseconds        int64    `json:"milliseconds"`
	NextCPUMilliseconds int64    `json:"nextCPUMilliseconds"`
	Passed              bool     `json:"passed"`
	Error               string   `json:"error,omitempty"`
}

// updateSummaryVerification は専用プロフィールで概要を表示し、終了直後の次局をDrawまで測る。
func (g *window) updateSummaryVerification() error {
	v, c := g.verification, g.controller
	current := &v.SummaryCases[v.summaryIndex]
	fail := func(message string) error {
		current.Error = message
		current.Passed = false
		v.Passed = false
		return errors.Join(g.saveVerification(), errors.New(message))
	}
	switch v.summaryStage {
	case 0:
		settings := app.Settings{Rule: current.Rule, Size: current.Size, Difficulty: 1, Side: 0}
		if err := c.Start(v.profile, settings); err != nil {
			return err
		}
		v.profile = c.Profile
		c.Abort()
		b, err := fixtureBoard(current.Size, current.Rule, current.Size*2)
		if err != nil {
			return err
		}
		if err = b.Play(-1); err != nil {
			return err
		}
		if err = b.Play(-1); err != nil {
			return err
		}
		c.Board = b
		c.Phase = app.Finished
		c.Evaluations = make([]*engine.Analysis, len(b.Moves)+1)
		c.Score, err = app.ScoreBoard(b, nil, nil)
		if err != nil {
			return err
		}
		v.summaryDrawn = false
		v.summaryBegan = time.Now()
		if err = g.openSummary(); err != nil {
			return err
		}
		v.summaryStage = 1
	case 1:
		s := c.SummarySession
		if time.Since(v.summaryBegan) > app.SummaryTimeLimit+2*time.Second {
			return fail("summary watchdog exceeded")
		}
		if !s.Ready || !v.summaryDrawn {
			return nil
		}
		current.Milliseconds = time.Since(v.summaryBegan).Milliseconds()
		current.Positions = s.Completed
		current.EvaluatedMoves = s.Data.Evaluated
		if s.Err != nil || s.Data.Evaluated == 0 || s.Completed != s.Total || s.Total > app.MaxSummaryPositions || current.Milliseconds > app.SummaryTimeLimit.Milliseconds() {
			return fail("summary analysis acceptance failed")
		}
		g.finishSummary()
		c.Settings.Side = 1
		v.summaryBegan = time.Now()
		v.displayedTurn = -1
		v.expectedTurn = 1
		g.chooseDialog(2)
		if c.Phase != app.Playing {
			return fail("next game after summary failed")
		}
		v.profile = c.Profile
		v.summaryStage = 2
	case 2:
		if c.Err != nil || time.Since(v.summaryBegan) > 8*time.Second {
			return fail("next CPU after summary failed")
		}
		if len(c.Board.Moves) < 1 || v.displayedTurn < 1 {
			return nil
		}
		current.NextCPUMilliseconds = max(c.LastCPUTime.Milliseconds(), time.Since(v.summaryBegan).Milliseconds())
		current.Passed = current.NextCPUMilliseconds <= app.CPUResponseLimit.Milliseconds() && !c.CPUDeadlineMiss
		if !current.Passed {
			return fail("next CPU after summary exceeded deadline")
		}
		fmt.Printf("summary %d/%d %s %dx%d positions=%d evaluated=%d summary=%dms nextCPU=%dms passed=true\n", v.summaryIndex+1, len(v.SummaryCases), current.Rule, current.Size, current.Size, current.Positions, current.EvaluatedMoves, current.Milliseconds, current.NextCPUMilliseconds)
		c.Abort()
		v.summaryIndex++
		v.summaryStage = 0
	}
	return nil
}
