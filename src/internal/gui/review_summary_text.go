// 作成日: 2026-10-24。概要の事実・推定・次の確認を日本語で説明する。AIの数値を確定得点や未来の保証へ変換しない。
package gui

import (
	"fmt"
	"strings"

	"github.com/Tremendous1192/NobiGo/internal/app"
)

// summaryActor はプレイヤーとの関係と石色を一緒に示す。
func summaryActor(c *app.Controller, color app.Color) string {
	name := "CPU"
	if color == c.Player {
		name = "あなた"
	}
	return name + "（" + summaryColor(color) + "）"
}

// summaryColor は盤上の石色を短い名前へ変換する。
func summaryColor(color app.Color) string {
	if color == app.Black {
		return "黒"
	}
	return "白"
}

// summaryPlace は座標を知らなくても探せるよう、盤面の方向を添える。
func summaryPlace(point, size int) string {
	if point < 0 {
		return "パス"
	}
	x, y := point%size, point/size
	horizontal, vertical := "", ""
	if x < size/3 {
		horizontal = "左"
	} else if x >= size-size/3 {
		horizontal = "右"
	}
	if y < size/3 {
		vertical = "上"
	} else if y >= size-size/3 {
		vertical = "下"
	}
	area := horizontal + vertical
	if area == "" {
		area = "中央"
	} else if horizontal == "" || vertical == "" {
		area += "側"
	}
	return area + "の" + adviceVertex(point, size)
}

// summaryMoveLead はいつ・誰が・どこへ打ったかを示す。
func summaryMoveLead(c *app.Controller, move *app.SummaryMove) string {
	if move.Point < 0 {
		return fmt.Sprintf("%d手目、%sはパスしました。", move.Turn, summaryActor(c, move.Color))
	}
	return fmt.Sprintf("%d手目、%sは%sに打ちました。", move.Turn, summaryActor(c, move.Color), summaryPlace(move.Point, c.Board.Size))
}

// summaryEffectText は同時に起きた効果と自分の危険を省かず、座標・石数・空きの変化を説明する。
func summaryEffectText(c *app.Controller, after *app.Board, facts moveFacts, point int, color app.Color) string {
	if point < 0 {
		return "石を置かず、盤上の石はそのままです。"
	}
	var parts []string
	if facts.taken > 0 {
		parts = append(parts, fmt.Sprintf("%sの石を%d個取り、盤から取り除きました。", summaryActor(c, color.Other()), facts.taken))
	}
	if facts.connected >= 2 {
		parts = append(parts, fmt.Sprintf("離れていた%d組の%s石を、一つのまとまりにつなぎました。", facts.connected, summaryColor(color)))
	} else if facts.connected == 1 {
		parts = append(parts, "打った石が近くの味方とつながりました。")
	}
	for _, g := range facts.rescued {
		parts = append(parts, fmt.Sprintf("%sを含む%s石%d個は、まとまりに接する空きが1か所から%dか所に増えました。", summaryPlace(g.point, after.Size), summaryColor(color), g.stones, g.newLiberties))
	}
	for _, g := range facts.attacked {
		parts = append(parts, fmt.Sprintf("%sを含む%s石%d個の空きを、%dか所から残り1か所に減らしました。", summaryPlace(g.point, after.Size), summaryColor(color.Other()), g.stones, g.oldLiberties))
	}
	if len(parts) == 0 {
		parts = append(parts, "打った石は、まだ近くの味方とはつながっていません。")
	}
	if len(facts.liberties) == 1 {
		last := facts.liberties[0]
		parts = append(parts, fmt.Sprintf("ただし、打った石を含む%s石%d個も、空きは%sの1か所だけです。", summaryColor(color), facts.stones, summaryPlace(last, after.Size)))
		if after.Legal(last) {
			parts = append(parts, "相手にそこをふさがれると、このまとまりは取られます。")
		} else {
			parts = append(parts, "ただし相手は今そこへ打てないため、すぐ取られるとは限りません。")
		}
	}
	return strings.Join(parts, "\n")
}

