// 作成日: 2026-10-19。着目範囲に限定して連結・AI推定を描く。着手番号と全体盤の可読性を保つ。
package gui

import (
	"image"
	"math"

	"github.com/Tremendous1192/PlayGo/internal/app"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// readColor はAI推定の安定・注意・未確定を緑・赤・灰色へ写す。
func readColor(strength readStrength) uint32 {
	switch strength {
	case readStrong:
		return 0x00866b
	case readRisk:
		return 0xc03d4c
	default:
		return 0x858379
	}
}

// drawReadSegment は色と実線・破線・点線を併用する。
func drawReadSegment(screen *ebiten.Image, x1, y1, x2, y2 float64, strength readStrength) {
	length := math.Hypot(x2-x1, y2-y1)
	if length < 1 {
		return
	}
	dash, gap := length, 0.0
	if strength == readRisk {
		dash, gap = 6, 3
	}
	if strength == readUnknown {
		dash, gap = 2, 3
	}
	for start := 0.0; start < length; start += dash + gap {
		end := min(start+dash, length)
		ax, ay := float32(x1+(x2-x1)*start/length), float32(y1+(y2-y1)*start/length)
		bx, by := float32(x1+(x2-x1)*end/length), float32(y1+(y2-y1)*end/length)
		vector.StrokeLine(screen, ax, ay, bx, by, 4, tint(0xfff6dd), true)
		vector.StrokeLine(screen, ax, ay, bx, by, 2, tint(readColor(strength)), true)
	}
}

// visibleLink は共有呼吸点と両側の石が範囲内にある経路を1つ選び、枠外への線を作らない。
func visibleLink(p *diagramPosition, view image.Rectangle, link readLink) (a, bridge, b int, ok bool) {
	for _, bridge = range link.bridges {
		if !image.Pt(bridge%p.size, bridge/p.size).In(view) {
			continue
		}
		a, b = -1, -1
		for _, point := range app.Neighbors(bridge, p.size) {
			if !image.Pt(point%p.size, point/p.size).In(view) {
				continue
			}
			if p.groupAt[point] == link.a {
				a = point
			}
			if p.groupAt[point] == link.b {
				b = point
			}
		}
		if a >= 0 && b >= 0 {
			return a, bridge, b, true
		}
	}
	return 0, 0, 0, false
}

// drawTactics は石のつながりとAIの生存見込みを示す。left/topは盤全体の原点。
func (g *window) drawTactics(screen *ebiten.Image, p *diagramPosition, view image.Rectangle, left, top, step, radius float64) {
	xy := func(point int) (float64, float64) {
		return left + float64(point%p.size)*step, top + float64(point/p.size)*step
	}
	for _, link := range p.read.links {
		a, bridge, b, ok := visibleLink(p, view, link)
		if !ok {
			continue
		}
		x, y := xy(bridge)
		for _, point := range []int{a, b} {
			px, py := xy(point)
			dx, dy := x-px, y-py
			drawReadSegment(screen, px+dx*radius/step, py+dy*radius/step, x, y, link.strength)
		}
	}
	for y := view.Min.Y; y < view.Max.Y; y++ {
		for x := view.Min.X; x < view.Max.X; x++ {
			point := y*p.size + x
			id := p.groupAt[point]
			if id < 0 {
				continue
			}
			px, py := xy(point)
			for _, n := range []int{point + 1, point + p.size} {
				if n >= len(p.cells) || n == point+1 && n/p.size != y || !image.Pt(n%p.size, n/p.size).In(view) || p.groupAt[n] != id {
					continue
				}
				nx, ny := xy(n)
				dx, dy := nx-px, ny-py
				vector.StrokeLine(screen, float32(px+dx*radius/step), float32(py+dy*radius/step), float32(nx-dx*radius/step), float32(ny-dy*radius/step), 3, tint(0x13728a), true)
			}
			strength := readUnknown
			if id < len(p.read.life) {
				strength = p.read.life[id]
			}
			vector.StrokeCircle(screen, float32(px), float32(py), float32(radius*.8), 2, tint(readColor(strength)), true)

		}
	}
}

// drawTacticsLegend は連結済みの石とAI推定を区別し、色だけに意味を依存させない。
func (g *window) drawTacticsLegend(screen *ebiten.Image, p *diagramPosition, x, y float64) {
	g.label(screen, "輪＝AI生存見込み：緑 安定／赤 注意／灰 未確定", x, y, 13, foreground)
	g.label(screen, "線＝連絡：青 連結済／緑 強め／赤 破線 注意／灰 点線 未確定", x, y+19, 12, foreground)
	g.label(screen, g.tacticsStatus(p), x, y+38, 13, muted)
}
