// 作成日: 2026-10-18。NobiGoの引数と起動・終了を管理する。

// NobiGo はローカルのCPU囲碁ゲームを起動し、ウィンドウ終了時に資源を解放する。
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tremendous1192/NobiGo/internal/gui"
)

// main は終了コードへ変換する。
func main() { os.Exit(run()) }

// run は設定を読み、GUIの終了後にエラーを報告する。
func run() int {
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve executable:", err)
		return 1
	}
	root := flag.String("root", filepath.Dir(executable), "directory containing engine and profile")
	screenshot := flag.String("screenshot", "", "save application screenshot and exit")
	preview := flag.String("preview", "", "render a synthetic fixture; requires screenshot; modes: startup/startup-check/startup-download/startup-verify/startup-load/startup-error/startup-repair/startup-cancel/game/game-header-7/game-header-9/game-header-11/game-header-13/game-header-15/game-header-19/game-header-chinese/game-header-jungo/game-header-long/game-header-min/game-wait/game-pass/scoring/result/choices/review/review-player/review-19/review-detail/review-detail-cpu/review-detail-combined/review-detail-wait/review-detail-error/review-detail-long/review-detail-bottom/review-detail-min/game-white-jungo/game-extremes/game-gap/review-detail-before/review-detail-outside/review-detail-pass/review-detail-empty/review-detail-7/review-detail-9/review-detail-11/review-detail-13/review-detail-15/review-detail-19/review-focus-moved/review-focus-before/review-ai/review-ai-19/review-detail-ai/review-detail-ai-risk/review-detail-rescue/review-detail-atari/review-detail-jungo-empty/review-detail-jungo-pass/summary-1/summary-2/summary-3/summary-4/summary-wait/summary-error/summary-missing/summary-white/summary-jungo/summary-draw/summary-empty/summary-noresult/summary-long/summary-bottom/summary-min/summary-7/summary-9/summary-11/summary-13/summary-15/summary-19/summary-rescue/summary-compare/summary-compare-bottom/summary-combined/summary-cpu-actions/summary-jungo-rescue/summary-white-rescue/summary-chinese/summary-pass-candidate")
	verify := flag.String("verify", "", "run CPU-to-render acceptance checks and write JSON; uses temporary profiles")
	prepare := flag.String("prepare", "", "prepare model and engine, write startup timing JSON and exit; does not access profiles")
	flag.Parse()
	if err = gui.Run(gui.Options{Root: *root, Screenshot: *screenshot, Preview: *preview, Verify: *verify, Prepare: *prepare}); err != nil {
		fmt.Fprintln(os.Stderr, "run game:", err)
		return 1
	}
	return 0
}
