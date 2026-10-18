# PlayGo v0.2.0

作成日: 2026-10-18。初心者向けのWindows囲碁ゲーム。日本・中国・純碁、7/9/11/13/15/19路、5段階の難易度に対応する。
日本語GUIでCPUと対局し、採点確認と感想戦まで行う。配布利用者向けの操作説明は [README.txt](README.txt)。

## 構成

この説明の `/` は `C:\dev\PlayGo` を指す。

| 場所 | 役割 |
| --- | --- |
| `/src/cmd/main.go` | 起動引数・終了コード |
| `/src/internal/app/` | 合法手、採点、履歴、プロフィール、難易度、対局状態 |
| `/src/internal/gui/` | 起動・対局・採点・結果・感想戦の画面 |
| `/src/engine/` | KataGo通信、期限、プロセス管理、設定、互換ヘッダー |
| `/src/profile/` | 開発実行時の個人データ。Git・配布対象外 |
| `/src/tools/` | 資産取得、エンジンビルド、ZIP作成、単体応答測定 |
| `/codex/task-status.json` | 旧版T0～T12とv0.2.0のUX-0～UX-7の実行結果。Taskは統合しない |

対局状態はGUIスレッドが所有し、解析は1つの作業・1件の待機キューで実行する。
待った・中止・局面変更では解析を取り消し、世代番号で古い応答を破棄する。
GUI終了時は作業を待ち、KataGoも終了する。
途中解析は最新1件の独立通知で表示へ渡し、CPU着手の確定応答を優先する。
現在局面の結果が届くまでは「解析待ち」を表示し、旧局面の色分けは流用しない。
感想戦では表示局面と直前局面の不足解析を順番に補い、盤面差分から手の効果を説明する。
自動再生は必要な解析を待ち、解析失敗時は停止して再試行を受け付ける。

## 開発環境と資産の準備

Windows 11 / amd64、Go 1.26.4、既存のMinGW GCC 16.1.0を使う。
`dev.cmd` はGoのキャッシュ・一時ファイル・ツールチェーンをプロジェクトの `.cache` 内へ隔離する。
本番ビルドは `CGO_ENABLED=0`。race検査だけ、既存GCCを使用してcgoを有効にする。
初回準備はネット接続が必要。Go・モデルなどのダウンロード先はプロジェクト内で、システム設定は変更しない。
資産取得にはWindowsの `curl.exe` を使い、HTTPS・3分・256MiBの上限とSHA-256照合を適用する。検証済みcurlは8.21.0。

VS CodeのCommand Prompt、または通常のcmdで実行する。

```cmd
cd /d C:\dev\PlayGo\src
dev.cmd version
dev.cmd env GOCACHE GOMODCACHE GOPATH GOTOOLCHAIN
dev.cmd mod download
dev.cmd run ./tools/assets
dev.cmd run ./tools/enginebuild
dev.cmd run ./cmd -root C:\dev\PlayGo\src
```

MinGWが別の場所にある場合、最後から2行目に `-mingw` と既存MinGWの絶対パスを指定する。
コンパイラーは自動インストールしない。CMake 3.31.8、Eigen 3.4.0、zlib 1.3.1はハッシュ照合後に `.cache/native` へ展開する。
エンジンの並列ビルドは4作業、期限は15分。ログは `.cache/native/build-*.log` に保存する。
KataGoはSSE2のCPUビルドとし、Windows標準DLL以外に依存しないことをPEのインポート表で検査する。
MinGW/WindowsのAVX戻り値で異常終了を再現したため、AVX2を有効にしない。

資産の出所は [THIRD_PARTY.md](THIRD_PARTY.md)。URL・ハッシュは `assets.lock.json` で固定する。
内容が変わってハッシュが一致しない場合は停止する。出所をレビューせずハッシュを書き換えない。
資産更新時は `tools/assets/main.go` の入力を変更し、明示的に `dev.cmd run ./tools/assets -record` でlockを生成する。
生成済みライセンス・出所記録・checksumsは直接編集しない。

## 検証

```cmd
cd /d C:\dev\PlayGo\src
check.cmd
dev.cmd test -race -count=1 ./...
dev.cmd mod verify
dev.cmd run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
```

`check.cmd` はgofmt、vet、test、buildを順に実行する。現在は全コードが本実装の対象である。
後日無関係な変更が混在する場合、gofmtは変更パッケージへ限定して実行する。
テストはプロジェクト内の一時データを使用する。実エンジン検査は通常のテストから除外している。

