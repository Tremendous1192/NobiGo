// 作成日: 2026-10-18。感想戦は保存済み対局を変更せず再生する。助言は盤面と解析結果だけから作る。
package gui

import (
	"fmt"
	"image"
	"strings"
	"time"

	"github.com/Tremendous1192/PlayGo/engine"
	"github.com/Tremendous1192/PlayGo/internal/app"
	"github.com/hajimehoshi/ebiten/v2"
)

// setReview は再生位置を更新し、必要な局面解析を要求する。
// 失敗は画面へ案内して再生も停止するため、再生ボタン側では終了エラーへ変換しない。
func (g *window) setReview(turn int) error {
	var before *app.Board
	if turn > 0 {
		var err error
		before, err = g.controller.Snapshot(turn - 1)
		if err != nil {
			g.notice = "直前の局面を再生できません。結果画面へ戻ってください。"
			g.autoplay = false
			return err
		}
	}
	b, err := g.controller.ReviewAt(turn)
	if err != nil {
		g.notice = "この局面を再生できません。結果画面へ戻ってください。"
		g.autoplay = false
		return err
	}
	g.reviewBoard = b
	g.reviewBefore = before
	g.reviewTurn = turn
	g.advice = nil
	g.reviewFocus = nil
	g.notice = ""
	g.nextReplay = time.Now().Add(1200 * time.Millisecond)
	return nil
}

// advanceReview は必要な解析の完了を待って再生する。失敗を飛ばさず停止し、再試行の操作を残す。
func (g *window) advanceReview(now time.Time) {
	c := g.controller
	if g.advice != nil {
		g.autoplay = false
		return
	}
	if c == nil || !g.autoplay || c.Phase != app.Review || g.dialog != nil || g.menu != nil {
		return
	}
	if c.Err != nil || g.reviewTurn >= len(c.Board.Moves) {
		g.autoplay = false
		return
	}
	if !c.Busy && now.After(g.nextReplay) {
		_ = g.setReview(g.reviewTurn + 1)
	}
}

// drawReview は元の棋譜・戦績を変えずに再生する。
func (g *window) drawReview(screen *ebiten.Image) {
	c := g.controller
	b := g.reviewBoard
	if b == nil {
		_ = g.setReview(0)
		b = g.reviewBoard
		if b == nil {
			return
		}
	}
	g.label(screen, "感想戦", 35, 25, 34, foreground)
	g.label(screen, fmt.Sprintf("%s · %d路 · %d / %d手", ruleName(b.Rule), b.Size, g.reviewTurn, len(c.Board.Moves)), 230, 36, 21, muted)
	status := "一手ずつ振り返り、次の対局のヒントを見つけましょう。"
	if c.Busy {
		status = "この手と直前の局面を解析しています…"
	}
	g.label(screen, status, 38, 82, 19, accent)
	a := g.currentAnalysis(g.reviewTurn)
	if g.reviewTurn == len(c.Board.Moves) && a == nil {
		a = scoreOwnership(c.Score)
	}
	g.drawBoard(screen, b, a, nil, nil, false)
	g.drawSidebar(screen, b, g.reviewTurn)
	g.drawReviewFocus(screen)
	g.button(screen, image.Rect(35, 741, 215, 788), "一手戻る", g.reviewTurn > 0, func() { g.autoplay = false; _ = g.setReview(g.reviewTurn - 1) })
	playLabel := "自動再生"
	if g.autoplay {
		playLabel = "再生を停止"
	}
	g.button(screen, image.Rect(229, 741, 409, 788), playLabel, true, func() {
		if g.reviewTurn == len(c.Board.Moves) {
			if err := g.setReview(0); err != nil {
				return
			}
		}
		g.autoplay = !g.autoplay
		g.nextReplay = time.Now().Add(1200 * time.Millisecond)
	})
	g.button(screen, image.Rect(423, 741, 635, 788), "一手進める", g.reviewTurn < len(c.Board.Moves), func() { g.autoplay = false; _ = g.setReview(g.reviewTurn + 1) })
	if c.Err != nil {
		g.button(screen, image.Rect(660, 741, 865, 788), "解析を再試行", true, func() { _ = g.setReview(g.reviewTurn) })
	}
	g.button(screen, image.Rect(880, 741, 1145, 788), "感想戦を終える", true, g.finishReview)
}

type adviceText struct {
	summary, detail string
	focus           []focusTarget
}

