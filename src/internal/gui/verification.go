// 作成日: 2026-10-18。専用フラグで画面確認と実CPUの受入測定を行う。ユーザーのプロフィールは読まない。
package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	"github.com/Tremendous1192/NobiGo/engine"
	"github.com/Tremendous1192/NobiGo/internal/app"
	"github.com/hajimehoshi/ebiten/v2"
)

type acceptanceCase struct {
	Rule                           app.Rule `json:"rule"`
	Size                           int      `json:"size"`
	Difficulty                     int      `json:"difficulty"`
	Position                       string   `json:"position"`
	Action                         string   `json:"action,omitempty"`
	Moves                          int      `json:"moves"`
	Milliseconds                   int64    `json:"milliseconds"`
	Passed                         bool     `json:"passed"`
	Error                          string   `json:"error,omitempty"`
	OwnershipMilliseconds          int64    `json:"ownershipMilliseconds,omitempty"`
	OwnershipStatus                string   `json:"ownershipStatus,omitempty"`
	BeforeCPUOwnershipMilliseconds int64    `json:"beforeCPUOwnershipMilliseconds,omitempty"`
	BeforeCPUOwnershipStatus       string   `json:"beforeCPUOwnershipStatus,omitempty"`
}

// 2026-10-18の96条件の最大3529msに25%の余裕を加え、500ms単位で切り上げた検証上限。
const ownershipLimitMilliseconds int64 = 4500

type verification struct {
	SummaryCases               []summaryAcceptanceCase `json:"summaryCases"`
	StartupMS                  int64                   `json:"startupMilliseconds"`
	ModelPreparationMS         int64                   `json:"modelPreparationMilliseconds"`
	EngineLoadMS               int64                   `json:"engineLoadMilliseconds"`
	Cases                      []acceptanceCase        `json:"cases"`
	Passed                     bool                    `json:"passed"`
	Method                     string                  `json:"method"`
	OwnershipLimitMS           int64                   `json:"ownershipLimitMilliseconds"`
	output                     string
	index                      int
	active                     bool
	profile                    app.Profile
	expectedTurn               int
	displayedTurn              int
	began                      time.Time
	cpuComplete                bool
	positionShownAt            time.Time
	ownershipShown             bool
	ownershipMS                int64
	beforeCPUOwnershipShown    bool
	beforeCPUOwnershipMS       int64
	summaryIndex, summaryStage int
	summaryDrawn               bool
	summaryBegan               time.Time
}

