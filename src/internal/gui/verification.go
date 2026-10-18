// 作成日: 2026-10-18。専用フラグで画面確認と実CPUの受入測定を行う。ユーザーのプロフィールは読まない。
package gui

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"time"

	"github.com/Tremendous1192/PlayGo/engine"
	"github.com/Tremendous1192/PlayGo/internal/app"
	"github.com/hajimehoshi/ebiten/v2"
)

type acceptanceCase struct {
	Rule         app.Rule `json:"rule"`
	Size         int      `json:"size"`
	Difficulty   int      `json:"difficulty"`
	Position     string   `json:"position"`
	Action       string   `json:"action,omitempty"`
	Moves        int      `json:"moves"`
	Milliseconds int64    `json:"milliseconds"`
	Passed       bool     `json:"passed"`
	Error        string   `json:"error,omitempty"`
}
type verification struct {
	StartupMS     int64            `json:"startupMilliseconds"`
	Cases         []acceptanceCase `json:"cases"`
	Passed        bool             `json:"passed"`
	Method        string           `json:"method"`
	output        string
	index         int
	active        bool
	profile       app.Profile
	expectedTurn  int
	displayedTurn int
	began         time.Time
}

// newVerification は全90条件とCPU先手・プレイヤーパスの6条件を順番に測る。
func newVerification(output string) *verification {
	v := &verification{output: output, Passed: true, Method: "legal generated positions; accepted player move to completed Draw callback; excludes startup; not OS compositor latency"}
	for _, rule := range []app.Rule{app.Japanese, app.Chinese, app.Jungo} {
		for _, size := range []int{7, 9, 11, 13, 15, 19} {
			for difficulty := 0; difficulty < 5; difficulty++ {
				position := []string{"opening", "middle", "late", "middle", "opening"}[difficulty]
				v.Cases = append(v.Cases, acceptanceCase{Rule: rule, Size: size, Difficulty: difficulty, Position: position})
			}
		}
	}
	for _, rule := range []app.Rule{app.Japanese, app.Chinese, app.Jungo} {
		for _, action := range []string{"cpu-opening", "player-pass"} {
			v.Cases = append(v.Cases, acceptanceCase{Rule: rule, Size: 19, Difficulty: 1, Position: "opening", Action: action})
		}
	}
	return v
}

// fixtureBoard は固定乱数の合法着手だけで局面を作る。対局の棋力品質を評価するデータではない。
func fixtureBoard(size int, rule app.Rule, count int) (*app.Board, error) {
	b, err := app.NewBoard(size, rule)
	if err != nil {
		return nil, err
	}
	rng := rand.New(rand.NewPCG(uint64(size), uint64(count+1)))
	for i := 0; i < count; i++ {
		points := rng.Perm(size * size)
		point := -2
		for _, p := range points {
			if b.Legal(p) {
				point = p
				break
			}
		}
		if point < 0 || b.NoResult {
			break
		}
		if err = b.Play(point); err != nil {
			return nil, err
		}
		if b.NoResult {
			if !b.Undo(1) {
				return nil, errors.New("fixture undo failed")
			}
			break
		}
	}
	return b, nil
}