// reviewAdvice は表示局面の事実・推定・学びを要点と詳細へ分ける。外部通信や盤面の変更は行わない。
func reviewAdvice(c *app.Controller, turn int, before, after *app.Board) adviceText {
	if c == nil || c.Board == nil || turn < 0 || turn > len(c.Board.Moves) {
		return adviceText{summary: "局面を準備しています。", detail: "局面を準備しています。"}
	}
	if turn == 0 {
		return adviceText{summary: "最初の局面です。隅や辺に注目しましょう。", detail: "まだ石を置いていない局面です。\n隅や辺では盤の端も囲いに使えるため、中央より少ない石で地を囲めます。\n最初の数手は、隅や辺に打った石がどうつながるかを見てみましょう。"}
	}
	if before == nil || after == nil {
		return adviceText{summary: "局面を準備しています。", detail: "局面を準備しています。"}
	}
	move := c.Board.Moves[turn-1]
	playerMove := move.Color == c.Player
	if move.Point < 0 {
		return passAdvice(after, playerMove)
	}
	facts := collectMoveFacts(before, after, move)
	actor, other, title := "あなた", "CPU", "あなたの手"
	if !playerMove {
		actor, other, title = "CPU", "あなた", "CPUの狙い（推定）"
	}
	vertex := adviceVertex(move.Point, after.Size)
	changes := describeMoveChanges(facts, after.Size, actor, other)
	detail := []string{
		fmt.Sprintf("%sは%sに打ち、%s。", actor, vertex, changes),
		fmt.Sprintf("着手した%sの連（上下左右につながった石）は%d個で、呼吸点（連に隣接する空点）は%dか所です。", vertex, facts.stones, len(facts.liberties)),
	}
	meaning, caution, action := explainMove(facts, after, playerMove)
	detail = append(detail, meaning, caution, action)
	if playerMove && c.Err == nil && turn < len(c.Evaluations) {
		a := c.Evaluations[turn-1]
		if loss, ok := c.Loss(turn); ok && a != nil && len(a.Moves) > 0 && loss >= 1 && a.Moves[0].Point != move.Point && before.Legal(a.Moves[0].Point) {
			qualifier := ""
			if a.Partial || c.Evaluations[turn].Partial {
				qualifier = "（途中解析）"
			}
			detail = append(detail, "比較候補："+adviceVertex(a.Moves[0].Point, before.Size)+qualifier+"も着手前の盤面で見直し、取れる石や、打った後の呼吸点を比べてみましょう。")
		}
	}
	summary := summarizeMove(facts, vertex, actor, playerMove, after.Size)
	return adviceText{summary: summary, detail: title + "\n" + strings.Join(detail, "\n"), focus: moveFocusTargets(before, after, move, facts)}
}

// summarizeMove は危険・主な効果・確認事項を短い3行へ分け、詳細の省略で危険が隠れないようにする。
func summarizeMove(facts moveFacts, vertex, actor string, playerMove bool, size int) string {
	first := fmt.Sprintf("%sの手：%s、呼吸点%d。", actor, vertex, len(facts.liberties))
	if len(facts.liberties) == 1 {
		first = actor + "の手：アタリ。呼吸点は" + adviceVertex(facts.liberties[0], size) + "だけ。"
	}
	var effects []string
	if facts.taken > 0 {
		effects = append(effects, fmt.Sprintf("取り%d個", facts.taken))
	}
	if len(facts.attacked) > 0 {
		effects = append(effects, "相手アタリ")
	}
	if len(facts.rescued) > 0 {
		effects = append(effects, "救出")
	}
	if facts.connected >= 2 {
		effects = append(effects, "連結")
	}
	second := strings.Join(effects, "・")
	if second == "" {
		second = "周囲の石とのつながりに注目。"
		if facts.connected == 1 {
			second = "自分の石から伸びました。"
		}
	}
	third := "次局は打った後の呼吸点も確認。"
	if !playerMove {
		third = "自分の連とCPUの呼吸点を確認。"
	}
	return strings.Join([]string{first, second, third}, "\n")
}

// adviceVertex は検証済みの盤上点を盤面表示と同じ座標へ変換する。
func adviceVertex(point, size int) string {
	if point == -1 {
		return "パス"
	}
	vertex, err := engine.Vertex(point, size)
	if err != nil {
		return "盤外"
	}
	return vertex
}

