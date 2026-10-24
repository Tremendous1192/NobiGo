// 作成日: 2026-10-18。日本語GUIの起動と描画を担当する。GUI操作はメインスレッドに限定する。

// Package gui はNobiGoの日本語画面を描画する。
package gui

import (
	"bytes"
	"context"
	_ "embed" // 日本語フォントを同梱してインストール済みフォントへの依存をなくす。
	"errors"
	"image"
	"image/png"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"

	"github.com/Tremendous1192/NobiGo/engine"
	"github.com/Tremendous1192/NobiGo/internal/app"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/exp/textinput"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

//go:embed assets/NotoSansJP.ttf
var fontData []byte

type window struct {
	face                *text.GoTextFace
	frames              int
	screenshot          string
	captureErr          error
	store               *app.Store
	profiles            []app.Profile
	broken              []string
	selected            int
	settings            app.Settings
	engine              *engine.Managed
	controller          *app.Controller
	ready               chan preparationResult
	preparationUpdates  chan preparationStage
	preparationStage    preparationStage
	modelRepair         bool
	preparationReport   string
	engineDone          chan struct{}
	engineCancel        context.CancelFunc
	engineLoading       bool
	engineError         error
	actions             []hit
	menu                *choiceMenu
	dialog              *dialog
	field               textinput.Field
	notice              string
	tab                 int
	shownPhase          app.Phase
	dead                map[int]bool
	seki                map[int]bool
	scorePreview        app.Score
	scoreError          string
	markSeki            bool
	animationStart      time.Time
	resultPromptPending bool
	reviewTurn          int
	reviewBoard         *app.Board
	reviewBefore        *app.Board
	reviewFocus         *reviewDiagram
	advice              *adviceDialog
	summary             *summaryDialog
	adviceCanvas        *ebiten.Image
	autoplay            bool
	nextReplay          time.Time
	previewMode         bool
	previewMinimum      bool
	verification        *verification
	engineBegan         time.Time
	inputCanvas         *ebiten.Image
	inputScroll         float64
}

type Options struct{ Root, Screenshot, Preview, Verify, Prepare string }

// Run はGUIが閉じるまで待機する。screenshot指定時は検証画像を保存して終了する。
func Run(options Options) (result error) {
	root, screenshot := options.Root, options.Screenshot
	source, err := text.NewGoTextFaceSource(bytes.NewReader(fontData))
	if err != nil {
		return err
	}
	g := &window{face: &text.GoTextFace{Source: source, Size: 24}, screenshot: screenshot, selected: -1, settings: app.DefaultSettings(), engine: &engine.Managed{Root: root}}
	g.preparationReport = options.Prepare
	if options.Prepare != "" && (options.Verify != "" || options.Preview != "" || screenshot != "") {
		return errors.New("preparation check cannot be combined with other inspection modes")
	}
	g.face.SetVariation(text.Tag(0x77676874), 400)
	profileDir := filepath.Join(root, "profile")
	if options.Verify != "" {
		if options.Preview != "" || screenshot != "" {
			return errors.New("verification cannot be combined with preview or screenshot")
		}
		profileDir, err = os.MkdirTemp(filepath.Dir(options.Verify), ".verification-profile-*")
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, os.RemoveAll(profileDir)) }()
		g.verification = newVerification(options.Verify)
	}
	if options.Preview != "" {
		if screenshot == "" {
			return errors.New("preview requires screenshot")
		}
		g.previewMode = true
		if err = g.preparePreview(options.Preview); err != nil {
			return err
		}
	} else if options.Prepare != "" {
		g.prepareEngine()
	} else {
		g.store, err = app.OpenStore(profileDir)
		if err != nil {
			g.notice = "保存フォルダーを開けません。書き込み可能な場所へZIPを展開してください。"
		} else if err = g.reload(""); err != nil {
			g.notice = "プレイヤー情報を読めません。保存フォルダーを確認してください。"
		}
		g.prepareEngine()
	}
	defer func() {
		g.field.Blur()
		if g.engineCancel != nil {
			g.engineCancel()
		}
		if g.engineDone != nil {
			<-g.engineDone
		}
		if g.controller != nil && !g.previewMode {
			g.controller.Close()
		}
		result = errors.Join(result, g.engine.Close())
	}()
	ebiten.SetWindowTitle("NobiGo(v0.7.1)")
	ebiten.SetWindowSize(1180, 800)
	if g.previewMinimum {
		ebiten.SetWindowSize(885, 600)
	}
	ebiten.SetWindowSizeLimits(885, 600, -1, -1)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	err = ebiten.RunGame(g)
	return errors.Join(err, g.captureErr)
}

