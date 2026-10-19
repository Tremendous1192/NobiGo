// 作成日: 2026-10-18。感想戦の説明を合法な棋譜と盤面差分で検証する。外部エンジンは使用しない。
package gui

import (
	"bytes"
	"errors"
	"image"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Tremendous1192/NobiGo/engine"
	"github.com/Tremendous1192/NobiGo/internal/app"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// TestSummaryPreservesDangerWithinThreeLines は複合効果・最大石数でも危険と確認事項が3行内に残ることを検証する。
func TestSummaryPreservesDangerWithinThreeLines(t *testing.T) {
	source, err := text.NewGoTextFaceSource(bytes.NewReader(fontData))
	if err != nil {
		t.Fatal(err)
	}
	g := &window{face: &text.GoTextFace{Source: source}}
	facts := moveFacts{taken: 361, connected: 4, liberties: []int{360}, attacked: []changedGroup{{point: 0}}, rescued: []changedGroup{{point: 1}}}
	for _, actor := range []string{"あなた", "CPU"} {
		summary := summarizeMove(facts, "T19", actor, actor == "あなた", 19)
		lines := g.wrapLines(summary, 445, 16)
		if len(lines) != 3 || g.wrap(summary, 445, 16, 3) != summary {
			t.Fatalf("summary was truncated: %q", summary)
		}
		for _, want := range []string{actor, "T19", "アタリ", "石を361個取りました", "残る空き", "詳細"} {
			if !strings.Contains(summary, want) {
				t.Fatalf("summary lost %q", want)
			}
		}
		if !strings.Contains(lines[0], "アタリ") {
			t.Fatal("danger was not prioritized")
		}
	}
	c, before, after := adviceFixture(t, app.Japanese, []int{2, 1, 8, 7, 0})
	advice := reviewAdvice(c, 5, before, after)
	if len(g.wrapLines(advice.summary, 445, 16)) > 3 || !strings.Contains(advice.summary, "アタリ") || !strings.Contains(advice.detail, "すぐに打てない") {
		t.Fatal("compact summary changed detailed caution")
	}
}

// adviceFixture は最終手の前後を独立した盤面で用意する。
func adviceFixture(t *testing.T, rule app.Rule, points []int) (*app.Controller, *app.Board, *app.Board) {
	t.Helper()
	b, err := app.NewBoard(7, rule)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range points {
		if err := b.Play(p); err != nil {
			t.Fatal(err)
		}
	}
	before, err := app.Replay(b.Size, rule, b.Moves, len(b.Moves)-1)
	if err != nil {
		t.Fatal(err)
	}
	c := &app.Controller{Board: b, Player: b.Moves[len(b.Moves)-1].Color, Evaluations: make([]*engine.Analysis, len(b.Moves)+1)}
	return c, before, b
}

// TestAdviceDialogBlocksBackgroundAndOffersScrolling は詳細描画後に背面操作が消え、上下と閉じるだけが作用することを検証する。
func TestAdviceDialogBlocksBackgroundAndOffersScrolling(t *testing.T) {
	source, err := text.NewGoTextFaceSource(bytes.NewReader(fontData))
	if err != nil {
		t.Fatal(err)
	}
	backgroundActivated := false
	g := &window{face: &text.GoTextFace{Source: source}, advice: &adviceDialog{turn: 5, body: strings.Repeat("盤面の石と呼吸点を確認しましょう。\n", 30)}}
	g.actions = []hit{{image.Rect(0, 0, 1180, 800), func() { backgroundActivated = true }}}
	screen := ebiten.NewImage(1180, 800)
	g.drawAdviceDialog(screen)
	if len(g.actions) != 2 || g.advice.maxScroll <= 0 {
		t.Fatal("expected down and close actions only")
	}
	for _, h := range g.actions {
		if h.rect.Min.Y < 677 {
			t.Fatal("background action survived overlay")
		}
		if image.Pt(280, 715).In(h.rect) {
			h.action()
		}
	}
	if g.advice.scroll <= 0 || backgroundActivated {
		t.Fatal("scroll activated background")
	}
	g.drawAdviceDialog(screen)
	if len(g.actions) != 3 {
		t.Fatal("up action missing after scroll")
	}
	for _, h := range g.actions {
		if image.Pt(1000, 720).In(h.rect) {
			h.action()
		}
	}
	if g.advice != nil || g.autoplay || backgroundActivated {
		t.Fatal("close changed background or resumed playback")
	}
}

// TestAdviceDialogStopsPlaybackAndKeepsSnapshot は閲覧中の手と本文が固定され、閉じても再生が再開しないことを検証する。
func TestAdviceDialogStopsPlaybackAndKeepsSnapshot(t *testing.T) {
	c, before, after := adviceFixture(t, app.Japanese, []int{2, 1, 8, 7, 0})
	c.Phase = app.Review
	c.Busy = true
	g := &window{controller: c, reviewTurn: 5, reviewBefore: before, reviewBoard: after, autoplay: true, shownPhase: app.Review}
	g.openReviewAdvice()
	if g.advice == nil || g.autoplay || g.advice.turn != 5 || g.advice.status == "" {
		t.Fatal("detail did not pause current move")
	}
	body := g.advice.body
	c.Busy = false
	c.Evaluations[4] = &engine.Analysis{ScoreLead: 8, Moves: []engine.Candidate{{Point: 3}}}
	c.Evaluations[5] = &engine.Analysis{ScoreLead: 1}
	g.advanceReview(time.Unix(100, 0))
	if g.reviewTurn != 5 || g.advice.body != body {
		t.Fatal("detail changed while reading")
	}
	g.dismiss()
	g.advanceReview(time.Unix(101, 0))
	if g.advice != nil || g.autoplay || g.reviewTurn != 5 {
		t.Fatal("closing detail resumed playback")
	}
	g.openReviewAdvice()
	if !strings.Contains(g.advice.body, "比較候補") || g.advice.scroll != 0 {
		t.Fatal("reopening did not refresh analysis and scroll")
	}
	c.Phase = app.Finished
	g.syncPhase()
	if g.advice != nil {
		t.Fatal("detail survived phase change")
	}
}

// TestAdviceScrollBounds は短文・長文の上下端と小数ホイール量、局面移動時の状態解除を検証する。
func TestAdviceScrollBounds(t *testing.T) {
	g := &window{advice: &adviceDialog{maxScroll: 100}}
	g.scrollAdvice(31.5)
	if g.advice.scroll != 31.5 {
		t.Fatal("fractional scrolling was lost")
	}
	g.scrollAdvice(1000)
	if g.advice.scroll != 100 {
		t.Fatal("scrolled past end")
	}
	g.scrollAdvice(-1000)
	if g.advice.scroll != 0 {
		t.Fatal("scrolled before start")
	}
	g.advice.maxScroll = 0
	g.scrollAdvice(1000)
	if g.advice.scroll != 0 {
		t.Fatal("short text scrolled")
	}
	c, before, after := adviceFixture(t, app.Japanese, []int{0, 1, 7})
	c.Phase = app.Review
	for i := range c.Evaluations {
		c.Evaluations[i] = &engine.Analysis{LocalRead: true}
	}
	g.controller, g.reviewBefore, g.reviewBoard = c, before, after
	if err := g.setReview(1); err != nil {
		t.Fatal(err)
	}
	if g.advice != nil || g.reviewTurn != 1 {
		t.Fatal("navigation retained old detail")
	}
}

// TestAdviceWrappingPreservesFullText は日本語長文の末尾まで省略せず、既存の短文表示も維持することを検証する。
func TestAdviceWrappingPreservesFullText(t *testing.T) {
	source, err := text.NewGoTextFaceSource(bytes.NewReader(fontData))
	if err != nil {
		t.Fatal(err)
	}
	g := &window{face: &text.GoTextFace{Source: source, Size: adviceFontSize}}
	value := strings.Repeat("A7の石と呼吸点を確認しましょう。", 40) + "全文の末尾。"
	lines := g.wrapLines(value, adviceWidth-16, adviceFontSize)
	if strings.Join(lines, "") != value || len(lines)*adviceLineHeight <= adviceHeight {
		t.Fatal("long detail lost text or did not overflow")
	}
	for _, line := range lines {
		if width, _ := text.Measure(line, g.face, adviceLineHeight); width > adviceWidth-16 {
			t.Fatal("line exceeded viewport")
		}
	}
	if got := g.wrap(value, adviceWidth-16, adviceFontSize, 3); strings.Count(got, "\n") != 2 || !strings.HasSuffix(got, "…") {
		t.Fatal("summary truncation changed")
	}
	for _, value := range []string{"あいう。えお", "あい（うえ）お", "あいう、えお"} {
		lines := g.wrapLines(value, 67, adviceFontSize)
		if strings.Join(lines, "") != value {
			t.Fatal("punctuation wrapping lost text")
		}
		for _, line := range lines {
			if strings.HasPrefix(line, "。") || strings.HasPrefix(line, "、") || strings.HasPrefix(line, "）") || strings.HasSuffix(line, "（") {
				t.Fatalf("invalid punctuation boundary: %q", line)
			}
		}
	}
	for _, value := range []string{"あCPUの石", "あT19の石", "あ（CPU）の石"} {
		lines := g.wrapLines(value, 67, adviceFontSize)
		if strings.Join(lines, "") != value {
			t.Fatal("word wrapping lost text")
		}
		token := "CPU"
		if strings.Contains(value, "T19") {
			token = "T19"
		}
		if !strings.Contains(strings.Join(lines, "\n"), token) {
			t.Fatalf("token split: %q", lines)
		}
		for _, line := range lines {
			if w, _ := text.Measure(line, g.face, adviceLineHeight); w > 67 {
				t.Fatal("word wrapping exceeded width")
			}
		}
	}
}

// TestMoveFactsPreserveCombinedEffects は取りと自分のアタリ、救出と連結が同時に残り、入力が不変であることを検証する。
func TestMoveFactsPreserveCombinedEffects(t *testing.T) {
	for _, tc := range []struct {
		name                                 string
		points                               []int
		taken, connected, rescued, liberties int
	}{
		{"capture-and-self-atari", []int{2, 1, 8, 7, 0}, 1, 0, 0, 1},
		{"rescue-and-connect", []int{0, 1, 14, 48, 7}, 0, 2, 1, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, before, after := adviceFixture(t, app.Japanese, tc.points)
			beforeCopy, err := app.Replay(before.Size, before.Rule, before.Moves, len(before.Moves))
			if err != nil {
				t.Fatal(err)
			}
			afterCopy, err := app.Replay(after.Size, after.Rule, after.Moves, len(after.Moves))
			if err != nil {
				t.Fatal(err)
			}
			move := c.Board.Moves[len(tc.points)-1]
			facts := collectMoveFacts(before, after, move)
			if facts.taken != tc.taken || facts.connected != tc.connected || len(facts.rescued) != tc.rescued || len(facts.liberties) != tc.liberties {
				t.Fatalf("unexpected combined facts: %+v", facts)
			}
			effect, lesson := moveLesson(before, after, move)
			if tc.taken > 0 && (!strings.Contains(effect, "相手の石を1個") || !strings.Contains(effect, "呼吸点が1つ") || !strings.Contains(lesson, "アタリ")) {
				t.Fatal("capture concealed own danger")
			}
			if tc.rescued > 0 && (!strings.Contains(effect, "アタリの石を助け") || !strings.Contains(effect, "石をつなぎ")) {
				t.Fatal("rescue concealed connection")
			}
			if !reflect.DeepEqual(before, beforeCopy) || !reflect.DeepEqual(after, afterCopy) {
				t.Fatal("fact extraction mutated board or history")
			}
		})
	}
}

