// 作成日: 2026-10-18。PlayGoの引数と起動・終了を管理する。

// PlayGo はローカルのCPU囲碁ゲームを起動し、ウィンドウ終了時に資源を解放する。
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tremendous1192/PlayGo/internal/gui"
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
	preview := flag.String("preview", "", "render a synthetic startup/game/game-wait/game-pass/scoring/result/choices/review/review-player/review-19/review-detail/review-detail-cpu/review-detail-combined/review-detail-wait/review-detail-error/review-detail-long/review-detail-bottom/review-detail-min fixture; requires screenshot")
	verify := flag.String("verify", "", "run CPU-to-render acceptance checks and write JSON; uses temporary profiles")
	flag.Parse()
	if err = gui.Run(gui.Options{Root: *root, Screenshot: *screenshot, Preview: *preview, Verify: *verify}); err != nil {
		fmt.Fprintln(os.Stderr, "run game:", err)
		return 1
	}
	return 0
}