// summaryNextAction は説明した効果に結び付く確認を一つ選ぶ。
func summaryNextAction(facts moveFacts) string {
	switch {
	case len(facts.liberties) == 1:
		return "次局は、打った後の自分の石に空きが1か所しか残らないか、先に確かめましょう。"
	case len(facts.rescued) > 0:
		return "次局も、空きが残り1か所の味方を見つけたら、その空きを増やせる場所を探しましょう。"
	case facts.taken > 0:
		return "次局も、取れる相手の石を見つけたら、取った後の自分の石にも空きが残るか確かめましょう。"
	case facts.connected >= 2:
		return "次局も、離れた味方をつなぐ場所を探し、つながった石全体の空きを数えてみましょう。"
	case len(facts.attacked) > 0:
		return "次局は、相手の空きを残り1か所にした後、相手がどこへ打つと空きを増やせるか探しましょう。"
	default:
		return "次局は、打とうとする場所の上下左右を見て、味方とつながるか確かめましょう。"
	}
}

// summaryStageText は行動回数と代表手から区間の傾向を示す。石の個数を戦略の頻度として扱わない。
func summaryStageText(c *app.Controller, stage app.SummaryStage, name string) string {
	if stage.CPUMoves == 0 {
		return name + "：この区間にCPUの着手はありません。"
	}
	body := fmt.Sprintf("%s（%d～%d手目）\n", name, stage.From, stage.To)
	var actions []string
	for _, item := range []struct {
		count int
		text  string
	}{{stage.CaptureMoves, "あなたの石を取った手"}, {stage.RescueMoves, "取られそうな味方の空きを増やした手"}, {stage.ConnectMoves, "離れた味方をつないだ手"}} {
		if item.count > 0 {
			actions = append(actions, fmt.Sprintf("%sが%d回", item.text, item.count))
		}
	}
	if len(actions) > 0 {
		body += strings.Join(actions, "、") + "ありました。\n"
	} else {
		body += fmt.Sprintf("CPUの%d手のうち、隅や辺に打った手は%d回でした。\n", stage.CPUMoves, stage.EdgeMoves)
	}
	switch {
	case stage.CaptureMoves > max(stage.RescueMoves, stage.ConnectMoves):
		body += "相手の石を取り、周りを空ける狙いが考えられます。"
	case stage.RescueMoves > max(stage.CaptureMoves, stage.ConnectMoves):
		body += "取られそうな自分の石を守る狙いが考えられます。"
	case stage.ConnectMoves > max(stage.CaptureMoves, stage.RescueMoves):
		body += "味方をつないで、一つのまとまりにする狙いが考えられます。"
	case len(actions) > 0:
		body += "複数の行動が同じ回数あり、一つの狙いには絞れません。"
	case stage.EdgeMoves*2 >= stage.CPUMoves:
		body += "隅や辺を足がかりに石を増やす狙いが考えられます。"
	default:
		body += "これだけでは、一つの狙いには絞れません。"
	}
	if stage.Representative > 0 && stage.Representative <= len(c.SummarySession.Data.Moves) {
		m := &c.SummarySession.Data.Moves[stage.Representative-1]
		example := summaryMoveLead(c, m)
		place := fmt.Sprintf("%d手目の%sでは、", m.Turn, summaryPlace(m.Point, c.Board.Size))
		most := max(stage.CaptureMoves, stage.RescueMoves, stage.ConnectMoves)
		switch {
		case m.Captured > 0 && stage.CaptureMoves == most:
			example = place + fmt.Sprintf("あなたの石を%d個取りました。", m.Captured)
		case m.Saved > 0 && stage.RescueMoves == most:
			example = place + fmt.Sprintf("取られそうなCPUの石%d個の空きを増やしました。", m.Saved)
		case m.Connected > 0 && stage.ConnectMoves == most:
			example = place + fmt.Sprintf("離れたCPUの石%d組をつなぎました。", m.Connected+1)
		}
		body += "\n例：" + example
	}
	return body
}

// summaryComparisonText は同じ元の味方か取り石数を比較し、説明できないAIの優劣を補わない。
func summaryComparisonText(s summaryScene) string {
	if g := s.comparison; g != nil {
		text := fmt.Sprintf("着手前の%sを含む%s石%d個に注目すると、その石を含むまとまりの空きは、実際の手で%dか所、候補で%dか所になります。", summaryPlace(g.point, s.before.Size), summaryColor(s.before.ToMove), g.stones, g.playedLiberties, g.candidateLiberties)
		if g.playedLiberties == 1 && g.candidateLiberties > 1 {
			return text + "\n次局は、このように空きが残り1か所になる味方を見つけ、空きを増やす手を比べましょう。"
		}
		return text + "\n空きの数だけで安全とは決まりません。次局も、同じ味方の石を追って二つの手を比べてみましょう。"
	}
	if s.facts.taken != s.alternativeFacts.taken {
		return fmt.Sprintf("相手の石を取る数は、実際の手が%d個、候補が%d個です。\n取る数だけでは手のよさは決まりません。次局は、取れる石を見つけたら、取った後の自分の石にも空きが残るか確かめましょう。", s.facts.taken, s.alternativeFacts.taken)
	}
	if s.facts.connected != s.alternativeFacts.connected {
		return "二つの手では、つながる味方が違います。次局は、つなぐ前に、つながった石全体の空きを数えてみましょう。"
	}
	limit := "この一手の取り・つながり・空きの変化だけでは、AIが候補を選んだ理由を説明できません。"
	if s.alternative.Moves[len(s.alternative.Moves)-1].Point < 0 {
		return limit + "\n次局はパスの前に、空きが残り1か所の味方がないか確認してみましょう。"
	}
	return limit + "\n次局は、まず赤い印と青い印の上下左右を見て、味方と相手のどちらが近くにいるか比べてみましょう。"
}