// TestMoveFactsCountEachGroupOnce は複数方向で接する同じ連を一度だけ数えることを検証する。
func TestMoveFactsCountEachGroupOnce(t *testing.T) {
	c, before, after := adviceFixture(t, app.Japanese, []int{0, 48, 1, 47, 7, 46, 8})
	facts := collectMoveFacts(before, after, c.Board.Moves[6])
	if facts.connected != 1 || facts.stones != 4 {
		t.Fatalf("duplicate friendly group: %+v", facts)
	}
	c, before, after = adviceFixture(t, app.Chinese, []int{2, 0, 48, 1, 47, 7, 8})
	facts = collectMoveFacts(before, after, c.Board.Moves[6])
	if len(facts.attacked) != 1 || facts.attacked[0] != (changedGroup{0, 3, 2, 1}) {
		t.Fatalf("duplicate or incorrect opponent group: %+v", facts)
	}
}

// TestMoveFactsIgnorePassAndMissingBoards は着手のない局面から効果を作らないことを検証する。
func TestMoveFactsIgnorePassAndMissingBoards(t *testing.T) {
	c, before, after := adviceFixture(t, app.Jungo, []int{0, -1})
	for _, facts := range []moveFacts{
		collectMoveFacts(before, after, c.Board.Moves[1]),
		collectMoveFacts(nil, after, c.Board.Moves[0]),
		collectMoveFacts(before, nil, c.Board.Moves[0]),
	} {
		if !reflect.DeepEqual(facts, moveFacts{}) {
			t.Fatalf("unexpected facts: %+v", facts)
		}
	}
}

