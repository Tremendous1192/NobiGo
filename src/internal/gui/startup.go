// 作成日: 2026-10-18。プレイヤー管理とルール別設定を表示する。保存成功後だけ選択値を更新する。
package gui

import (
	"context"
	"errors"
	"image"
	"math"

	"github.com/Tremendous1192/NobiGo/engine"
	"github.com/Tremendous1192/NobiGo/internal/app"
	"github.com/hajimehoshi/ebiten/v2"
)

// reload はIDで選択を引き継ぎ、改名してもプロフィールの対応を保つ。
func (g *window) reload(id string) error {
	profiles, broken, err := g.store.List()
	if err != nil {
		return err
	}
	g.profiles = profiles
	g.broken = broken
	g.selected = -1
	for i, p := range profiles {
		if p.ID == id {
			g.selected = i
			g.settings = p.Last
			break
		}
	}
	return nil
}

// playerDialog は新規登録または選択プレイヤーの改名を開始する。
func (g *window) playerDialog(rename bool) {
	title, name := "プレイヤーを登録", ""
	if rename {
		title = "プレイヤー名を変更"
		name = g.profiles[g.selected].Name
	}
	g.resetNameInput(name)
	g.dialog = &dialog{title: title, body: "名前と戦績は、このPCの保存フォルダーに保存します。", input: true, submit: func(name string) error {
		if rename {
			p := g.profiles[g.selected]
			p.Name = name
			if err := g.store.Save(&p); err != nil {
				return err
			}
			return g.reload(p.ID)
		}
		p, err := g.store.Create(name)
		if err != nil {
			return err
		}
		return g.reload(p.ID)
	}}
}

// saveRank は手動棋力修正を保存し、失敗時に画面上の値を維持する。
func (g *window) saveRank(rule, index int) {
	p := g.profiles[g.selected]
	p.Ranks[rule] = float64(index)
	if err := g.store.Save(&p); err != nil {
		g.notice = "棋力を保存できません。他のNobiGoを閉じて再度お試しください。"
		return
	}
	g.profiles[g.selected] = p
	g.notice = "棋力を更新しました。表示段級位はアプリ内の目安です。"
}

// begin は保存済みプロフィールと選択中の条件で対局を開始する。
func (g *window) begin() {
	if g.controller == nil || g.selected < 0 {
		return
	}
	if err := g.controller.Start(g.profiles[g.selected], g.settings); err != nil {
		g.notice = "開始できません。他のNobiGoを閉じ、保存先の書き込み権限を確認してください。"
		return
	}
	g.profiles[g.selected] = g.controller.Profile
	g.notice = ""
}

