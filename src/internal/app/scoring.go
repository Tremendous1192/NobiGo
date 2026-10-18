// 作成日: 2026-10-18。確定する死石・セキの指定からルール別の得点を計算する。
package app

import (
	"errors"
	"math"
	"slices"
)

type region struct {
	points   []int
	boundary map[int]bool
	color    Color
}

type Score struct {
	Black     float64 `json:"black"`
	White     float64 `json:"white"`
	Margin    float64 `json:"margin"`
	Winner    Color   `json:"winner"`
	NoResult  bool    `json:"noResult"`
	Stones    [3]int  `json:"stones"`
	Territory [3]int  `json:"territory"`
	Captures  [3]int  `json:"captures"`
	Dead      [3]int  `json:"dead"`
	Komi      float64 `json:"komi"`
	Owner     []Color `json:"owner"`
	DeadMask  []bool  `json:"deadMask"`
	SekiMask  []bool  `json:"sekiMask"`
}

// regions は空点領域と隣接する石の連を列挙する。
func regions(cells []Color, size int) (groups [][]int, groupAt []int, areas []region) {
	groupAt = make([]int, len(cells))
	for i := range groupAt {
		groupAt[i] = -1
	}
	for p, c := range cells {
		if c != Empty && groupAt[p] < 0 {
			stones, _ := group(cells, size, p)
			id := len(groups)
			groups = append(groups, stones)
			for _, n := range stones {
				groupAt[n] = id
			}
		}
	}
	seen := make([]bool, len(cells))
	for p, c := range cells {
		if c != Empty || seen[p] {
			continue
		}
		r := region{points: []int{p}, boundary: make(map[int]bool)}
		seen[p] = true
		mixed := false
		for i := 0; i < len(r.points); i++ {
			for _, n := range Neighbors(r.points[i], size) {
				if cells[n] == Empty {
					if !seen[n] {
						seen[n] = true
						r.points = append(r.points, n)
					}
				} else {
					r.boundary[groupAt[n]] = true
					if r.color == Empty {
						r.color = cells[n]
					} else if r.color != cells[n] {
						mixed = true
					}
				}
			}
		}
		if mixed {
			r.color = Empty
		}
		areas = append(areas, r)
	}
	return
}

// passAlive は二つ以上の独立した眼が残る連を反復除去法で確認する。
func passAlive(cells []Color, size int, groups [][]int, at []int, areas []region) []bool {
	alive := make([]bool, len(groups))
	for i := range alive {
		alive[i] = true
	}
	for changed := true; changed; {
		changed = false
		for id, stones := range groups {
			if !alive[id] {
				continue
			}
			eyes := 0
			for _, r := range areas {
				if r.color != cells[stones[0]] {
					continue
				}
				valid := true
				for other := range r.boundary {
					if !alive[other] {
						valid = false
						break
					}
				}
				if !valid {
					continue
				}
				for _, p := range r.points {
					adjacent := false
					for _, n := range Neighbors(p, size) {
						if at[n] == id {
							adjacent = true
							break
						}
					}
					if !adjacent {
						valid = false
						break
					}
				}
				if valid {
					eyes++
				}
			}
			if eyes < 2 {
				alive[id] = false
				changed = true
			}
		}
	}
	return alive
}

// proposedSeki は独立した活きを証明できず中立点を共有する連を修正可能な候補にする。
func proposedSeki(cells []Color, size int, groups [][]int, at []int, areas []region) []bool {
	alive := passAlive(cells, size, groups, at, areas)
	seki := make([]bool, len(groups))
	// 静的な共有ダメだけではセキを断定できないため、確定画面で手動上書きを受け付ける。
	for _, r := range areas {
		if r.color != Empty {
			continue
		}
		colors := [3]bool{}
		for id := range r.boundary {
			if !alive[id] {
				colors[cells[groups[id][0]]] = true
			}
		}
		if colors[Black] && colors[White] {
			for id := range r.boundary {
				if !alive[id] {
					seki[id] = true
				}
			}
		}
	}
	return seki
}