// TestReviewAutoplayWaitsAndStopsOnError は補完待ちと失敗した局面を自動再生が飛ばさないことを検証する。
func TestReviewAutoplayWaitsAndStopsOnError(t *testing.T) {
	c, _, _ := adviceFixture(t, app.Japanese, []int{0, 1, 7})
	c.Phase = app.Review
	c.Busy = true
	g := &window{controller: c, autoplay: true, reviewTurn: 1}
	g.advanceReview(time.Unix(100, 0))
	if g.reviewTurn != 1 || !g.autoplay {
		t.Fatal("autoplay skipped pending analysis")
	}
	c.Busy = false
	c.Err = errors.New("test analysis failure")
	g.advanceReview(time.Unix(101, 0))
	if g.autoplay || g.reviewTurn != 1 {
		t.Fatal("autoplay skipped failed analysis")
	}
}

// TestMoveLessons は取り・救出・アタリ・連結・自分の危険を区別することを検証する。
func TestMoveLessons(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		points     []int
	}{
		{"capture", "相手の石を1個", []int{0, 1, -1, 7}},
		{"escape", "アタリの石を助け", []int{0, 1, 7}},
		{"atari", "相手の石をアタリ", []int{20, 0, 1}},
		{"connect", "石をつなぎ", []int{0, 48, 2, 47, 1}},
		{"self-atari", "呼吸点が1つ", []int{20, 0, 21, 2, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, before, after := adviceFixture(t, app.Japanese, tc.points)
			effect, lesson := moveLesson(before, after, c.Board.Moves[len(tc.points)-1])
			if !strings.Contains(effect, tc.want) || lesson == "" {
				t.Fatalf("unexpected lesson: %s / %s", effect, lesson)
			}
		})
	}
}