```cmd
cd /d C:\dev\PlayGo\src
set "PLAYGO_ENGINE_ROOT=C:\dev\PlayGo\src"
dev.cmd test -count=1 -run TestRealEngine -v ./engine
set "PLAYGO_ENGINE_ROOT="
dev.cmd run ./cmd -root C:\dev\PlayGo\src -verify C:\dev\PlayGo\.work\acceptance.json
```

受入測定は全ルール・路数・難易度の90条件と、19路のCPU先手・プレイヤーパス6条件で行う。
固定乱数の合法手から序盤・中盤・終盤相当の盤面を作る。
プレイヤー着手の受付からCPU着手の `Draw` 完了までを記録し、5秒超過または解析失敗で失敗終了する。
OSコンポジターやモニターの表示遅延は含まない。初回モデル準備時間は別項目へ記録する。
専用の一時プロフィールだけを使用し、終了時に削除する。実プレイヤーとの対戦成績・難易度の校正試験とは別である。
v0.2.0では、CPU着手前は操作受付から、着手後は新局面の初回描画から、最初の有効な色分け描画までを別記録する。
[校正測定](../codex/ownership-calibration-v0.2.0.json)の最大3,529msに25%の余裕を加え、500ms単位で切り上げた4,500msを色分けの検証上限とする。
CPU着手は従来どおり5,000ms以内。確定採点は色分け測定対象外で、途中局面が先に進んだ場合は未観測と記録する。
色分け上限は確認済みPCでの回帰基準であり、すべてのPCでの保証値ではない。

画面確認には次のコマンドを使う。`startup` を `game` / `game-wait` / `game-pass` / `scoring` / `result` / `choices` / `review` / `review-player` / `review-19` に替えられる。
合成局面であることを画面に明記し、プロフィールを読み書きしない。

```cmd
cd /d C:\dev\PlayGo\src
dev.cmd run ./cmd -preview startup -screenshot C:\dev\PlayGo\.work\startup.png
```

## EXEと配布ZIP

```cmd
cd /d C:\dev\PlayGo\src
dev.cmd run ./tools/package
dev.cmd run ./tools/verify
```

`/dist/PlayGo-v0.2.0-windows-amd64.zip` と `.sha256` を生成する。
GUI EXEは `-trimpath -buildvcs=false -ldflags=-H=windowsgui` で作成し、個人パスを除去する。
デバッグ用情報の一括削除は行わない。署名は付けていない。
ZIPにはEXE、CPUエンジン、モデル、設定、日本語説明、空のprofile、ライセンス、出所・ハッシュを含める。
Microsoft VC、CUDA、OpenCL、OpenSSLのDLLや、ユーザーのプロフィールは含めない。
同梱Eigenソースは未変更で、MPLのソース提供にも対応する。
`tools/verify` はZIPを専用一時フォルダーへ展開し、全ハッシュ・空のprofile・10画面・実エンジンの96条件を検査する。
色分け待ちも測定するため全体の停止期限は20分。展開先は終了時に回収する。
画像は `.work/package-v0.2.0-*.png`、測定値は `/codex/acceptance-v0.2.0.json` に残す。
旧版の `/codex/acceptance.json` は比較用に保持する。

## 採点・推定値・保存の限界

純碁の最終得点は盤上石数で厳密に計算する。途中のCPU・勝率・目差・助言はKataGoの近似値である。
日本・中国では死石候補を表示し、プレイヤーが確認・修正して確定する。
セキ候補は共有する空点と二眼の静的検査からの提案であり、複雑な死活の完全判定ではない。
推定棋力は公式段級位と対応を保証しない。難易度の目標勝数は複数局での調整目安である。
プロフィールはID別JSON、改名時もID維持、Revisionで競合検出、保存前バックアップと同一ディレクトリ置換を行う。
完了した結果要約は最新1000局を保持し、通算成績は残す。対局中の棋譜は終了後に復元しない。

試験機はIntel Core i7-12650H、物理メモリ32,472MB。より低いCPU・メモリの最低条件は未検証。
日本語IMEの実操作、専用GPUなしの別PC、異なるDPI・ディスプレイでの見え方は手動確認が必要。
検証結果は [実行記録](../codex/task-status.json)、[受入測定](../codex/acceptance-v0.2.0.json)、[検証範囲](../codex/validation.md) を参照する。