// newVerification は全90条件とCPU先手・プレイヤーパスの6条件を順番に測る。
func newVerification(output string) *verification {
	v := &verification{output: output, Passed: true, OwnershipLimitMS: ownershipLimitMilliseconds, Method: "CPU: accepted player action to Draw completion; beforeCPUOwnership: accepted action to first valid ownership Draw; ownership: CPU-result board first Draw to first valid ownership Draw; final scoring excluded; startup separate; not OS compositor latency"}
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
		for _, size := range []int{7, 19} {
			v.SummaryCases = append(v.SummaryCases, summaryAcceptanceCase{Rule: rule, Size: size})
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
		if v.summaryIndex < len(v.SummaryCases) {
			return g.updateSummaryVerification()
		}
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
		v.cpuComplete = false
		v.beforeCPUOwnershipShown = false
		v.beforeCPUOwnershipMS = 0
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
	if c.Err != nil || !v.cpuComplete && time.Since(v.began) > 8*time.Second {
		current.Error = "request failed or exceeded watchdog"
		if c.Err != nil {
			current.Error = c.Err.Error()
		}
		if !v.cpuComplete {
			current.Milliseconds = time.Since(v.began).Milliseconds()
		}
		current.Passed = false
		v.Passed = false
	} else if !v.cpuComplete && len(c.Board.Moves) >= v.expectedTurn && v.displayedTurn >= v.expectedTurn {
		current.Milliseconds = c.LastCPUTime.Milliseconds()
		if current.Action == "cpu-opening" {
			// 開始時の設定保存も含め、計測開始が早い側を採る。
			current.Milliseconds = max(current.Milliseconds, time.Since(v.began).Milliseconds())
		}
		current.Passed = current.Milliseconds <= app.CPUResponseLimit.Milliseconds() && c.LastCPUTime <= app.CPUResponseLimit && !c.CPUDeadlineMiss
		if !current.Passed {
			v.Passed = false
			current.Error = "display deadline exceeded"
		}
		v.cpuComplete = true
	} else if !v.cpuComplete {
		return nil
	}
	if current.Passed {
		if c.Phase == app.Scoring {
			current.OwnershipStatus = "Not applicable: final scoring"
		} else if v.ownershipShown {
			current.OwnershipMilliseconds = v.ownershipMS
			current.OwnershipStatus = "Measured"
		} else if !v.positionShownAt.IsZero() && time.Since(v.positionShownAt) < 8*time.Second {
			return nil
		} else {
			current.Passed, v.Passed = false, false
			current.Error = "ownership display missing"
		}
	}
	current.BeforeCPUOwnershipStatus = "Not observed: CPU move prioritized"
	if v.beforeCPUOwnershipShown {
		current.BeforeCPUOwnershipStatus = "Measured"
		current.BeforeCPUOwnershipMilliseconds = v.beforeCPUOwnershipMS
	}
	if current.Passed && v.OwnershipLimitMS > 0 && (current.OwnershipMilliseconds > v.OwnershipLimitMS || current.BeforeCPUOwnershipMilliseconds > v.OwnershipLimitMS) {
		current.Passed, v.Passed = false, false
		current.Error = "ownership display deadline exceeded"
	}
	fmt.Printf("acceptance %d/%d %s %dx%d level=%d %s CPU=%dms ownership=%dms passed=%v\n", v.index+1, len(v.Cases), current.Rule, current.Size, current.Size, current.Difficulty, current.Position, current.Milliseconds, current.OwnershipMilliseconds, current.Passed)
	c.Abort()
	v.index++
	v.active = false
	if !current.Passed {
		return errors.Join(g.saveVerification(), fmt.Errorf("acceptance failed: %s", current.Error))
	}
	return nil
}

// recordVerificationDraw は現在局面が描かれた時刻と、最初の有効な色分けの描画時刻だけを記録する。
func (g *window) recordVerificationDraw(now time.Time) {
	v := g.verification
	if g.summary != nil && g.controller.SummarySession.Ready {
		v.summaryDrawn = true
	}
	turn := len(g.controller.Board.Moves)
	if v.displayedTurn != turn {
		v.displayedTurn = turn
		v.positionShownAt = now
		v.ownershipShown = false
		v.ownershipMS = 0
	}
	if a := g.currentAnalysis(turn); !v.ownershipShown && a != nil && len(a.Ownership) == len(g.controller.Board.Cells) {
		v.ownershipShown = true
		v.ownershipMS = now.Sub(v.positionShownAt).Milliseconds()
		if turn == v.expectedTurn-1 && !v.beforeCPUOwnershipShown {
			v.beforeCPUOwnershipShown = true
			v.beforeCPUOwnershipMS = now.Sub(v.began).Milliseconds()
		}
	}
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
	if strings.HasPrefix(mode, "summary-") {
		return g.prepareSummaryPreview(mode)
	}
	if mode == "review-ai" || mode == "review-ai-19" || mode == "review-detail-ai" || mode == "review-detail-ai-risk" {
		return g.prepareAIPreview(mode)
	}
	if mode == "review-focus-moved" || mode == "review-focus-before" {
		detail := "review-detail-19"
		if mode == "review-focus-before" {
			detail = "review-detail-before"
		}
		if err := g.prepareAdvicePreview(detail); err != nil {
			return err
		}
		if mode == "review-focus-moved" {
			g.advice.diagram.pan(-4, 2)
		}
		g.dismiss()
		return nil
	}
	if strings.HasPrefix(mode, "review-detail") {
		return g.prepareAdvicePreview(mode)
	}
	size, moves := 9, 32
	if mode == "review-19" {
		size, moves = 19, 220
	}
	if mode == "game-pass" {
		moves = 31
	}
	rule := app.Japanese
	if mode == "game-white-jungo" {
		rule = app.Jungo
	}
	b, err := fixtureBoard(size, rule, moves)
	if err != nil {
		return err
	}
	c := &app.Controller{Phase: app.Playing, Board: b, Player: app.Black, Settings: app.DefaultSettings(), Profile: app.Profile{Name: "画面確認用サンプル", Ranks: [3]float64{0, 0, 0}}}
	g.controller = c
	c.Settings.Rule, c.Settings.Size = rule, size
	if mode == "game-white-jungo" {
		c.Player = app.White
	}
	g.selected = 0
	g.profiles = []app.Profile{c.Profile}
	g.notice = "画面確認用の合成例です。戦績は保存しません。"
	if mode == "startup" || strings.HasPrefix(mode, "startup-") {
		c.Phase = app.Idle
		switch mode {
		case "startup":
		case "startup-check":
			g.engineLoading, g.preparationStage = true, preparationChecking
		case "startup-download":
			g.engineLoading, g.preparationStage = true, preparationDownloading
		case "startup-verify":
			g.engineLoading, g.preparationStage = true, preparationVerifying
		case "startup-load":
			g.engineLoading, g.preparationStage = true, preparationLoading
		case "startup-error":
			g.engineError = errors.New("download failed")
		case "startup-repair":
			g.engineError = engine.ErrModelInvalid
		case "startup-cancel":
			g.engineError = context.Canceled
		default:
			return errors.New("unknown startup preview")
		}
		return nil
	}
	g.notice = ""
	if mode == "game-pass" {
		if err = b.Play(-1); err != nil {
			return err
		}
	}
	if !strings.HasPrefix(mode, "game") {
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
	case "game-white-jungo":
	case "game-extremes":
		c.Evaluations[len(b.Moves)-1].ScoreLead = -20000
		c.Evaluations[len(b.Moves)].ScoreLead = 20000
		c.Evaluations[len(b.Moves)].Winrate = 1
	case "game-gap":
		for i := 8; i <= 12; i++ {
			c.Evaluations[i] = nil
		}
	case "game-pass":
	case "game-wait":
		c.Evaluations[len(b.Moves)] = nil
	case "scoring":
		c.Phase = app.Scoring
	case "result", "choices":
		c.Phase = app.Finished
		g.shownPhase = app.Finished
		g.animationStart = time.Now().Add(-2 * time.Second)
		if mode == "choices" {
			g.openResultChoices()
		}
	case "review", "review-19", "review-player":
		c.Phase = app.Review
		g.reviewTurn = 12
		if mode == "review-19" {
			g.reviewTurn = 200
		}
		if mode == "review-player" {
			g.reviewTurn = 13
		}
		g.reviewBoard, err = c.Snapshot(g.reviewTurn)
		if err == nil {
			g.reviewBefore, err = c.Snapshot(g.reviewTurn - 1)
		}
		if err == nil && mode == "review-player" {
			c.Evaluations[12].ScoreLead, c.Evaluations[13].ScoreLead = 8, 2
			for point := range g.reviewBefore.Cells {
				if point != c.Board.Moves[12].Point && g.reviewBefore.Legal(point) {
					c.Evaluations[12].Moves = []engine.Candidate{{Point: point}}
					break
				}
			}
		}
		g.tab = 2
	default:
		return errors.New("unknown preview mode")
	}
	return err
}

// prepareAIPreview は合法棋譜と明示した合成読み筋で表示を確認し、実AIの測定値には使わない。
func (g *window) prepareAIPreview(mode string) error {
	size := 7
	if mode == "review-ai-19" {
		size = 19
	}
	b, err := app.NewBoard(size, app.Japanese)
	if err != nil {
		return err
	}
	for _, point := range []int{size + 1, 3*size + 1, size + 3} {
		if err = b.Play(point); err != nil {
			return err
		}
	}
	g.reviewBoard = b
	g.reviewBefore, err = app.Replay(size, b.Rule, b.Moves, 2)
	if err != nil {
		return err
	}
	g.reviewTurn = 3
	g.controller = &app.Controller{Phase: app.Review, Board: b, Player: app.Black, Settings: app.DefaultSettings(), Evaluations: make([]*engine.Analysis, 4)}
	g.controller.Settings.Size = size
	a := &engine.Analysis{LocalRead: true, Ownership: make([]float64, size*size), Winrate: .5}
	for point, color := range b.Cells {
		if color == app.Black {
			a.Ownership[point] = .9
		}
		if color == app.White {
			a.Ownership[point] = -.9
		}
	}
	var pvs = [][]int{{size + 2, 2*size + 1, 0, 2*size + 2, 1, 2*size + 3}, {0, 2*size + 1, size + 2, 2*size + 2, 1, 2*size + 3}}
	if mode == "review-detail-ai-risk" {
		pvs = [][]int{{size + 2, 2*size + 1}}
		a.Ownership[size+3] = -.9
	}
	for _, pv := range pvs {
		visits := make([]int, len(pv))
		for i := range visits {
			visits[i] = 4
		}
		a.Moves = append(a.Moves, engine.Candidate{Point: pv[0], Visits: 8, PV: pv, PVVisits: visits, Ownership: append([]float64(nil), a.Ownership...)})
	}
	g.controller.Evaluations[3] = a
	g.tab = 2
	g.syncPhase()
	if strings.HasPrefix(mode, "review-detail") {
		g.openReviewAdvice()
	}
	return nil
}

// prepareAdvicePreview は詳細・複合効果・待機・長文の描画例を作り、実プロフィールやエンジンへ接続しない。
func (g *window) prepareAdvicePreview(mode string) error {
	switch mode {
	case "review-detail", "review-detail-cpu", "review-detail-combined", "review-detail-wait", "review-detail-error", "review-detail-long", "review-detail-bottom", "review-detail-min",
		"review-detail-before", "review-detail-outside", "review-detail-pass", "review-detail-empty",
		"review-detail-7", "review-detail-9", "review-detail-11", "review-detail-13", "review-detail-15", "review-detail-19":
	default:
		return errors.New("unknown advice preview mode")
	}
	base := "review-player"
	if mode == "review-detail-cpu" {
		base = "review"
	}
	if err := g.preparePreview(base); err != nil {
		return err
	}
	c := g.controller
	if mode == "review-detail-combined" || mode == "review-detail-before" {
		b, err := app.NewBoard(7, app.Japanese)
		if err != nil {
			return err
		}
		for _, point := range []int{2, 1, 8, 7, 0, -1, -1} {
			if err := b.Play(point); err != nil {
				return err
			}
		}
		c.Board = b
		c.Evaluations = make([]*engine.Analysis, len(b.Moves)+1)
		g.reviewTurn = 5
		g.reviewBoard, err = c.Snapshot(5)
		if err != nil {
			return err
		}
		g.reviewBefore, err = c.Snapshot(4)
		if err != nil {
			return err
		}
	}
	if size := map[string]int{"review-detail-7": 7, "review-detail-9": 9, "review-detail-11": 11, "review-detail-13": 13, "review-detail-15": 15, "review-detail-19": 19}[mode]; size > 0 {
		b, err := fixtureBoard(size, app.Japanese, size*2)
		if err != nil {
			return err
		}
		c.Board = b
		c.Evaluations = make([]*engine.Analysis, len(b.Moves)+1)
		g.reviewTurn = len(b.Moves) - 1
		g.reviewBoard, err = c.Snapshot(g.reviewTurn)
		if err != nil {
			return err
		}
		g.reviewBefore, err = c.Snapshot(g.reviewTurn - 1)
		if err != nil {
			return err
		}
	}
	if mode == "review-detail-outside" {
		b, err := app.NewBoard(19, app.Japanese)
		if err != nil {
			return err
		}
		for p := 0; p < 19; p++ {
			if err = b.Play(p); err != nil {
				return err
			}
			if err = b.Play(342 + p); err != nil {
				return err
			}
		}
		c.Board = b
		c.Evaluations = make([]*engine.Analysis, len(b.Moves)+1)
		g.reviewTurn = 37
		g.reviewBoard, err = c.Snapshot(37)
		if err != nil {
			return err
		}
		g.reviewBefore, err = c.Snapshot(36)
		if err != nil {
			return err
		}
	}
	if mode == "review-detail-pass" || mode == "review-detail-empty" {
		g.reviewTurn = len(c.Board.Moves)
		if mode == "review-detail-empty" {
			g.reviewTurn = 0
		}
		var err error
		g.reviewBoard, err = c.Snapshot(g.reviewTurn)
		if err != nil {
			return err
		}
		g.reviewBefore = nil
		if g.reviewTurn > 0 {
			g.reviewBefore, err = c.Snapshot(g.reviewTurn - 1)
			if err != nil {
				return err
			}
		}
	}
	if mode == "review-detail-wait" || mode == "review-detail-error" {
		c.Evaluations = make([]*engine.Analysis, len(c.Board.Moves)+1)
		c.Busy = mode == "review-detail-wait"
		if mode == "review-detail-error" {
			c.Err = errors.New("synthetic analysis failure")
		}
	}
	g.syncPhase()
	g.openReviewAdvice()
	if mode == "review-detail-before" {
		g.advice.diagram.selectTarget(1)
		g.advice.diagram.showBefore = true
	}
	if mode == "review-detail-long" || mode == "review-detail-bottom" {
		body := g.advice.body
		for i := 1; i <= 6; i++ {
			g.advice.body += fmt.Sprintf("\n\n長文表示の検証 %d\n%s", i, body)
		}
		g.advice.body += "\n長文表示の末尾まで確認できました。"
		if mode == "review-detail-bottom" {
			g.advice.scroll = math.MaxFloat64
		}
	}
	g.previewMinimum = mode == "review-detail-min"
	return nil
}