// TestAdviceSeparatesPlayerAndCPU は実際に打った側を基準に説明し、未解析でも事実を表示することを検証する。
func TestAdviceSeparatesPlayerAndCPU(t *testing.T) {
	for _, rule := range []app.Rule{app.Japanese, app.Chinese, app.Jungo} {
		for _, points := range [][]int{{0, 1, 7}, {0, 1, -1, 7}} {
			c, before, after := adviceFixture(t, rule, points)
			turn := len(points)
			own := reviewAdvice(c, turn, before, after).detail
			c.Player = c.Player.Other()
			cpu := reviewAdvice(c, turn, before, after).detail
			if !strings.Contains(own, "あなたの手") || !strings.Contains(cpu, "CPUの狙い（推定）") || own == cpu {
				t.Fatalf("wrong perspective: own=%s CPU=%s", own, cpu)
			}
			if strings.Contains(own+cpu, "近似") || strings.Contains(own+cpu, "戻ると解析") || strings.Contains(own+cpu, "比較候補") {
				t.Fatal("repetitive disclaimer, manual prerequisite or unsupported candidate")
			}
		}
	}
}

// TestAdviceCandidateRequiresEvaluations は前後の解析と合法な候補が揃った場合だけ比較候補を示すことを検証する。
func TestAdviceCandidateRequiresEvaluations(t *testing.T) {
	c, before, after := adviceFixture(t, app.Japanese, []int{20, 0, 1})
	c.Evaluations[2] = &engine.Analysis{ScoreLead: 5, Moves: []engine.Candidate{{Point: 2}}}
	if strings.Contains(reviewAdvice(c, 3, before, after).detail, "比較候補") {
		t.Fatal("candidate shown without after evaluation")
	}
	c.Evaluations[3] = &engine.Analysis{ScoreLead: 1}
	if !strings.Contains(reviewAdvice(c, 3, before, after).detail, "比較候補：C7") {
		t.Fatal("candidate missing")
	}
	c.Evaluations[2].Moves[0].Point = 0
	if strings.Contains(reviewAdvice(c, 3, before, after).detail, "比較候補") {
		t.Fatal("occupied candidate shown")
	}
}

