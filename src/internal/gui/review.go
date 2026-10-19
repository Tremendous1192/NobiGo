// 作成日: 2026-10-18。感想戦を再生し、盤面の事実から初心者向けの助言を作る。棋譜を変更せず、推定は断定しない。
package gui

import (
	"fmt"
	"image"
	"strings"
	"time"

	"github.com/Tremendous1192/NobiGo/engine"
	"github.com/Tremendous1192/NobiGo/internal/app"
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
		goal := "日本ルールでは、囲った空き（地）と取った相手の石を得点に数えます。"
		if c.Board.Rule == app.Chinese {
			goal = "中国ルールでは、盤上の自分の石と囲った空き（地）を得点に数えます。"
		} else if c.Board.Rule == app.Jungo {
			goal = "純碁では、最後に盤上に残った石が多い方が勝ちで、囲った空きだけでは得点になりません。"
		}
		return adviceText{summary: "まだ石を置いていません。\n石の上下左右の空きに注目しましょう。", detail: "まだ石を置いていない局面です。\n" + goal + "\n石の上下左右に接する空いた交点を「呼吸点」と呼び、これがなくなると石は取られます。\n最初の数手では、置いた石に接する空きを一つずつ数えてみましょう。"}
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
		changes,
		fmt.Sprintf("%sからつながる石は%d個で、上下左右の空き（呼吸点）は%dか所です。", vertex, facts.stones, len(facts.liberties)),
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
			detail = append(detail, "比較候補："+adviceVertex(a.Moves[0].Point, before.Size)+qualifier+"もAIが挙げていますが、その先の結果は未確定なので、打つ前の図で自分の石を守れるか比べてみましょう。")
		}
	}
	summary := summarizeMove(facts, vertex, actor, playerMove, after.Size)
	return adviceText{summary: summary, detail: title + "：" + vertex + "\n" + strings.Join(detail, "\n"), focus: moveFocusTargets(before, after, move, facts)}
}

