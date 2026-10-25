// 作成日: 2026-10-24。概要の説明が具体的な盤面変化・話者・比較の限界に対応することを検証する。
package gui

import (
	"strings"
	"testing"

	"github.com/Tremendous1192/NobiGo/internal/app"
)

// summaryTextFixture は説明対象を固定した合法棋譜を用意する。AI選定そのものはapp側で別に検証する。
func summaryTextFixture(t *testing.T, points []int, alternative int) *app.Controller {
	t.Helper()
	c, _, _ := adviceFixture(t, app.Japanese, points)
	last := c.Board.Moves[len(points)-1]
	m := &app.SummaryMove{Turn: len(points), Point: last.Point, Color: last.Color, Evaluated: true, HasAlternative: true, Alternative: alternative}
	c.Player = last.Color
	c.SummarySession = &app.SummarySession{Ready: true, Data: app.GameSummary{Highlight: m, GoodMove: m, Improvement: m}}
	return c
}

// TestSummaryExplainsRescueAndJoinedGroups は助けた対象・空きの増加・実際の組数・次の確認を説明することを検証する。
func TestSummaryExplainsRescueAndJoinedGroups(t *testing.T) {
	c := summaryTextFixture(t, []int{0, 1, 14, 48, 7}, 40)
	_, body, _ := summaryPageContent(c, 2)
	for _, want := range []string{"あなた（黒）", "左上のA6", "2組の黒石", "左上のA7", "黒石1個", "1か所から3か所", "良かったところ", "次局も", "その後も取られない保証ではありません"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing rescue evidence %q: %s", want, body)
		}
	}
	if strings.Contains(body, "推定損失") || strings.Contains(body, "連結1組") {
		t.Fatal("statistic substituted for explanation")
	}
}

// TestSummaryExplainsAlternativeForSameStone は実手で危険なままの石を候補が助ける一手の違いを説明することを検証する。
func TestSummaryExplainsAlternativeForSameStone(t *testing.T) {
	c := summaryTextFixture(t, []int{0, 1, 14, 48, 40}, 7)
	scene, err := makeSummaryScene(c, c.SummarySession.Data.Improvement)
	if err != nil {
		t.Fatal(err)
	}
	body := summaryComparisonText(scene)
	for _, want := range []string{"A7", "黒石1個", "実際の手で1か所", "候補で3か所", "空きを増やす手を比べ"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing comparison %q: %s", want, body)
		}
	}
	if strings.Contains(body, "必ず") || strings.Contains(body, "安全です") {
		t.Fatal("local comparison promised survival")
	}
}

// TestSummaryExplainsCombinedDanger は取り石と自分の危険を併記し、コウで直ちに取り返せないことを区別する。
func TestSummaryExplainsCombinedDanger(t *testing.T) {
	c := summaryTextFixture(t, []int{2, 1, 8, 7, 0}, 48)
	_, body, _ := summaryPageContent(c, 0)
	for _, want := range []string{"CPU（白）の石を1個取り", "黒石1個も", "1か所だけ", "今そこへ打てない", "この手だけが勝因・敗因とは限りません"} {
		if !strings.Contains(body, want) {
			t.Fatalf("combined effect missing %q: %s", want, body)
		}
	}
}

// TestSummaryDoesNotInventCandidateReason は局所差でAIの理由を説明できない場合に限界を伝えることを検証する。
func TestSummaryDoesNotInventCandidateReason(t *testing.T) {
	c := summaryTextFixture(t, []int{16, 0, 24}, 40)
	scene, err := makeSummaryScene(c, c.SummarySession.Data.Improvement)
	if err != nil {
		t.Fatal(err)
	}
	body := summaryComparisonText(scene)
	if !strings.Contains(body, "AIが候補を選んだ理由を説明できません") || strings.Contains(body, "候補なら勝") {
		t.Fatal(body)
	}
	c.SummarySession.Data.Improvement.Alternative = 16
	scene, err = makeSummaryScene(c, c.SummarySession.Data.Improvement)
	if err != nil || scene.alternative != nil {
		t.Fatal("illegal alternative was accepted", err)
	}
	c.SummarySession.Data.Improvement.Alternative = -1
	scene, err = makeSummaryScene(c, c.SummarySession.Data.Improvement)
	if err != nil {
		t.Fatal(err)
	}
	body = summaryComparisonText(scene)
	if !strings.Contains(body, "次局はパスの前に") || strings.Contains(body, "青い印") {
		t.Fatal("pass candidate refers to a board marker", body)
	}
}

// TestSummaryStageSeparatesActionsFromIntent は行動の頻度・同率・空区間を区別し、一度の救出に偏らないことを検証する。
func TestSummaryStageSeparatesActionsFromIntent(t *testing.T) {
	c := summaryTextFixture(t, []int{16, 0, 24}, 40)
	stage := app.SummaryStage{From: 1, To: 10, CPUMoves: 5, CaptureMoves: 3, RescueMoves: 1, Saved: 20}
	body := summaryStageText(c, stage, "序盤")
	if !strings.Contains(body, "あなたの石を取った手が3回") || !strings.Contains(body, "空きを増やした手が1回") || strings.Contains(body, "自分の石を守る狙い") {
		t.Fatal(body)
	}
	stage.RescueMoves = 3
	if body = summaryStageText(c, stage, "中盤"); !strings.Contains(body, "一つの狙いには絞れません") {
		t.Fatal(body)
	}
	if body = summaryStageText(c, app.SummaryStage{}, "終盤"); !strings.Contains(body, "CPUの着手はありません") {
		t.Fatal(body)
	}
}

// TestSummaryRulePerspectiveAndMissingEvidence は白視点・純碁の確定石数・欠損を混同しないことを検証する。
func TestSummaryRulePerspectiveAndMissingEvidence(t *testing.T) {
	c := summaryTextFixture(t, []int{0, 1}, -1)
	c.Board.Rule = app.Jungo
	_, body, _ := summaryPageContent(c, 0)
	for _, want := range []string{"あなたは白、CPUは黒", "あなた（白）", "最後に残った石の数", "取った石を別に加点しません"} {
		if !strings.Contains(body, want) {
			t.Fatal(body)
		}
	}
	c.SummarySession.Data = app.GameSummary{}
	for _, page := range []int{0, 2, 3} {
		_, body, move := summaryPageContent(c, page)
		missing := "揃っていません"
		if page == 3 {
			missing = "今回は紹介できる場面が見つかりませんでした"
		}
		if move != nil || !strings.Contains(body, missing) || strings.Contains(body, "損失は0") {
			t.Fatal("missing evidence was judged", body)
		}
	}
}