// TestPassAdviceUsesRuleAndMover はパスの助言が手番と採点ルールを取り違えないことを検証する。
func TestPassAdviceUsesRuleAndMover(t *testing.T) {
	for _, rule := range []app.Rule{app.Japanese, app.Chinese, app.Jungo} {
		c, before, after := adviceFixture(t, rule, []int{0, -1})
		own := reviewAdvice(c, 2, before, after).detail
		if strings.Contains(own, "純碁") != (rule == app.Jungo) {
			t.Fatal("wrong rule advice")
		}
		c.Player = app.Black
		if !strings.Contains(reviewAdvice(c, 2, before, after).detail, "CPUはパス") {
			t.Fatal("CPU pass missing")
		}
	}
}

// TestDetailedAdviceUsesFactsAndCautions は具体的な石数と座標、複合効果、コウの制約を両視点で説明することを検証する。
func TestDetailedAdviceUsesFactsAndCautions(t *testing.T) {
	for _, rule := range []app.Rule{app.Japanese, app.Chinese, app.Jungo} {
		c, before, after := adviceFixture(t, rule, []int{2, 1, 8, 7, 0})
		for _, player := range []app.Color{app.Black, app.White} {
			c.Player = player
			advice := reviewAdvice(c, 5, before, after)
			for _, want := range []string{"A7", "石を1個", "空き（呼吸点）", "1か所", "アタリ", "B7", "すぐに打てない"} {
				if !strings.Contains(advice.detail, want) {
					t.Fatalf("missing %q in %s", want, advice.detail)
				}
			}
			if count := strings.Count(advice.detail, "。"); count < 4 || count > 6 {
				t.Fatalf("unexpected sentence count: %d", count)
			}
			if advice.summary == "" || strings.Contains(advice.detail, "近似") {
				t.Fatal("missing summary or repetitive disclaimer")
			}
			if (player == app.White) != strings.Contains(advice.detail, "CPUの狙い（推定）") {
				t.Fatal("wrong mover perspective")
			}
		}
	}
	c, before, after := adviceFixture(t, app.Japanese, []int{0, 1, 14, 48, 7})
	advice := reviewAdvice(c, 5, before, after)
	for _, want := range []string{"A6", "2組の石", "A7", "1か所から3か所", "次局では"} {
		if !strings.Contains(advice.detail, want) {
			t.Fatalf("missing combined effect %q: %s", want, advice.detail)
		}
	}
}