// prepareEngine は初回取得とモデル読込みを順番に行い、成功後に対局を許可する。
func (g *window) prepareEngine() {
	g.startPreparation(engine.PrepareModel, g.engine.Prepare)
}

// Update は操作をUIスレッドで適用し、作業から所有権を受け取る。
func (g *window) Update() error {
	if g.screenshot != "" && g.frames >= 2 {
		return ebiten.Termination
	}
	if g.previewMode {
		return nil
	}
	if g.engineLoading {
		select {
		case result := <-g.ready:
			g.engineLoading = false
			g.engineError = result.err
			if g.preparationReport != "" {
				return g.finishPreparationReport(result)
			}
			if g.verification != nil {
				g.verification.StartupMS = time.Since(g.engineBegan).Milliseconds()
				g.verification.ModelPreparationMS = result.modelMS
				g.verification.EngineLoadMS = result.engineMS
			}
			if result.err == nil && g.store != nil {
				var err error
				g.controller, err = app.NewController(context.Background(), g.engine, g.store, rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())))
				g.engineError = err
			}
		default:
			select {
			case stage := <-g.preparationUpdates:
				g.preparationStage = stage
			default:
			}
		}
	}
	if g.controller != nil {
		g.controller.Poll()
		g.syncPhase()
		g.advanceResult(time.Now())
	}
	if g.verification != nil {
		return g.updateVerification()
	}
	if g.dialog != nil && g.dialog.input {
		if g.updateNameInput() {
			return nil
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.dismiss()
		return nil
	}
	if g.advice != nil {
		g.updateAdviceInput()
	}
	if g.summary != nil {
		g.updateSummaryInput(g.summary)
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		for i := len(g.actions) - 1; i >= 0; i-- {
			h := g.actions[i]
			if image.Pt(x, y).In(h.rect) {
				h.action()
				return nil
			}
		}
	}
	// 詳細を開くクリックより先に再生を進めると、読もうとした手と助言がずれる。
	if g.controller != nil {
		g.advanceReview(time.Now())
	}
	return nil
}

// Draw は画面と最前面の操作領域を再構成する。
func (g *window) Draw(screen *ebiten.Image) {
	screen.Fill(tint(background))
	g.actions = g.actions[:0]
	g.syncPhase()
	if g.controller == nil || g.controller.Phase == app.Idle {
		g.drawStartup(screen)
	} else {
		g.drawActive(screen)
	}
	if g.menu != nil {
		g.drawMenu(screen)
	}
	if g.dialog != nil {
		g.drawDialog(screen)
	}
	if g.advice != nil {
		g.drawAdviceDialog(screen)
	}
	if g.summary != nil {
		g.drawSummaryDialog(screen, g.summary)
	}
	if g.previewMode {
		g.label(screen, "画面確認用の合成例・架空の評価値", 830, 7, 12, warning)
	}
	if g.controller != nil && !g.previewMode {
		g.controller.RecordDisplayed()
		if g.verification != nil && g.controller.Board != nil {
			g.recordVerificationDraw(time.Now())
		}
	}
	g.frames++
	if g.screenshot != "" && g.frames == 2 {
		capture := screen
		if g.previewMinimum {
			// 固定の論理座標の画像だけでは最小ウィンドウの文字サイズを確認できない。
			capture = ebiten.NewImage(885, 600)
			op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
			op.GeoM.Scale(.75, .75)
			capture.DrawImage(screen, op)
		}
		f, err := os.Create(g.screenshot)
		if err == nil {
			err = errors.Join(png.Encode(f, capture), f.Close())
		}
		g.captureErr = err
	}
}

// syncPhase は画面へ入るときだけ初期表示を設定し、同じ画面内のタブ選択を保持する。
func (g *window) syncPhase() {
	if g.controller == nil || g.shownPhase == g.controller.Phase {
		return
	}
	phase := g.controller.Phase
	g.advice = nil
	g.reviewFocus = nil
	if phase != app.Summary {
		g.summary = nil
	}
	switch phase {
	case app.Playing:
		if g.shownPhase != app.Scoring {
			g.tab = 1
		}
		g.autoplay = false
		g.reviewBoard = nil
		g.reviewBefore = nil
		g.resultPromptPending = false
	case app.Review:
		g.tab = 2
	case app.Summary:
		g.autoplay = false
	case app.Scoring:
		g.prepareScore()
	case app.Finished:
		if g.shownPhase != app.Review && g.shownPhase != app.Summary {
			g.animationStart = time.Now()
			g.resultPromptPending = true
		}
		g.notice = ""
	}
	g.shownPhase = phase
}

// Layout は描画座標を固定してウィンドウの拡大縮小に追従する。
func (g *window) Layout(width, height int) (int, int) { return 1180, 800 }