// ScoreBoard は元の盤面を変更せず採点する。deadは連単位に拡張し、sekiは明示falseで候補を解除できる。
func ScoreBoard(b *Board, dead, seki map[int]bool) (Score, error) {
	s := Score{Komi: Komi(b.Rule), Captures: b.Captured, Owner: make([]Color, len(b.Cells)), DeadMask: make([]bool, len(b.Cells)), SekiMask: make([]bool, len(b.Cells)), NoResult: b.NoResult}
	if b.Rule == Jungo && (len(dead) > 0 || len(seki) > 0) {
		return s, errors.New("jungo does not remove dead stones during scoring")
	}
	cells := slices.Clone(b.Cells)
	for p, marked := range dead {
		if !marked {
			continue
		}
		if p < 0 || p >= len(cells) || b.Cells[p] == Empty {
			return s, errors.New("invalid dead-stone selection")
		}
		stones, _ := b.Group(p)
		for _, n := range stones {
			s.DeadMask[n] = true
		}
	}
	for p, marked := range s.DeadMask {
		if marked {
			s.Dead[cells[p]]++
			s.Captures[cells[p].Other()]++
			cells[p] = Empty
		}
	}
	groups, at, areas := regions(cells, b.Size)
	sekiGroups := make([]bool, len(groups))
	if b.Rule == Japanese {
		sekiGroups = proposedSeki(cells, b.Size, groups, at, areas)
	}
	explicit := make(map[int]bool)
	for p, value := range seki {
		if p < 0 || p >= len(cells) || cells[p] == Empty {
			return s, errors.New("invalid seki selection")
		}
		id := at[p]
		if old, ok := explicit[id]; ok && old != value {
			return s, errors.New("conflicting seki selections")
		}
		explicit[id] = value
		sekiGroups[id] = value
	}
	for p, c := range cells {
		if c != Empty {
			s.Stones[c]++
			s.Owner[p] = c
			s.SekiMask[p] = sekiGroups[at[p]]
		}
	}
	if b.Rule != Jungo {
		for _, r := range areas {
			if r.color == Empty {
				continue
			}
			excluded := false
			if b.Rule == Japanese {
				for id := range r.boundary {
					if sekiGroups[id] {
						excluded = true
						break
					}
				}
			}
			if !excluded {
				s.Territory[r.color] += len(r.points)
				for _, p := range r.points {
					s.Owner[p] = r.color
				}
			}
		}
	}
	switch b.Rule {
	case Japanese:
		s.Black = float64(s.Territory[Black] + s.Captures[Black])
		s.White = float64(s.Territory[White]+s.Captures[White]) + s.Komi
	case Chinese:
		s.Black = float64(s.Stones[Black] + s.Territory[Black])
		s.White = float64(s.Stones[White]+s.Territory[White]) + s.Komi
	case Jungo:
		s.Black = float64(s.Stones[Black])
		s.White = float64(s.Stones[White])
	default:
		return s, errors.New("unknown scoring rule")
	}
	if !s.NoResult {
		s.Margin = math.Abs(s.Black - s.White)
		if s.Black > s.White {
			s.Winner = Black
		} else if s.White > s.Black {
			s.Winner = White
		}
	}
	return s, nil
}

// SuggestDead は黒視点の所有領域評価から死石候補を返す。最終判断はプレイヤーが行う。
func SuggestDead(b *Board, ownership []float64) map[int]bool {
	marked := make(map[int]bool)
	if b.Rule == Jungo || len(ownership) != len(b.Cells) {
		return marked
	}
	seen := make([]bool, len(b.Cells))
	for p, c := range b.Cells {
		if c == Empty || seen[p] {
			continue
		}
		stones, _ := b.Group(p)
		certain := true
		for _, n := range stones {
			seen[n] = true
			v := ownership[n]
			if c == White {
				v = -v
			}
			if math.IsNaN(v) || math.IsInf(v, 0) || v < -1 || v > -.85 {
				certain = false
			}
		}
		if certain {
			marked[p] = true
		}
	}
	return marked
}
