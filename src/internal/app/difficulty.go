// 作成日: 2026-10-18。対局中は固定した強さで着手を選び、完了した対局だけを次局の調整へ使う。
package app

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"

	"github.com/Tremendous1192/NobiGo/engine"
)

// DifficultyName は保存値に対応する承認済み名称を返す。
func DifficultyName(level int) string {
	names := [...]string{"のびのび", "いい勝負", "腕試し", "強敵", "大挑戦"}
	if level < 0 || level >= len(names) {
		return "未設定"
	}
	return names[level]
}

// RuleIndex はプロフィール内の日本・中国・純碁の順番を返す。
func RuleIndex(rule Rule) int {
	switch rule {
	case Chinese:
		return 1
	case Jungo:
		return 2
	default:
		return 0
	}
}

// RankName はアプリ内推定棋力を表示する。公式段級位との対応は保証しない。
func RankName(rank float64) string {
	n := int(math.Round(math.Max(0, math.Min(38, rank))))
	if n < 30 {
		return fmt.Sprintf("%d級", 30-n)
	}
	return fmt.Sprintf("%d段", n-29)
}

// Strength は開始時のCPU強度を返す。対局途中のプロフィール変更は反映しない。
func Strength(p Profile, s Settings) float64 {
	offsets := [...]float64{-1, 0, 2, 4, 6}
	v := p.Ranks[RuleIndex(s.Rule)] + offsets[s.Difficulty]
	for _, a := range p.Adjustments {
		if a.Rule == s.Rule && a.Size == s.Size && a.Difficulty == s.Difficulty {
			v += a.Offset
			break
		}
	}
	return math.Max(-2, math.Min(44, v))
}

// EnginePosition は棋譜を独立した解析要求へ変換する。
func EnginePosition(b *Board) engine.Position {
	p := engine.Position{Size: b.Size, Rules: string(b.Rule), Komi: Komi(b.Rule), Moves: make([]engine.Ply, len(b.Moves))}
	if b.Rule == Jungo {
		p.Rules = "stone-scoring"
	}
	for i, m := range b.Moves {
		color := "B"
		if m.Color == White {
			color = "W"
		}
		p.Moves[i] = engine.Ply{Color: color, Point: m.Point}
	}
	return p
}

// trueEye は隣接石と斜めの敵石から、自分の単眼を保守的に判定する。
func trueEye(cells []Color, size, point int, color Color) bool {
	if cells[point] != Empty {
		return false
	}
	for _, n := range Neighbors(point, size) {
		if cells[n] != color {
			return false
		}
	}
	x, y := point%size, point/size
	bad, edge := 0, false
	for _, dx := range []int{-1, 1} {
		for _, dy := range []int{-1, 1} {
			nx, ny := x+dx, y+dy
			if nx < 0 || nx >= size || ny < 0 || ny >= size {
				edge = true
				continue
			}
			if cells[ny*size+nx] == color.Other() {
				bad++
			}
		}
	}
	return bad == 0 || !edge && bad == 1
}

// safeFill は活きが確認できた自分の領域だけを、二眼を維持して埋める。
func safeFill(b *Board, point int) bool {
	next, captured, err := b.trial(point)
	if err != nil {
		return false
	}
	if captured > 0 {
		return true
	}
	for _, n := range Neighbors(point, b.Size) {
		if b.Cells[n] != b.ToMove {
			return false
		}
	}
	groups, at, areas := regions(next, b.Size)
	alive := passAlive(next, b.Size, groups, at, areas)
	return alive[at[point]]
}

// pureFinish はパス前に取れる石と安全な自陣の埋め残しを探す。死石を自動除去しない。
func pureFinish(b *Board, a engine.Analysis) int {
	best, value := -1, -1.0
	for p, c := range b.Cells {
		if c != Empty || !safeFill(b, p) {
			continue
		}
		v := 0.0
		if len(a.Policy) > p {
			v = math.Max(0, a.Policy[p])
		}
		_, captured, _ := b.trial(p)
		v += float64(captured)
		if v > value {
			best, value = p, v
		}
	}
	return best
}

