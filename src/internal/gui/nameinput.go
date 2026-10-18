// 作成日: 2026-10-18。日本語IMEと文字編集を扱う。変換中のキーを通常編集と二重処理しない。
package gui

import (
	"image"
	"math"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/exp/textinput"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/rivo/uniseg"
)

// clusterBounds は表示上の文字の直前・直後の境界を返す。
func clusterBounds(value string, position int) (int, int) {
	previous, next := 0, len(value)
	clusters := uniseg.NewGraphemes(value)
	for clusters.Next() {
		start, end := clusters.Positions()
		if end <= position {
			previous = start
		}
		if end > position {
			next = end
			break
		}
	}
	return previous, next
}

// editName は選択範囲と日本語・結合文字を壊さず編集する。範囲はFieldのUTF-8境界を前提とする。
func editName(value string, start, end int, key ebiten.Key) (string, int) {
	start = max(0, min(len(value), start))
	end = max(start, min(len(value), end))
	previous, _ := clusterBounds(value, start)
	_, next := clusterBounds(value, end)
	switch key {
	case ebiten.KeyBackspace:
		if start == end {
			start = previous
		}
		return value[:start] + value[end:], start
	case ebiten.KeyDelete:
		if start == end {
			end = next
		}
		return value[:start] + value[end:], start
	case ebiten.KeyLeft:
		if start != end {
			return value, start
		}
		return value, previous
	case ebiten.KeyRight:
		if start != end {
			return value, end
		}
		return value, next
	case ebiten.KeyHome:
		return value, 0
	case ebiten.KeyEnd:
		return value, len(value)
	}
	return value, start
}

// repeated は長押し時も一定間隔で編集する。
func repeated(key ebiten.Key) bool {
	n := inpututil.KeyPressDuration(key)
	return n == 1 || n > 30 && n%3 == 0
}

// resetNameInput は前の入力セッションを終了して、エラー状態も新しくする。
func (g *window) resetNameInput(value string) {
	g.field.Blur()
	g.field = textinput.Field{}
	g.field.SetTextAndSelection(value, len(value), len(value))
	g.field.Focus()
	g.inputScroll = 0
}

// caretPosition は変換中文字を含めたカーソルのバイト位置を返す。
func (g *window) caretPosition() int {
	start, _ := g.field.Selection()
	if offset, _, ok := g.field.CompositionSelection(); ok {
		start += offset
	}
	return max(0, min(len(g.field.TextForRendering()), start))
}

// updateNameInput はIMEが処理したフレームでは追加のキー操作を止める。
func (g *window) updateNameInput() bool {
	g.face.Size = 23
	value := g.field.TextForRendering()
	cursor := g.caretPosition()
	x := 365 + int(math.Max(0, math.Min(450, text.Advance(value[:cursor], g.face)-g.inputScroll)))
	handled, err := g.field.HandleInputWithBounds(image.Rect(x, 350, x+1, 382))
	if err != nil {
		g.dialog.problem = "文字入力を開始できません。入力欄を開き直してください。"
		return true
	}
	value = g.field.Text()
	if g.field.UncommittedTextLengthInBytes() == 0 && utf8.RuneCountInString(value) > 32 {
		runes := []rune(value)
		value = string(runes[:32])
		g.field.SetTextAndSelection(value, len(value), len(value))
		g.dialog.problem = "入力は32文字までです。"
	}
	if handled || g.field.UncommittedTextLengthInBytes() > 0 {
		return true
	}
	if (ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight)) && inpututil.IsKeyJustPressed(ebiten.KeyA) {
		g.field.SetSelection(0, len(value))
		return true
	}
	for _, key := range []ebiten.Key{ebiten.KeyBackspace, ebiten.KeyDelete, ebiten.KeyLeft, ebiten.KeyRight, ebiten.KeyHome, ebiten.KeyEnd} {
		if repeated(key) {
			start, end := g.field.Selection()
			updated, position := editName(value, start, end, key)
			g.field.SetTextAndSelection(updated, position, position)
			return true
		}
	}
	return false
}

// drawNameInput は選択・変換下線・カーソルを描き、長い名前を欄内で横スクロールする。
func (g *window) drawNameInput(screen *ebiten.Image) {
	if g.inputCanvas == nil {
		g.inputCanvas = ebiten.NewImage(480, 58)
	}
	g.inputCanvas.Fill(tint(background))
	g.face.Size = 23
	value := g.field.TextForRendering()
	cursor := g.caretPosition()
	advance := text.Advance(value[:cursor], g.face)
	g.inputScroll = math.Max(0, advance-440)
	start, end := g.field.Selection()
	composition := g.field.UncommittedTextLengthInBytes()
	if composition == 0 && end > start {
		x := 15 + text.Advance(value[:start], g.face) - g.inputScroll
		width := text.Advance(value[start:end], g.face)
		vector.DrawFilledRect(g.inputCanvas, float32(x), 8, float32(width), 39, tint(0x35516d), false)
	}
	g.label(g.inputCanvas, value, 15-g.inputScroll, 10, 23, foreground)
	if composition > 0 {
		x := 15 + text.Advance(value[:start], g.face) - g.inputScroll
		width := text.Advance(value[start:start+composition], g.face)
		vector.StrokeLine(g.inputCanvas, float32(x), 45, float32(x+width), 45, 1, tint(accent), false)
	}
	if g.frames%60 < 40 {
		x := float32(15 + advance - g.inputScroll)
		vector.StrokeLine(g.inputCanvas, x, 11, x, 45, 1, tint(accent), false)
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(350, 342)
	screen.DrawImage(g.inputCanvas, op)
	vector.StrokeRect(screen, 350, 342, 480, 58, 1, tint(accent), false)
	g.actions = append(g.actions, hit{image.Rect(350, 342, 830, 400), func() {
		if g.field.UncommittedTextLengthInBytes() > 0 {
			return
		}
		g.face.Size = 23
		x, _ := ebiten.CursorPosition()
		target := float64(x-365) + g.inputScroll
		value := g.field.Text()
		position := len(value)
		clusters := uniseg.NewGraphemes(value)
		for clusters.Next() {
			start, end := clusters.Positions()
			left, right := text.Advance(value[:start], g.face), text.Advance(value[:end], g.face)
			if target < (left+right)/2 {
				position = start
				break
			}
		}
		g.field.SetSelection(position, position)
	}})
}
