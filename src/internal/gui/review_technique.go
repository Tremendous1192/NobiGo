// 作成日: 2026-10-25。今回の局面を使う教材と仮想手順を表示する。描画は保存せず、次のUpdateで紹介履歴だけを記録する。
package gui

import (
	"fmt"
	"image"
	"strings"

	"github.com/Tremendous1192/NobiGo/internal/app"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// techniqueName は保存IDを初心者向けの名前へ変換する。
func techniqueName(id app.TechniqueID) string {
	switch id {
	case app.TechniqueCapture:
		return "アタリの石を取る"
	case app.TechniqueEscape:
		return "アタリから逃げる"
	case app.TechniqueConnect:
		return "ツギ — 味方をつなぐ"
	case app.TechniqueAtari:
		return "アタリをかける"
	case app.TechniqueDoubleAtari:
		return "両アタリ"
	case app.TechniqueLadder:
		return "シチョウ"
	case app.TechniqueNet:
		return "ゲタ"
	case app.TechniqueSnapback:
		return "ウッテガエシ"
	default:
		return "今回のテクニック"
	}
}

// techniquePageContent は技術の目的・今回の一手・確認した範囲・次局の行動を説明する。
func techniquePageContent(c *app.Controller) (string, string) {
	m := c.ChooseTechnique()
	if m == nil {
		if c.SummarySession != nil && !c.SummarySession.Ready {
			return "今回のテクニック", "今回の棋譜から、説明できる手筋を探しています。\n準備が終わるまで、他のページも読めます。"
		}
		return "今回のテクニック", "今回は紹介できる場面が見つかりませんでした\n\n紹介履歴は更新しません。"
	}
	intro := "はじめて紹介する技術です。"
	for _, r := range c.Profile.Techniques {
		if r.ID == m.ID {
			intro = "以前に紹介した技術を、今回の局面で復習します。"
		}
	}
	// 保存直後に文面が切り替わらないよう、本文は概要ページのキャッシュで固定する。
	body := intro + fmt.Sprintf("\n\n%d手目の直前、%sの番です。\n%sに打つ場面を見てみましょう。", m.Turn, summaryActor(c, m.Color), summaryPlace(m.Line[0], c.Board.Size))
	switch m.ID {
	case app.TechniqueCapture:
		body += fmt.Sprintf("\n\n相手の石につながる空きが、残り1か所です。そこをふさぐと、相手の石%d個を取れます。", m.Stones)
	case app.TechniqueEscape:
		body += fmt.Sprintf("\n\n味方の石%d個は、空きが残り1か所のアタリでした。この手で、そのまとまりの空きが%dか所に増えます。今すぐ取られる状態を解消する手です。", m.Stones, m.AfterLiberties)
	case app.TechniqueConnect:
		body += fmt.Sprintf("\n\n離れていた味方%d組を、一つにつなぐ手です。つながった石は、まとまり全体の空きを使えます。これをツギと呼びます。", m.Groups)
	case app.TechniqueAtari:
		body += "\n\n相手の石の空きを残り1か所にする手を、アタリと呼びます。相手は逃げたり、周りの石を取ったりして守れることがあります。アタリだけで捕獲が決まるわけではありません。"
	case app.TechniqueDoubleAtari:
		body += "\n\n離れた相手のまとまりを、同時にアタリにする手です。この局面では、相手がどちらを守っても、次にもう一方を取れることを確認しました。手順の図で、相手が逃げた後の取り方を見てください。"
	case app.TechniqueLadder:
		body += "\n\nアタリを続け、相手の逃げる方向を狭めながら追う手筋です。この局面では、逃げ方を調べて対象の石を取れることを確認しました。途中に相手の味方がいると逃げられるため、追う先まで見ることが大切です。"
	case app.TechniqueNet:
		body += "\n\nすぐにアタリにせず、逃げ道を囲って取る手筋です。この局面では、相手の逃げ方を変えても、囲いの中で取れることを確認しました。図では相手が逃げた場合の一例を示します。"
	case app.TechniqueSnapback:
		body += fmt.Sprintf("\n\nいったん味方を取らせ、同じ場所に打ち直して、相手の石%d個を取り返す手筋です。図は相手が取った場合の手順です。相手が取ることを強制する手ではありません。直後の取り返しが禁止されるコウとは区別しています。", m.Stones)
	}
	body += "\n\n右の「一手進む」で、石と空きがどう変わるか確かめましょう。次局でも、同じ形を見つけたら周りの石まで確認してください。"
	body += "\n\n図は教材用の仮想手順です。実戦の続きや、盤全体での最善手を保証するものではありません。"
	return techniqueName(m.ID), body
}

// refreshTechniqueBoard は実戦の着手前から教材の選択手までを独立して再生する。
func refreshTechniqueBoard(c *app.Controller, d *summaryDialog) {
	m := d.lesson
	d.board = nil
	if m == nil {
		return
	}
	b, err := c.Snapshot(m.Turn - 1)
	if err != nil {
		d.boardCaption = "対象局面を再現できません。"
		return
	}
	d.techniqueStep = max(0, min(len(m.Line), d.techniqueStep))
	for _, point := range m.Line[:d.techniqueStep] {
		if b.Play(point) != nil {
			d.boardCaption = "教材の手順を再現できません。"
			return
		}
	}
	d.board = b
	d.boardCaption = fmt.Sprintf("%d手目の直前から · 教材 %d / %d手", m.Turn, d.techniqueStep, len(m.Line))
}

// changeTechniqueStep は教材だけを一手進め、実棋譜と紹介履歴を維持する。
func (d *summaryDialog) changeTechniqueStep(delta int) {
	if d.lesson == nil {
		return
	}
	d.techniqueStep = max(0, min(len(d.lesson.Line), d.techniqueStep+delta))
}

// drawTechniqueMarks は仮想手順の番号と対象石だけを表示し、実戦の着手番号と混同させない。
func (g *window) drawTechniqueMarks(screen *ebiten.Image, d *summaryDialog, left, top, step float64) {
	b, m := d.board, d.lesson
	for _, point := range m.Targets {
		if point < 0 || point >= len(b.Cells) || b.Cells[point] == app.Empty {
			continue
		}
		x, y := left+float64(point%b.Size)*step, top+float64(point/b.Size)*step
		vector.StrokeRect(screen, float32(x-step*.47), float32(y-step*.47), float32(step*.94), float32(step*.94), 2, tint(0x805b08), true)
	}
	numbers := make(map[int]int)
	for i, point := range m.Line[:d.techniqueStep] {
		if point >= 0 && b.Cells[point] != app.Empty {
			numbers[point] = i + 1
		}
	}
	if d.techniqueStep == 0 {
		numbers[m.Line[0]] = 1
	}
	for point, number := range numbers {
		x, y := left+float64(point%b.Size)*step, top+float64(point/b.Size)*step
		vector.StrokeCircle(screen, float32(x), float32(y), float32(step*.46), 2, tint(0x125bb4), true)
		ink := uint32(0x18222d)
		if b.Cells[point] == app.Black {
			ink = 0xffffff
		}
		g.label(screen, fmt.Sprint(number), x-4, y-9, 13, ink)
	}
	caption := "青い印・数字：教材の手順　枠：対象の石"
	if d.techniqueStep > 0 && m.Line[d.techniqueStep-1] < 0 {
		caption += "\nこの手順はパスです。"
	}
	g.label(screen, caption, 655, 618, 15, muted)
	g.button(screen, image.Rect(655, 654, 795, 688), "一手戻る", d.techniqueStep > 0, func() { d.changeTechniqueStep(-1) })
	g.button(screen, image.Rect(805, 654, 945, 688), "最初へ", d.techniqueStep > 0, func() { d.changeTechniqueStep(-d.techniqueStep) })
	g.button(screen, image.Rect(955, 654, 1105, 688), "一手進む", d.techniqueStep < len(m.Line), func() { d.changeTechniqueStep(1) })
}

// updateTechniqueSave は描画済み通知を一度だけ処理する。失敗後は明示的な再試行まで書き込まない。
func (g *window) updateTechniqueSave() {
	d := g.summary
	if g.previewMode || d == nil || !d.techniquePresented || d.saveAttempted || g.controller == nil {
		return
	}
	d.saveAttempted = true
	d.saveFailed = g.controller.RecordTechniqueShown() != nil
	if !d.saveFailed {
		g.updateSelectedProfile()
	}
}

// techniquePreviewID は指定された教材の合成画面だけを判定する。
func techniquePreviewID(mode string) app.TechniqueID {
	if !strings.HasPrefix(mode, "summary-technique-") {
		return ""
	}
	return app.TechniqueID(strings.TrimPrefix(mode, "summary-technique-"))
}