// TestAdviceExplainsEachEffectInPlainSentences は複合効果を省かず、短文の意味と次の確認を両手番で検証する。
func TestAdviceExplainsEachEffectInPlainSentences(t *testing.T) {
	for _, tc := range []struct {
		name, fact, lesson string
		points             []int
	}{
		{"capture", "石を1個取りました", "取る前に", []int{0, 1, -1, 7}},
		{"rescue", "1か所から2か所", "増やせる交点", []int{0, 1, 7}},
		{"connect-and-rescue", "2組の石をつなぎました", "1か所から3か所", []int{0, 1, 14, 48, 7}},
		{"atari", "1か所に減り、アタリ", "相手が空きを増やせる交点", []int{20, 0, 1}},
		{"capture-and-ko", "石を1個取りました", "すぐに打てない", []int{2, 1, 8, 7, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, before, after := adviceFixture(t, app.Japanese, tc.points)
			own := reviewAdvice(c, len(tc.points), before, after)
			for _, want := range []string{tc.fact, tc.lesson, "空き（呼吸点）"} {
				if !strings.Contains(own.detail, want) {
					t.Fatalf("missing explanation %q: %s", want, own.detail)
				}
			}
			if strings.Contains(own.detail, "この手の注目点は") || strings.Contains(own.summary, "救出・連結") {
				t.Fatal("terse effect list or repeated generic meaning survived")
			}
			if n := strings.Count(own.detail, "。"); n < 4 || n > 6 {
				t.Fatalf("ordinary explanation has %d sentences", n)
			}
			c.Player = c.Player.Other()
			cpu := reviewAdvice(c, len(tc.points), before, after)
			if !strings.Contains(cpu.detail, tc.fact) || !strings.Contains(cpu.detail, "だったと考えられます") || strings.Contains(cpu.detail, "次局では") {
				t.Fatalf("CPU explanation lost facts or player response: %s", cpu.detail)
			}
		})
	}
}

// TestAdviceKeepsAllTargetsInDensePosition は複数の対象や効果があっても詳細で座標・個数・変化を失わないことを検証する。
func TestAdviceKeepsAllTargetsInDensePosition(t *testing.T) {
	facts := moveFacts{taken: 2, connected: 2,
		rescued:  []changedGroup{{point: 0, stones: 2, oldLiberties: 1, newLiberties: 3}, {point: 2, stones: 1, oldLiberties: 1, newLiberties: 2}},
		attacked: []changedGroup{{point: 14, stones: 3, oldLiberties: 2, newLiberties: 1}}}
	got := describeMoveChanges(facts, 7, "あなた", "CPU")
	for _, want := range []string{"石を2個取りました。", "2組の石をつなぎました。", "A7のあなたの石2個", "C7のあなたの石1個", "A5のCPUの石3個", "1か所から3か所", "1か所から2か所", "2か所から1か所"} {
		if !strings.Contains(got, want) {
			t.Fatalf("lost target or effect %q: %s", want, got)
		}
	}
	if strings.Count(got, "。\n") != 4 {
		t.Fatal("combined effects were joined into one long sentence")
	}
}

// TestAdviceOpeningAndPassExplainScoring は初期局面・1回目のパス・連続パスで3ルールの数え方を混同しないことを検証する。
func TestAdviceOpeningAndPassExplainScoring(t *testing.T) {
	for _, tc := range []struct {
		rule app.Rule
		name string
	}{
		{app.Japanese, "日本ルール"}, {app.Chinese, "中国ルール"}, {app.Jungo, "純碁"},
	} {
		t.Run(string(tc.rule), func(t *testing.T) {
			c, before, after := adviceFixture(t, tc.rule, []int{0, -1})
			opening := reviewAdvice(c, 0, nil, nil).detail
			pass := reviewAdvice(c, 2, before, after).detail
			if !strings.Contains(opening, tc.name) || !strings.Contains(pass, tc.name) || !strings.Contains(pass, "まだ連続パスではない") {
				t.Fatal("scoring rule or first pass was not explained")
			}
			if tc.rule == app.Jungo && (!strings.Contains(opening, "空きだけでは得点になりません") || !strings.Contains(pass, "実際に取る必要")) {
				t.Fatal("stone scoring was confused with territory")
			}
			c, before, after = adviceFixture(t, tc.rule, []int{0, -1, -1})
			if got := reviewAdvice(c, 3, before, after).detail; !strings.Contains(got, "終局して採点") || strings.Contains(got, "まだ連続パスではない") {
				t.Fatal("terminal pass used ongoing-game wording")
			}
		})
	}
}