// describeMoveChanges は同時に変化した連を座標と実数で説明する。
func describeMoveChanges(facts moveFacts, size int, actor, other string) string {
	var changes []string
	if facts.taken > 0 {
		changes = append(changes, fmt.Sprintf("%sの石を%d個取り", other, facts.taken))
	}
	if facts.connected >= 2 {
		changes = append(changes, fmt.Sprintf("%sの%dつの連をつなぎ", actor, facts.connected))
	} else if facts.connected == 1 {
		changes = append(changes, actor+"の石から伸び")
	}
	for _, group := range facts.rescued {
		changes = append(changes, fmt.Sprintf("%sにあった%sの%d個の石の呼吸点を1か所から%dか所に増やし", adviceVertex(group.point, size), actor, group.stones, group.newLiberties))
	}
	for _, group := range facts.attacked {
		changes = append(changes, fmt.Sprintf("%sの%sの%d個の石の呼吸点を%dか所から1か所に減らし", adviceVertex(group.point, size), other, group.stones, group.oldLiberties))
	}
	if len(changes) == 0 {
		return "新しい場所に石を置きました"
	}
	return strings.Join(changes, "、") + "ました"
}

// explainMove は盤面の意味と次の確認を分け、アタリや推定意図を勝敗の断定へ変換しない。
func explainMove(facts moveFacts, after *app.Board, playerMove bool) (meaning, caution, action string) {
	var aims []string
	if facts.taken > 0 {
		aims = append(aims, "相手の石を取ること")
	}
	if len(facts.rescued) > 0 {
		aims = append(aims, "アタリだった石の呼吸点を増やすこと")
	}
	if len(facts.attacked) > 0 {
		aims = append(aims, "相手の石の逃げ道を減らすこと")
	}
	if facts.connected >= 2 {
		aims = append(aims, "離れた石をつなぐこと")
	}
	if len(aims) == 0 {
		if facts.connected == 1 {
			aims = append(aims, "石をつなげながら周囲へ伸びること")
		} else {
			aims = append(aims, "今後の石の足場を作ること")
		}
	}
	meaning = "この手の注目点は、" + strings.Join(aims, "と") + "です。"
	if !playerMove {
		meaning = "盤面の変化からは、" + strings.Join(aims, "と") + "がCPUの狙いと考えられます（推定）。"
	}
	caution = "呼吸点の数だけでは石の安全は決まらないので、周囲の相手の石も確認しましょう。"
	action = "次局では、打つ前に自分の連と相手の連の呼吸点をそれぞれ数えてみましょう。"
	if !playerMove {
		action = "あなたは、CPUの石に近い自分の連の呼吸点を数え、守る必要があるか確認しましょう。"
	}
	if len(facts.attacked) > 0 {
		caution = "相手の連はアタリ（呼吸点が残り1つ）ですが、逃げたり石を取ったりして助かる場合もあります。"
		if !playerMove {
			action = "あなたは、アタリになった連を伸びて助ける手と、周囲のCPUの石を取る手を確認しましょう。"
		}
	}
	if len(facts.rescued) > 0 && len(facts.attacked) == 0 {
		caution = "アタリ（呼吸点が残り1つ）は解消しましたが、この先も取られないと確定したわけではありません。"
	}
	if len(facts.liberties) == 1 {
		last := adviceVertex(facts.liberties[0], after.Size)
		caution = "ただし、着手した連もアタリ（呼吸点が残り1つ）で、最後の呼吸点は" + last + "です。"
		if len(facts.attacked) > 0 {
			caution = "相手の連をアタリにしましたが、着手した連もアタリ（呼吸点が残り1つ）で、最後の呼吸点は" + last + "です。"
		}
		if playerMove {
			action = "次局では、取れる石だけでなく、打った後に自分の連へ逃げ道が残るかも確認しましょう。"
		} else {
			action = "あなたは、最後の呼吸点へ打てるかと、打った後の自分の石の安全を確認しましょう。"
		}
		if !after.Legal(facts.liberties[0]) {
			action = "現在の規則ではその呼吸点へすぐに打てないため、次局でもアタリと実際に取れるかを区別して確認しましょう。"
		}
	}
	return meaning, caution, action
}