// summarizeMove は危険・主な効果・確認事項を3行にする。同時に起きた他の効果は詳細に残す。
func summarizeMove(facts moveFacts, vertex, actor string, playerMove bool, size int) string {
	first := actor + "の手：" + vertex + "に置きました。"
	other := "相手"
	if !playerMove {
		other = "あなた"
	}
	second := "周りの石とのつながりに注目しましょう。"
	switch {
	case facts.taken > 0:
		second = fmt.Sprintf("%sの石を%d個取りました。", other, facts.taken)
	case len(facts.rescued) > 0:
		second = "取られそうな石の空きを増やしました。"
	case len(facts.attacked) > 0:
		second = other + "の石をアタリにしました。"
	case facts.connected >= 2:
		second = "離れていた石をつなぎました。"
	case facts.connected == 1:
		second = "近くの石とつながりました。"
	}
	third := "次局は、打った後の空きも数えましょう。"
	if !playerMove {
		third = "近くの自分の石に空きがあるか確認。"
	}
	if len(facts.liberties) == 1 {
		first = actor + "の" + vertex + "：打った石もアタリです。"
		third = "残る空きは" + adviceVertex(facts.liberties[0], size) + "。すぐ取れるかは詳細へ。"
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

// describeMoveChanges は同時に起きた効果を短文で分け、各対象の座標と実数を残す。
func describeMoveChanges(facts moveFacts, size int, actor, other string) string {
	var changes []string
	if facts.taken > 0 {
		changes = append(changes, fmt.Sprintf("%sの石を%d個取りました。", other, facts.taken))
	}
	if facts.connected >= 2 {
		changes = append(changes, fmt.Sprintf("離れていた%sの%d組の石をつなぎました。", actor, facts.connected))
	} else if facts.connected == 1 {
		changes = append(changes, "近くにある"+actor+"の石とつながりました。")
	}
	for _, group := range facts.rescued {
		changes = append(changes, fmt.Sprintf("%sの%sの石%d個は、周りの空きが1か所から%dか所に増えました。", adviceVertex(group.point, size), actor, group.stones, group.newLiberties))
	}
	for _, group := range facts.attacked {
		changes = append(changes, fmt.Sprintf("%sの%sの石%d個は、周りの空きが%dか所から1か所に減り、アタリになりました。", adviceVertex(group.point, size), other, group.stones, group.oldLiberties))
	}
	if len(changes) == 0 {
		return "近くの味方とはまだつながっていません。"
	}
	// 複数の対象がある局面では、文数を減らすために危険や別の石の変化を省かない。
	return strings.Join(changes, "\n")
}

// explainMove は盤面の意味と次の確認を分け、アタリや推定意図を勝敗の断定へ変換しない。
func explainMove(facts moveFacts, after *app.Board, playerMove bool) (meaning, caution, action string) {
	meaning = "近くの味方と上下左右につながれば、その石も含めて空きを数えます。"
	aim := "周りに石を増やす足場を作ること"
	action = "次局では、打つ前に近くの味方と相手の石の空きを一つずつ数えてみましょう。"
	switch {
	case facts.taken > 0:
		meaning = "石を取って空いた場所を使い、自分の石の空きを増やせるかも確認したい場面です。"
		aim = "あなたの石を取り、周りに空きを作ること"
		action = "次局では、取る前に、置いた後の自分の石に空きが残るかを確かめましょう。"
	case len(facts.rescued) > 0:
		meaning = "空きが増えた石は、残り1か所をふさがれると取られる状態から抜け出しました。"
		aim = "取られそうな自分の石の空きを増やすこと"
		action = "次局では、取られそうな石から先に空きを数え、増やせる交点を探してみましょう。"
	case len(facts.attacked) > 0:
		meaning = "残る空きをふさげれば相手の石を取れますが、相手にも次の手があります。"
		aim = "あなたの石の空きを減らし、守りを迫ること"
		action = "次局では、アタリにした後に相手が空きを増やせる交点も探してみましょう。"
	case facts.connected >= 1:
		meaning = "上下左右につながった石はひとまとまりなので、全体に接する空きを数えるのが大切です。"
		aim = "自分の石をつなぎ、まとまりを作ること"
		action = "次局では、つなぐ前に、つながった後の石全体の空きを数えてみましょう。"
	}
	caution = "周りに相手の石が増えると空きが減るため、今の数だけで安全とは決められません。"
	if !playerMove {
		meaning = "CPUの狙いは、" + aim + "だったと考えられます。"
		action = "近くの自分の石を見て、CPUに空きをふさがれそうか確認しましょう。"
	}
	if len(facts.attacked) > 0 {
		caution = "アタリは空きが残り1か所の状態ですが、石をつなぐ・相手の石を取るなどで空きを増やせる場合があります。"
		if !playerMove {
			action = "アタリになった自分の石を見て、空きを増やす手と近くのCPUの石を取る手を比べましょう。"
		}
	}
	if len(facts.rescued) > 0 && len(facts.attacked) == 0 {
		caution = "ひとまずアタリは解消しましたが、この先も取られないと決まったわけではありません。"
	}
	if len(facts.liberties) == 1 {
		last := adviceVertex(facts.liberties[0], after.Size)
		caution = "ただし、打った石も空きが1か所だけの「アタリ」で、残る空きは" + last + "です。"
		if len(facts.attacked) > 0 {
			caution = "相手だけでなく打った石もアタリ（空きが残り1か所）で、打った石に残る空きは" + last + "です。"
		}
		if playerMove {
			action = "次局では、取れる石だけでなく、打った自分の石が取られないかも確認しましょう。"
		} else {
			action = last + "に打てるかと、打った後の自分の石に空きが残るかを確認しましょう。"
		}
		if !after.Legal(facts.liberties[0]) {
			action = last + "は今の規則ではすぐに打てないため、アタリでも直ちに取り返せないことを図で確かめましょう。"
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
	ending := "まだ連続パスではないので、次の人は石を置くこともできます。"
	if after.Passes >= 2 {
		ending = "双方が続けてパスしたので、この対局は終局して採点へ進みました。"
	}
	rule := "日本ルールは囲った空き（地）と取り石で数えるため、囲いの切れ目や取られそうな石を見直しましょう。"
	if after.Rule == app.Chinese {
		rule = "中国ルールは盤上の石と囲った空き（地）で数えるため、囲いの切れ目や取られそうな石を見直しましょう。"
	}
	if after.Rule == app.Jungo {
		rule = "純碁は盤上の石数で決まり、終局時に死に石を取り除かないため、取れる石はパスの前に実際に取る必要があります。"
	}
	action := "次局では、パスの前に自分の石が取られそうな場所と、まだ取れる相手の石を確認してみましょう。"
	if !playerMove {
		action = "CPUは打つ必要がないと考えた可能性がありますが、あなたも取れる石や守る場所が残っていないか確認しましょう。"
	}
	return adviceText{summary: summary, detail: strings.Join([]string{summary + "盤上の石はそのままです。", ending, rule, action}, "\n")}
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
