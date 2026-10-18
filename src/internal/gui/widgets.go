// 作成日: 2026-10-18。描画とヒット領域を共通化する。ダイアログは背面の操作を受け付けない。
package gui

import (
	"image"
	"image/color"
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	background uint32 = 0x111d2b
	panel      uint32 = 0x1c2c3f
	surface    uint32 = 0x283c53
	foreground uint32 = 0xeaf1f8
	muted      uint32 = 0xa8b9cc
	accent     uint32 = 0x60dbc0
	warning    uint32 = 0xffc879
)

type hit struct {
	rect   image.Rectangle
	action func()
}
type choiceMenu struct {
	title            string
	options          []string
	selected, offset int
	choose           func(int)
}
type dialog struct {
	title, body, problem string
	input                bool
	submit               func(string) error
	confirm              func() error
	choices              []dialogChoice
}

type dialogChoice struct {
	title  string
	choose func() error
}

// tint は固定パレット値を不透明色へ変換する。
func tint(rgb uint32) color.RGBA {
	return color.RGBA{R: uint8(rgb >> 16), G: uint8(rgb >> 8), B: uint8(rgb), A: 255}
}

// rect は不透明な面を描画する。
func rect(screen *ebiten.Image, r image.Rectangle, c uint32) {
	vector.DrawFilledRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), tint(c), false)
}

// label は専用フォントで日本語を表示する。
func (g *window) label(screen *ebiten.Image, value string, x, y, size float64, c uint32) {
	g.face.Size = size
	op := &text.DrawOptions{}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(tint(c))
	op.LineSpacing = size * 1.5
	text.Draw(screen, value, g.face, op)
}

// fit は1行ラベルが領域を越える場合だけ末尾を省略する。
func (g *window) fit(value string, width, size float64) string {
	g.face.Size = size
	if w, _ := text.Measure(value, g.face, size*1.5); w <= width {
		return value
	}
	runes := []rune(value)
	for len(runes) > 0 {
		runes = runes[:len(runes)-1]
		s := string(runes) + "…"
		if w, _ := text.Measure(s, g.face, size*1.5); w <= width {
			return s
		}
	}
	return ""
}

// wrap は日本語の文章を表示幅で折り返す。
func (g *window) wrap(value string, width, size float64, maxLines int) string {
	g.face.Size = size
	lines := []string{}
	line := ""
	for _, r := range value {
		if r == '\n' {
			lines = append(lines, line)
			line = ""
			continue
		}
		next := line + string(r)
		if w, _ := text.Measure(next, g.face, size*1.5); w > width && line != "" {
			lines = append(lines, line)
			line = string(r)
		} else {
			line = next
		}
	}
	lines = append(lines, line)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines[maxLines-1] = g.fit(lines[maxLines-1]+"…", width, size)
	}
	return strings.Join(lines, "\n")
}

// button は有効なボタンだけをクリック対象へ登録する。
func (g *window) button(screen *ebiten.Image, r image.Rectangle, title string, enabled bool, action func()) {
	c, fg := surface, foreground
	x, y := ebiten.CursorPosition()
	if enabled && image.Pt(x, y).In(r) {
		c = 0x35516d
	}
	if !enabled {
		c = 0x202e3e
		fg = 0x75879a
	}
	rect(screen, r, c)
	g.label(screen, g.fit(title, float64(r.Dx()-24), 20), float64(r.Min.X+12), float64(r.Min.Y+(r.Dy()-30)/2), 20, fg)
	if enabled {
		g.actions = append(g.actions, hit{r, action})
	}
}

// tabButton は選択中のタブを枠と下線で示す。
func (g *window) tabButton(screen *ebiten.Image, r image.Rectangle, title string, selected bool, action func()) {
	g.button(screen, r, title, true, action)
	if selected {
		vector.StrokeRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 2, tint(accent), false)
		rect(screen, image.Rect(r.Min.X, r.Max.Y-4, r.Max.X, r.Max.Y), accent)
	}
}

// combo は選択値と選択リストを結ぶ。
func (g *window) combo(screen *ebiten.Image, r image.Rectangle, title string, options []string, selected int, enabled bool, choose func(int)) {
	value := "選択してください"
	if selected >= 0 && selected < len(options) {
		value = options[selected]
	}
	g.button(screen, r, value+"  ▼", enabled, func() {
		offset := 0
		if selected > 7 {
			offset = (selected / 8) * 8
		}
		g.menu = &choiceMenu{title: title, options: options, selected: selected, offset: offset, choose: choose}
	})
	if selected < 0 {
		vector.StrokeRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 2, tint(warning), false)
	}
}