// SelectMove は探索評価と方策を強度に応じて混ぜる。rngは対局所有者専用であり、入力を変更しない。
func SelectMove(b *Board, a engine.Analysis, strength float64, rng *rand.Rand) (int, error) {
	if rng == nil || !finiteRange(strength, -2, 44) || len(a.Moves) == 0 || !b.Legal(-1) {
		return 0, errors.New("cannot select move")
	}
	sign := 1.0
	if b.ToMove == White {
		sign = -1
	}
	best := -math.MaxFloat64
	valid := make([]engine.Candidate, 0, len(a.Moves))
	for _, m := range a.Moves {
		if b.Legal(m.Point) {
			valid = append(valid, m)
			best = math.Max(best, sign*m.ScoreLead)
		}
	}
	if len(valid) == 0 {
		return 0, errors.New("engine has no legal candidate")
	}
	if valid[0].Point == -1 {
		if b.Rule == Jungo {
			return pureFinish(b, a), nil
		}
		return -1, nil
	}
	quality := math.Max(.03, math.Min(1, (strength+3)/42))
	// 低visitでは候補数が少ないため、探索候補だけの抽選では初心者向けの幅が出ない。
	if len(a.Policy) == len(b.Cells)+1 && rng.Float64() > quality {
		weights := make([]float64, len(b.Cells))
		total := 0.0
		for p, c := range b.Cells {
			if c != Empty || !b.Legal(p) || trueEye(b.Cells, b.Size, p, b.ToMove) {
				continue
			}
			if len(a.Ownership) == len(b.Cells) && sign*a.Ownership[p] < -.95 {
				continue
			}
			v := math.Pow(math.Max(0, a.Policy[p])+.0001, .3+.7*quality)
			weights[p] = v
			total += v
		}
		if total > 0 {
			x := rng.Float64() * total
			for p, v := range weights {
				x -= v
				if v > 0 && x <= 0 {
					return p, nil
				}
			}
		}
	}
	temperature := .05 + 8*(1-quality)
	weights := make([]float64, len(valid))
	total := 0.0
	for i, m := range valid {
		weights[i] = math.Exp(-math.Max(0, best-sign*m.ScoreLead)/temperature) * math.Sqrt(float64(m.Visits)+1)
		total += weights[i]
	}
	x := rng.Float64() * total
	selected := valid[len(valid)-1].Point
	for i, v := range weights {
		x -= v
		if x <= 0 {
			selected = valid[i].Point
			break
		}
	}
	if selected == -1 && b.Rule == Jungo {
		selected = pureFinish(b, a)
	}
	return selected, nil
}

// CompletedProfile は結果IDの重複登録を防ぎ、保存前の独立コピーへ履歴・推定棋力を反映する。
func CompletedProfile(p Profile, g GameResult) (Profile, error) {
	for _, old := range p.Results {
		if old.ID == g.ID {
			return p, nil
		}
	}
	p.Results = append(slices.Clone(p.Results), g)
	// 長期利用でもJSONを有限に保つ。通算カウンターは要約履歴の保持数に制限されない。
	if len(p.Results) > MaxResults {
		p.Results = slices.Clone(p.Results[len(p.Results)-MaxResults:])
	}
	p.Adjustments = slices.Clone(p.Adjustments)
	p.GamesPlayed++
	if !g.NoResult {
		outcome := 0.0
		if g.BlackScore == g.WhiteScore {
			outcome = .5
			p.Draws++
		} else if (g.BlackScore > g.WhiteScore) == (g.Player == Black) {
			outcome = 1
			p.Wins++
		}
		targets := [...]float64{.85, .65, .45, .25, .05}
		if g.Settings.Validate() != nil {
			return Profile{}, errors.New("invalid result settings")
		}
		delta := .4 * (outcome - targets[g.Settings.Difficulty])
		for i := range p.Ranks {
			scale := .25
			if i == RuleIndex(g.Settings.Rule) {
				scale = 1
			}
			p.Ranks[i] = math.Max(0, math.Min(38, p.Ranks[i]+delta*scale))
		}
		n, sum := 0, 0.0
		for i := len(p.Results) - 1; i >= 0 && n < 10; i-- {
			r := p.Results[i]
			if r.NoResult || r.Settings.Rule != g.Settings.Rule || r.Settings.Size != g.Settings.Size || r.Settings.Difficulty != g.Settings.Difficulty {
				continue
			}
			n++
			if r.BlackScore == r.WhiteScore {
				sum += .5
			} else if (r.BlackScore > r.WhiteScore) == (r.Player == Black) {
				sum++
			}
		}
		if n == 10 {
			index := -1
			for i, a := range p.Adjustments {
				if a.Rule == g.Settings.Rule && a.Size == g.Settings.Size && a.Difficulty == g.Settings.Difficulty {
					index = i
					break
				}
			}
			if index < 0 {
				p.Adjustments = append(p.Adjustments, Adjustment{Rule: g.Settings.Rule, Size: g.Settings.Size, Difficulty: g.Settings.Difficulty})
				index = len(p.Adjustments) - 1
			}
			a := &p.Adjustments[index]
			a.Offset = math.Max(-1, math.Min(1, a.Offset+.5*(sum/10-targets[g.Settings.Difficulty])))
		}
	}
	return p, p.Validate()
}