// updateVerification は1フレームずつ測定を進め、各CPU着手が描画されてから記録する。
func (g *window) updateVerification() error {
	v := g.verification
	if g.engineLoading {
		return nil
	}
	if g.engineError != nil {
		v.Passed = false
		return errors.Join(g.saveVerification(), g.engineError)
	}
	if g.controller == nil {
		return errors.New("verification controller unavailable")
	}
	c := g.controller
	if v.profile.ID == "" {
		p, err := g.store.Create("受入試験")
		if err != nil {
			return err
		}
		v.profile = p
	}
	if v.index >= len(v.Cases) {
		if err := g.saveVerification(); err != nil {
			return err
		}
		if !v.Passed {
			return errors.New("CPU-to-render acceptance failed")
		}
		return ebiten.Termination
	}
	current := &v.Cases[v.index]
	if !v.active {
		s := app.Settings{Rule: current.Rule, Size: current.Size, Difficulty: current.Difficulty, Side: 0}
		if current.Action == "cpu-opening" {
			s.Side = 1
			v.began = time.Now()
		}
		if err := c.Start(v.profile, s); err != nil {
			return err
		}
		v.profile = c.Profile
		if current.Action == "cpu-opening" {
			v.expectedTurn = 1
			v.displayedTurn = -1
			v.active = true
			return nil
		}
		c.Abort()
		count := 0
		if current.Position == "middle" {
			count = current.Size * current.Size / 3
		}
		if current.Position == "late" {
			count = current.Size * current.Size * 3 / 4
		}
		b, err := fixtureBoard(current.Size, current.Rule, count)
		if err != nil {
			return err
		}
		c.Board = b
		c.Phase = app.Playing
		c.Player = b.ToMove
		c.Evaluations = make([]*engine.Analysis, len(b.Moves)+1)
		point := -2
		for p := range b.Cells {
			if b.Legal(p) {
				point = p
				break
			}
		}
		if point < 0 || current.Action == "player-pass" {
			point = -1
		}
		current.Moves = len(b.Moves)
		v.expectedTurn = len(b.Moves) + 2
		v.displayedTurn = -1
		v.began = time.Now()
		if err = c.Play(point); err != nil {
			return err
		}
		v.active = true
		return nil
	}
	if c.Err != nil || time.Since(v.began) > 8*time.Second {
		current.Error = "request failed or exceeded watchdog"
		if c.Err != nil {
			current.Error = c.Err.Error()
		}
		current.Milliseconds = time.Since(v.began).Milliseconds()
		v.Passed = false
	} else if len(c.Board.Moves) >= v.expectedTurn && v.displayedTurn >= v.expectedTurn {
		current.Milliseconds = c.LastCPUTime.Milliseconds()
		if current.Action == "cpu-opening" {
			// 開始時の設定保存も含め、計測開始が早い側を採る。
			current.Milliseconds = max(current.Milliseconds, time.Since(v.began).Milliseconds())
		}
		current.Passed = current.Milliseconds <= 5000 && c.LastCPUTime <= 5*time.Second && !c.CPUDeadlineMiss
		if !current.Passed {
			v.Passed = false
			current.Error = "display deadline exceeded"
		}
	} else {
		return nil
	}
	fmt.Printf("acceptance %d/%d %s %dx%d level=%d %s %dms passed=%v\n", v.index+1, len(v.Cases), current.Rule, current.Size, current.Size, current.Difficulty, current.Position, current.Milliseconds, current.Passed)
	c.Abort()
	v.index++
	v.active = false
	if !current.Passed {
		return errors.Join(g.saveVerification(), fmt.Errorf("acceptance failed: %s", current.Error))
	}
	return nil
}

// saveVerification は個人情報を含まない結果だけを書き出す。
func (g *window) saveVerification() error {
	data, err := json.MarshalIndent(g.verification, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(g.verification.output, append(data, '\n'), 0600)
}

// preparePreview は画面確認専用の合成例を作る。実測や実対局の結果には使用しない。
func (g *window) preparePreview(mode string) error {
	b, err := fixtureBoard(9, app.Japanese, 32)
	if err != nil {
		return err
	}
	c := &app.Controller{Phase: app.Playing, Board: b, Player: app.Black, Settings: app.DefaultSettings(), Profile: app.Profile{Name: "画面確認用サンプル", Ranks: [3]float64{0, 0, 0}}}
	g.controller = c
	g.selected = 0
	g.profiles = []app.Profile{c.Profile}
	g.notice = "画面確認用の合成例です。戦績は保存しません。"
	if mode == "startup" {
		c.Phase = app.Idle
		return nil
	}
	g.notice = ""
	if mode != "game" {
		if err = b.Play(-1); err != nil {
			return err
		}
		if err = b.Play(-1); err != nil {
			return err
		}
	}
	c.Evaluations = make([]*engine.Analysis, len(b.Moves)+1)
	for i := range c.Evaluations {
		board, err := app.Replay(b.Size, b.Rule, b.Moves, i)
		if err != nil {
			return err
		}
		a := &engine.Analysis{ScoreLead: 6 * math.Sin(float64(i)/7), Winrate: .5 + .2*math.Sin(float64(i)/7), Ownership: make([]float64, len(b.Cells)), Moves: []engine.Candidate{{Point: 40}}}
		for p, color := range board.Cells {
			if color == app.Black {
				a.Ownership[p] = .9
			} else if color == app.White {
				a.Ownership[p] = -.9
			} else {
				a.Ownership[p] = float64(4-p%9) / 5
			}
		}
		c.Evaluations[i] = a
	}
	score, err := app.ScoreBoard(b, nil, nil)
	if err != nil {
		return err
	}
	c.Score = score
	switch mode {
	case "game":
	case "scoring":
		c.Phase = app.Scoring
	case "result":
		c.Phase = app.Finished
		g.shownPhase = app.Finished
		g.animationStart = time.Now().Add(-2 * time.Second)
	case "review":
		c.Phase = app.Review
		g.reviewTurn = 12
		g.reviewBoard, err = c.Snapshot(12)
		g.tab = 2
	default:
		return errors.New("unknown preview mode")
	}
	return err
}