// dismiss は入力フォーカスとモーダルを解除する。
func (g *window) dismiss() { g.field.Blur(); g.dialog = nil; g.menu = nil }

// overlay は背面操作を取り除く。
func (g *window) overlay(screen *ebiten.Image) {
	vector.DrawFilledRect(screen, 0, 0, 1180, 800, color.RGBA{A: 180}, false)
	g.actions = g.actions[:0]
}

// drawMenu は8件ずつ選択肢を表示する。大きい段位一覧も画面内に収める。
func (g *window) drawMenu(screen *ebiten.Image) {
	m := g.menu
	g.overlay(screen)
	rect(screen, image.Rect(330, 95, 850, 720), panel)
	g.label(screen, m.title, 355, 115, 25, foreground)
	end := min(len(m.options), m.offset+8)
	for i := m.offset; i < end; i++ {
		i := i
		y := 175 + (i-m.offset)*50
		title := m.options[i]
		if i == m.selected {
			title = "● " + title
		}
		g.button(screen, image.Rect(355, y, 825, y+42), title, true, func() { g.menu = nil; m.choose(i) })
	}
	g.button(screen, image.Rect(355, 595, 495, 640), "前の項目", m.offset > 0, func() { m.offset = max(0, m.offset-8) })
	g.button(screen, image.Rect(505, 595, 645, 640), "次の項目", end < len(m.options), func() { m.offset += 8 })
	g.button(screen, image.Rect(655, 655, 825, 700), "閉じる", true, g.dismiss)
}

// drawDialog は日本語IME入力と確認操作を提供する。
func (g *window) drawDialog(screen *ebiten.Image) {
	d := g.dialog
	g.overlay(screen)
	if len(d.choices) > 0 {
		g.drawChoiceDialog(screen, d)
		return
	}
	rect(screen, image.Rect(325, 215, 855, 590), panel)
	g.label(screen, d.title, 350, 237, 27, foreground)
	g.label(screen, d.body, 350, 285, 18, muted)
	if d.input {
		g.drawNameInput(screen)
		g.label(screen, "1～32文字。Enterで変換を確定してから登録します。", 350, 413, 15, muted)
	}
	g.label(screen, d.problem, 350, 455, 16, warning)
	g.button(screen, image.Rect(350, 520, 550, 567), "キャンセル", true, g.dismiss)
	g.button(screen, image.Rect(630, 520, 830, 567), "確定する", g.field.UncommittedTextLengthInBytes() == 0 || !d.input, func() {
		var err error
		if d.input {
			name := g.field.Text()
			if utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 32 || strings.TrimSpace(name) != name {
				d.problem = "前後に空白を入れず、1～32文字で入力してください。"
				return
			}
			err = d.submit(name)
		} else if d.confirm != nil {
			err = d.confirm()
		}
		if err != nil {
			d.problem = "処理できませんでした。重複する名前や保存先を確認してください。"
			return
		}
		g.dismiss()
	})
}

// chooseDialog は移動成功時だけ選択ウィンドウを閉じ、失敗時は再試行できるよう保持する。
func (g *window) chooseDialog(index int) {
	d := g.dialog
	if d == nil || index < 0 || index >= len(d.choices) {
		return
	}
	if err := d.choices[index].choose(); err != nil {
		d.problem = "移動できませんでした。保存先を確認して再試行してください。"
		return
	}
	g.dismiss()
}

// drawChoiceDialog は終了後の3つの行動を背面操作から分離して表示する。
func (g *window) drawChoiceDialog(screen *ebiten.Image, d *dialog) {
	rect(screen, image.Rect(325, 170, 855, 650), panel)
	g.label(screen, d.title, 350, 196, 27, foreground)
	g.label(screen, g.wrap(d.body, 475, 18, 2), 350, 249, 18, muted)
	for i, choice := range d.choices {
		y := 330 + i*68
		g.button(screen, image.Rect(350, y, 830, y+54), choice.title, true, func() { g.chooseDialog(i) })
	}
	g.label(screen, g.wrap(d.problem, 475, 15, 2), 350, 529, 15, warning)
	g.button(screen, image.Rect(685, 589, 830, 629), "閉じる", true, g.dismiss)
}