// drawStartup は要件の順番で設定を並べる。
func (g *window) drawStartup(screen *ebiten.Image) {
	g.label(screen, "NobiGo", 50, 28, 44, foreground)
	g.label(screen, "囲碁を、楽しく。あなたのペースで一局ずつ。", 52, 91, 21, muted)
	status := "CPUエンジン 準備完了"
	c := accent
	if g.engineLoading {
		status = []string{"モデルを確認中…", "モデルを取得中…", "モデルを検証中…", "CPUエンジン 読込み中…"}[g.preparationStage]
		c = warning
	}
	if g.engineError != nil {
		status = "準備に失敗しました"
		if errors.Is(g.engineError, context.Canceled) {
			status = "準備を中止しました"
		}
		if errors.Is(g.engineError, engine.ErrModelInvalid) {
			status = "モデルの再取得が必要です"
		}
		c = warning
	}
	g.label(screen, status, 790, 58, 20, c)
	if g.engineLoading {
		g.button(screen, image.Rect(900, 100, 1125, 140), "準備を中止", true, g.cancelPreparation)
	}
	if g.engineError != nil {
		title := "準備を再試行"
		if errors.Is(g.engineError, engine.ErrModelInvalid) {
			title = "モデルを再取得"
		}
		g.button(screen, image.Rect(900, 100, 1125, 140), title, true, func() {
			g.modelRepair = g.modelRepair || errors.Is(g.engineError, engine.ErrModelInvalid)
			g.prepareEngine()
		})
	}
	rect(screen, image.Rect(35, 150, 1145, 725), panel)
	g.label(screen, "プレイヤー", 56, 168, 18, muted)
	names := make([]string, len(g.profiles))
	for i, p := range g.profiles {
		names[i] = p.Name
	}
	g.combo(screen, image.Rect(56, 201, 410, 246), "プレイヤー", names, g.selected, len(names) > 0, func(i int) { g.selected = i; g.settings = g.profiles[i].Last; g.notice = "" })
	g.button(screen, image.Rect(424, 201, 554, 246), "新規登録", g.store != nil, func() { g.playerDialog(false) })
	g.button(screen, image.Rect(568, 201, 698, 246), "名前を変更", g.selected >= 0, func() { g.playerDialog(true) })
	g.button(screen, image.Rect(972, 201, 1102, 246), "削除", g.selected >= 0, func() {
		p := g.profiles[g.selected]
		g.dialog = &dialog{title: "プレイヤーを削除", body: "選択したプレイヤーと戦績を一覧から削除します。\n削除前のファイルは復旧用に退避します。", confirm: func() error {
			if err := g.store.Delete(p.ID); err != nil {
				return err
			}
			return g.reload("")
		}}
	})
	rankNames := make([]string, 39)
	for i := range rankNames {
		rankNames[i] = app.RankName(float64(i))
	}
	for i, title := range []string{"日本ルールの棋力", "中国ルールの棋力", "純碁の棋力"} {
		x := 56 + i*356
		g.label(screen, title, float64(x), 269, 17, muted)
		selected := 0
		if g.selected >= 0 {
			selected = int(math.Round(g.profiles[g.selected].Ranks[i]))
		}
		g.combo(screen, image.Rect(x, 301, x+330, 344), title, rankNames, selected, g.selected >= 0, func(index int) { g.saveRank(i, index) })
	}
	rules := []app.Rule{app.Japanese, app.Chinese, app.Jungo}
	ruleIndex := app.RuleIndex(g.settings.Rule)
	g.label(screen, "ルール", 56, 367, 18, muted)
	g.combo(screen, image.Rect(252, 357, 728, 402), "ルール", []string{"日本棋院（日本ルール）", "中国ルール", "純碁（盤上の石数）"}, ruleIndex, true, func(i int) { g.settings.Rule = rules[i] })
	komi := []string{"コミ 6.5目", "コミ 7.5目", "コミ 0・途中評価は近似"}[ruleIndex]
	g.label(screen, komi, 760, 367, 18, muted)
	sizes := []int{9, 13, 19, 7, 11, 15}
	sizeIndex := 0
	for i, n := range sizes {
		if n == g.settings.Size {
			sizeIndex = i
		}
	}
	g.label(screen, "盤の大きさ", 56, 429, 18, muted)
	g.combo(screen, image.Rect(252, 419, 728, 464), "盤の大きさ", []string{"9路（おすすめ）", "13路", "19路", "7路", "11路", "15路"}, sizeIndex, true, func(i int) { g.settings.Size = sizes[i] })
	g.label(screen, "難易度", 56, 491, 18, muted)
	difficulties := make([]string, 5)
	for i := range difficulties {
		difficulties[i] = app.DifficultyName(i)
	}
	g.combo(screen, image.Rect(252, 481, 728, 526), "難易度", difficulties, g.settings.Difficulty, true, func(i int) { g.settings.Difficulty = i })
	g.label(screen, "強さは対局後に調整します", 760, 491, 18, muted)
	g.label(screen, "先攻・後攻", 56, 553, 18, muted)
	g.combo(screen, image.Rect(252, 543, 728, 588), "先攻・後攻", []string{"先攻（黒）", "後攻（白）", "ランダム"}, g.settings.Side, true, func(i int) { g.settings.Side = i })
	g.button(screen, image.Rect(760, 640, 1102, 698), "対局を始める →", g.selected >= 0 && g.controller != nil && !g.engineLoading && g.engineError == nil, g.begin)
	g.label(screen, "専用GPU不要 · 初回モデル取得後はオフライン", 56, 655, 17, accent)
	if len(g.broken) > 0 {
		g.button(screen, image.Rect(56, 604, 650, 641), "読めないプロフィールがあります：バックアップから復旧", true, func() {
			id := g.broken[0]
			g.dialog = &dialog{title: "バックアップから復旧", body: "破損した内容を退避し、直前の保存内容を復元します。", confirm: func() error {
				if err := g.store.Restore(id); err != nil {
					return err
				}
				return g.reload(id)
			}}
		})
	}
	message := g.notice
	if message == "" && g.engineError != nil {
		message = "接続と保存先を確認し、他のNobiGoを閉じて再試行してください。初回取得にはネット接続が必要です。"
	}
	if message == "" {
		message = "プレイヤーを登録すると、戦績と前回の設定を保存できます。"
	}
	g.label(screen, g.fit(message, 1080, 17), 50, 750, 17, warning)
}