// summaryPageContent は各ページの文章と根拠となる手を返す。未来の結果・戦略・評価不足を断定しない。
func summaryPageContent(c *app.Controller, page int) (title, body string, move *app.SummaryMove) {
	s := c.SummarySession.Data
	switch page {
	case 0:
		title = "結果と一局のハイライト"
		body = summaryResult(c) + fmt.Sprintf("\nあなたは%s、CPUは%sです。", summaryColor(c.Player), summaryColor(c.Player.Other()))
		body += fmt.Sprintf("\n\n全%d手。あなたは相手の石を%d個取り、CPUはあなたの石を%d個取りました。", len(c.Board.Moves), c.Board.Captured[c.Player], c.Board.Captured[c.Player.Other()])
		switch {
		case c.Board.Captured[c.Player] > 0 && c.Board.Captured[c.Player.Other()] > 0:
			body += "\nお互いに相手の石を取り合う場面があった一局です。"
		case c.Board.Captured[c.Player] > 0:
			body += "\nあなたが相手の石を取り、あなたの石は一度も取られなかった一局です。"
		case c.Board.Captured[c.Player.Other()] > 0:
			body += "\nCPUに石を取られる場面がありました。あなたが相手の石を取る場面はありませんでした。"
		default:
			body += "\n双方とも相手の石を取る場面はありませんでした。"
		}
		if c.Board.Rule == app.Jungo {
			body += "\n純碁は最後に残った石の数で決まります。取った石を別に加点しません。"
		}
		move = s.Highlight
		if move == nil {
			body += "\n\n注目の一手を選ぶための、着手前後のAI評価がまだ揃っていません。"
		} else {
			body += "\n\n" + summaryMoveLead(c, move)
			if scene, err := makeSummaryScene(c, move); err == nil {
				body += "\n" + summaryEffectText(c, scene.after, scene.facts, move.Point, move.Color)
			}
			body += "\nAIが見積もる得点差（目差）が、解析済みの範囲で最も大きく変わった場面です。この手だけが勝因・敗因とは限りません。"
		}
	case 1:
		title = "CPUの戦略（推定）"
		body = "手数を三等分して振り返ります。実際の行動から考えた狙いで、CPUの意図を断定するものではありません。\n\n"
		for i, name := range []string{"序盤", "中盤", "終盤"} {
			body += summaryStageText(c, s.Stages[i], name) + "\n\n"
		}
	case 2:
		title = "あなたの良かった場面"
		move = s.GoodMove
		if move == nil {
			body = "紹介する場面を選ぶためのAI評価と盤面の根拠がまだ揃っていません。良い手がなかったという意味ではありません。"
			break
		}
		body = summaryMoveLead(c, move)
		scene, err := makeSummaryScene(c, move)
		if err != nil {
			body += "\n対象局面を再現できません。結果画面へ戻ってください。"
			break
		}
		body += "\n" + summaryEffectText(c, scene.after, scene.facts, move.Point, move.Color)
		switch {
		case len(scene.facts.rescued) > 0:
			body += "\n取られそうな味方の空きを増やせた点が良かったところです。その後も取られない保証ではありません。"
		case scene.facts.taken > 0:
			body += "\n相手の石を取り除き、その場所を空けられた点が良かったところです。"
		case scene.facts.connected >= 2:
			body += "\n味方を一つにして、まとまり全体で空きを使えるようにした点が良かったところです。"
		default:
			body += "\nAIもこの手を候補の先頭に挙げています。ただし、取りや守りの具体的なよさまでは、この一手から説明できません。"
		}
		body += "\n\n" + summaryNextAction(scene.facts)
	case 3:
		title, body = techniquePageContent(c)
	}
	return
}
