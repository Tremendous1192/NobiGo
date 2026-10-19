// 作成日: 2026-10-19。AIの所有予測と合法な読み筋から局所のつながり・生存見込みを作る。確率・死活の保証ではない。
package gui

import (
	"cmp"
	"slices"

	"github.com/Tremendous1192/NobiGo/engine"
	"github.com/Tremendous1192/NobiGo/internal/app"
)

type readStrength uint8

const (
	readUnknown readStrength = iota
	readStrong
	readRisk
)

type readLink struct {
	a, b              int
	bridges           []int
	strength          readStrength
	tested, connected int
}

type tacticalRead struct {
	life              []readStrength
	links             []readLink
	variations, depth int
}

// sharedLinks は別々の同色連に共有される呼吸点を列挙する。形だけで強さを決めない。
func sharedLinks(p *diagramPosition) []readLink {
	var links []readLink
	indices := make(map[[2]int]int)
	for point, color := range p.cells {
		if color != app.Empty {
			continue
		}
		var groups []int
		for _, n := range app.Neighbors(point, p.size) {
			if id := p.groupAt[n]; id >= 0 && !slices.Contains(groups, id) {
				groups = append(groups, id)
			}
		}
		slices.Sort(groups)
		for i, a := range groups {
			for _, b := range groups[i+1:] {
				if p.cells[p.groups[a].stones[0]] != p.cells[p.groups[b].stones[0]] {
					continue
				}
				key := [2]int{a, b}
				index, ok := indices[key]
				if !ok {
					index = len(links)
					indices[key] = index
					links = append(links, readLink{a: a, b: b})
				}
				links[index].bridges = append(links[index].bridges, point)
			}
		}
	}
	return links
}

// ownershipLife は石の最も低い同色所有予測を使い、地の予測を生存確率へ換算しない。
func ownershipLife(stones []int, color app.Color, ownership []float64) readStrength {
	low, high := 1.0, -1.0
	for _, point := range stones {
		v := ownership[point]
		if color == app.White {
			v = -v
		}
		low, high = min(low, v), max(high, v)
	}
	if low >= .6 {
		return readStrong
	}
	if high <= -.6 {
		return readRisk
	}
	return readUnknown
}

// analyzeTactics は最大8候補・各16手を独立盤で再生する。低探索・違法・途切れた読みを強い連絡の証拠にしない。
func analyzeTactics(p *diagramPosition, a *engine.Analysis) tacticalRead {
	r := tacticalRead{life: make([]readStrength, len(p.groups)), links: sharedLinks(p)}
	if a == nil || !a.LocalRead || a.Partial || len(a.Ownership) != len(p.cells) {
		return r
	}
	stable := make([]int, len(p.groups))
	b, err := app.Replay(p.size, p.rule, p.moves, len(p.moves))
	if err != nil || !slices.Equal(b.Cells, p.cells) {
		return r
	}
	moves := slices.Clone(a.Moves)
	slices.SortStableFunc(moves, func(a, b engine.Candidate) int { return cmp.Compare(b.Visits, a.Visits) })
	for _, candidate := range moves[:min(8, len(moves))] {
		if candidate.Visits < 2 || len(candidate.PV) < 2 || candidate.PV[0] != candidate.Point || len(candidate.PVVisits) != len(candidate.PV) || len(candidate.Ownership) != len(p.cells) {
			continue
		}
		steps := min(16, len(candidate.PV))
		alive := make([]bool, len(p.groups))
		for i := range alive {
			alive[i] = true
		}
		attacked := make([]bool, len(r.links))
		played, valid := 0, true
		for i, point := range candidate.PV[:steps] {
			if candidate.PVVisits[i] < 2 {
				break
			}
			color := b.ToMove
			if err = b.Play(point); err != nil {
				valid = false
				break
			}
			played++
			for id, group := range p.groups {
				for _, stone := range group.stones {
					if b.Cells[stone] != p.cells[stone] {
						alive[id] = false
					}
				}
			}
			for j, link := range r.links {
				if color != p.cells[p.groups[link.a].stones[0]] && slices.Contains(link.bridges, point) {
					attacked[j] = true
				}
			}
		}
		if valid && played >= 2 {
			r.variations++
			r.depth = max(r.depth, played)
			for i, group := range p.groups {
				prediction := ownershipLife(group.stones, p.cells[group.stones[0]], candidate.Ownership)
				if !alive[i] || prediction == readRisk {
					r.life[i] = readRisk
				} else if prediction == readStrong {
					stable[i]++
				}
			}
			for j := range r.links {
				link := &r.links[j]
				if !attacked[j] {
					continue
				}
				link.tested++
				sa, sb := p.groups[link.a].stones, p.groups[link.b].stones
				group, _ := b.Group(sa[0])
				if alive[link.a] && alive[link.b] && slices.Contains(group, sb[0]) && ownershipLife(sa, p.cells[sa[0]], candidate.Ownership) == readStrong && ownershipLife(sb, p.cells[sb[0]], candidate.Ownership) == readStrong {
					link.connected++
				} else {
					link.strength = readRisk
				}
			}
		}
		if played > 0 && !b.Undo(played) {
			return tacticalRead{life: make([]readStrength, len(p.groups)), links: sharedLinks(p)}
		}
	}
	// 所有予測だけでは探索不足を識別できないため、生存の安定表示にも複数の合法変化を求める。
	for i, group := range p.groups {
		if r.life[i] != readRisk && r.variations >= 2 && stable[i] == r.variations && ownershipLife(group.stones, p.cells[group.stones[0]], a.Ownership) == readStrong {
			r.life[i] = readStrong
		}
	}
	for i := range r.links {
		link := &r.links[i]
		// 1本だけの成功を切断されにくさと断定せず、複数の探索変化で連絡を確認する。
		if link.tested >= 2 && link.connected == link.tested {
			link.strength = readStrong
		}
	}
	return r
}

// refreshTactics は表示局面の確定解析だけを取り込み、同じ結果を毎フレーム再計算しない。
func (g *window) refreshTactics(p *diagramPosition, turn int) {
	var a *engine.Analysis
	if turn >= 0 && turn < len(g.controller.Evaluations) {
		a = g.controller.Evaluations[turn]
	}
	if p.readSource == a && p.readReady {
		return
	}
	p.readSource, p.readReady = a, true
	p.read = analyzeTactics(p, a)
}

// tacticsStatus は解析待ち・失敗・途中結果と実際に使った読みの長さを区別する。
func (g *window) tacticsStatus(p *diagramPosition) string {
	if a := p.readSource; a != nil && a.LocalRead && !a.Partial {
		return "AIの予測です。石の安全を保証するものではありません"
	}
	if g.controller.Err != nil {
		return "AI解析失敗：閉じて再試行できます"
	}
	if g.controller.Busy {
		return "AI解析中：青線の石はつながっています"
	}
	return "AI未確定：青線の石はつながっています"
}