// TestDetailedAdviceCandidateBoundaries は未解析・解析失敗・小さい損失・同じ手・非合法手を比較候補にしないことを検証する。
func TestDetailedAdviceCandidateBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name                                               string
		player                                             app.Color
		point                                              int
		loss                                               float64
		missingBefore, missingAfter, failed, partial, want bool
	}{
		{name: "valid", player: app.Black, point: 2, loss: 4, want: true},
		{name: "partial", player: app.Black, point: 2, loss: 4, partial: true, want: true},
		{name: "white-perspective", player: app.White, point: 2, loss: 4},
		{name: "same-move", player: app.Black, point: 1, loss: 4},
		{name: "occupied", player: app.Black, point: 0, loss: 4},
		{name: "outside", player: app.Black, point: 49, loss: 4},
		{name: "low-loss", player: app.Black, point: 2, loss: .5},
		{name: "missing-before", player: app.Black, point: 2, loss: 4, missingBefore: true},
		{name: "missing-after", player: app.Black, point: 2, loss: 4, missingAfter: true},
		{name: "failed", player: app.Black, point: 2, loss: 4, failed: true},
		{name: "pass", player: app.Black, point: -1, loss: 4, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, before, after := adviceFixture(t, app.Japanese, []int{20, 0, 1})
			c.Player = tc.player
			if !tc.missingBefore {
				c.Evaluations[2] = &engine.Analysis{ScoreLead: tc.loss, Partial: tc.partial, Moves: []engine.Candidate{{Point: tc.point}}}
			}
			if !tc.missingAfter {
				c.Evaluations[3] = &engine.Analysis{}
			}
			if tc.failed {
				c.Err = errors.New("analysis failed")
			}
			advice := reviewAdvice(c, 3, before, after)
			if strings.Contains(advice.detail, "比較候補") != tc.want {
				t.Fatalf("unexpected candidate: %s", advice.detail)
			}
			if tc.want && tc.partial && !strings.Contains(advice.detail, "途中解析") {
				t.Fatal("partial candidate was not marked")
			}
			if !strings.Contains(advice.detail, "B7") {
				t.Fatal("facts disappeared without comparison")
			}
			if n := strings.Count(advice.detail, "。"); n < 4 || n > 6 {
				t.Fatalf("unexpected sentence count: %d", n)
			}
		})
	}
}

// TestAdviceInitialAndTerminalPass は初期局面と終局済みの連続パスを通常着手と区別することを検証する。
func TestAdviceInitialAndTerminalPass(t *testing.T) {
	c, before, after := adviceFixture(t, app.Jungo, []int{0, -1, -1})
	if got := reviewAdvice(c, 0, nil, nil); !strings.Contains(got.detail, "まだ石を置いていない") {
		t.Fatal("missing initial explanation")
	}
	if got := reviewAdvice(c, 3, before, after); !strings.Contains(got.detail, "終局して採点") || !strings.Contains(got.detail, "盤上の石数") {
		t.Fatal("wrong terminal pass explanation")
	}
	for _, turn := range []int{-1, 4} {
		if got := reviewAdvice(c, turn, nil, nil); got.summary != "局面を準備しています。" {
			t.Fatal("invalid turn accepted")
		}
	}
	if got := reviewAdvice(nil, 0, nil, nil); got.summary != "局面を準備しています。" {
		t.Fatal("missing controller accepted")
	}
}