// passAdvice は実際の連続パス数と採点ルールに沿って、終局前に確認する内容を示す。
func passAdvice(after *app.Board, playerMove bool) adviceText {
	actor := "あなた"
	if !playerMove {
		actor = "CPU"
	}
	summary := actor + "はパスしました。"
	ending := "双方が続けてパスすると終局するため、今は打ち残しを確かめる場面です。"
	if after.Passes >= 2 {
		ending = "双方が続けてパスしたので、この対局は終局して採点へ進みました。"
	}
	rule := "このルールでは地も得点になるため、囲った空点と守る必要のある石を見直しましょう。"
	if after.Rule == app.Jungo {
		rule = "純碁は盤上の石数で得点が決まるため、実際に取れる石と安全に増やせる石を見直しましょう。"
	}
	action := "次局では、パスの前に自分の弱い連と相手の取り残した石を一度確認してみましょう。"
	if !playerMove {
		action = "CPUは終局を目指した可能性がありますが、意図は断定せず、あなた自身の打ち残しも確認しましょう。"
	}
	return adviceText{summary: summary, detail: strings.Join([]string{summary + "盤上の石の配置は変わりません。", ending, rule, action}, "\n")}
}

type changedGroup struct {
	point, stones, oldLiberties, newLiberties int
}

// moveFacts は着手前後の事実を所有し、解析の優劣や読み筋を含まない。
type moveFacts struct {
	taken, stones, connected int
	liberties                []int
	rescued, attacked        []changedGroup
}

// collectMoveFacts は合法な着手前後から複合効果を収集する。入力盤面と履歴は変更しない。
func collectMoveFacts(before, after *app.Board, move app.Move) moveFacts {
	var facts moveFacts
	if before == nil || after == nil || before.Size != after.Size || move.Point < 0 || move.Point >= len(after.Cells) || (move.Color != app.Black && move.Color != app.White) || after.Cells[move.Point] != move.Color {
		return facts
	}
	facts.taken = after.Captured[move.Color] - before.Captured[move.Color]
	stones, liberties := after.Group(move.Point)
	facts.stones, facts.liberties = len(stones), liberties
	adjacent := make(map[int]bool)
	for _, p := range app.Neighbors(move.Point, before.Size) {
		adjacent[p] = true
	}
	seen := make([]bool, len(before.Cells))
	// 取り石で離れた味方の呼吸点も増えるため、着手点の隣だけには限定しない。
	for point, color := range before.Cells {
		if color == app.Empty || seen[point] {
			continue
		}
		group, oldLiberties := before.Group(point)
		touches := false
		for _, p := range group {
			seen[p] = true
			touches = touches || adjacent[p]
		}
		_, newLiberties := after.Group(point)
		change := changedGroup{point, len(group), len(oldLiberties), len(newLiberties)}
		if color == move.Color {
			if touches {
				facts.connected++
			}
			if len(oldLiberties) == 1 && len(newLiberties) > 1 {
				facts.rescued = append(facts.rescued, change)
			}
		} else if len(oldLiberties) > 1 && len(newLiberties) == 1 {
			facts.attacked = append(facts.attacked, change)
		}
	}
	return facts
}

// moveLesson は同時に起きた効果を併記し、自分の危険を助言の優先事項にする。
func moveLesson(before, after *app.Board, move app.Move) (effect, lesson string) {
	facts := collectMoveFacts(before, after, move)
	var effects []string
	lesson = "近くの石とのつながりも考えてみましょう。"
	if facts.taken > 0 {
		effects = append(effects, fmt.Sprintf("相手の石を%d個取りました。", facts.taken))
		lesson = "取る前に、相手の最後の呼吸点を確認しましょう。"
	}
	if len(facts.rescued) > 0 {
		effects = append(effects, "アタリの石を助け、呼吸点を増やしました。")
		lesson = "逃げる手と、相手の石を取る手を比べましょう。"
	}
	if len(facts.attacked) > 0 {
		effects = append(effects, "相手の石をアタリにしました。")
		lesson = "相手が逃げた後の、自分の石の安全も見ましょう。"
	}
	if facts.connected >= 2 {
		effects = append(effects, "離れていた自分の石をつなぎました。")
	}
	if len(facts.liberties) == 1 {
		effects = append(effects, "この石の連は呼吸点が1つです。")
		lesson = "アタリです。相手に取られないか確認しましょう。"
	}
	if len(effects) == 0 {
		if facts.connected == 1 {
			effects = append(effects, fmt.Sprintf("自分の石から伸び、呼吸点は%dです。", len(facts.liberties)))
		} else {
			effects = append(effects, fmt.Sprintf("新しい場所に石を置き、呼吸点は%dです。", len(facts.liberties)))
		}
	}
	return strings.Join(effects, ""), lesson
}
