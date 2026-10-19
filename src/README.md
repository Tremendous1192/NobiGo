# NobiGo v0.7.0

作成日: 2026-10-18。初心者向けのWindows囲碁ゲーム。日本・中国・純碁、7/9/11/13/15/19路、5段階の難易度に対応する。
日本語GUIでCPUと対局し、採点確認と感想戦まで行う。配布利用者向けの操作説明は [README.txt](README.txt)。
先攻・後攻の初期選択は「ランダム」。登録済みプレイヤーでは保存済みの選択を復元する。
対局画面の上部はアプリ名・ルール・盤サイズ・そのルールのプレイヤー棋力・CPU難易度の順に表示する。
右上はプレイヤー名と石色。長い名前だけを省略し、石色は常に残す。

## 構成

この説明の `/` は `C:\dev\NobiGo` を指す。

| 場所 | 役割 |
| --- | --- |
| `/src/cmd/main.go` | 起動引数・終了コード |
| `/src/internal/app/` | 合法手、採点、履歴、プロフィール、難易度、対局状態 |
| `/src/internal/gui/` | 起動・対局・採点・結果・感想戦の画面 |
| `/src/engine/` | KataGo通信、期限、プロセス管理、設定、互換ヘッダー |
| `/src/profile/` | 開発実行時の個人データ。Git・配布対象外 |
| `/src/tools/` | 資産取得、エンジンビルド、ZIP作成、単体応答測定 |
| `/codex/task-status.json` | 旧版の履歴とv0.7.0のT1～T4の実行結果。Taskは統合しない |

対局状態はGUIスレッドが所有し、解析は1つの作業・1件の待機キューで実行する。
待った・中止・局面変更では解析を取り消し、世代番号で古い応答を破棄する。
GUI終了時は作業を待ち、KataGoも終了する。
途中解析は最新1件の独立通知で表示へ渡し、CPU着手の確定応答を優先する。
現在局面の結果が届くまでは「解析待ち」を表示し、旧局面の色分けは流用しない。
感想戦では表示局面と直前局面の不足解析を順番に補い、盤面差分から手の効果を説明する。
自動再生は必要な解析を待ち、解析失敗時は停止して再試行を受け付ける。
目差（左軸・実線）と勝率（右軸・破線）は高さ85論理pxの共通グラフへ表示する。
数値欄は目差・勝率・直前損失を1行とし、純碁の「近似」を行末へ小さく表示する。
助言の要約は危険・効果・確認事項を優先する3行以内とする。
助言は「要点＋詳しく」。詳細は座標・石数・呼吸点を使った原則4～6文で、複合効果と危険も併記する。
自分の手は次局の学び、CPUの手は推定される狙いと確認事項を示す。比較候補には既存解析と合法性確認を使う。
詳細は文章と5路分の部分図を並べる。範囲内の石のつながりを示す。選択した連は外周を囲む。
部分図の横には同じ前後局面の全体盤を置き、部分図に表示する範囲を枠で示す。枠は対象切替・移動・選択位置への復帰に追従する。
詳細を開く前と閉じた後も、通常の感想戦盤面で同じ範囲を囲む。同じ手では対象・移動・前後の選択を保持し、手の移動や自動再生で新しい範囲へ更新する。
選択した連の石数は盤全体から求め、図外にある同じ連の石数も表示する。
説明対象・着手前後の切替、矢印での図の移動、「選択へ」での復帰に対応する。
感想戦だけで候補別の所有予測・探索回数付き読み筋を取得し、追加データは表示前後の2局面に限定する。感想戦の探索は最大4.5秒、通常対局は従来の2.5秒とする。
石の輪はAIの生存見込み（緑＝安定、赤＝注意、灰＝未確定）。2手以上の合法な読み筋を2本以上確認し、すべての採用変化と元局面で連の全石の同色所有予測が0.6以上、捕獲もなければ安定とする。採用変化で全石が-0.6以下または捕獲されれば注意とする。所有予測だけでは安定と判定せず、生存確率とも扱わない。
別々の同色連で呼吸点を共有する箇所は、探索回数上位8候補・各16手までの合法な変化を確認する。各手2訪問以上の部分だけを使う。
共有点への相手の攻めを含む複数変化で、生存・連結・同色所有予測0.6以上が維持された連絡は緑の実線。確認した攻めで連絡を保てない場合は赤の破線、それ以外は灰の点線。連結済みの石は青線。
これらは読めた範囲の推定で、死活・切断耐性の保証ではない。未探索の連絡やコウ・セキは未確定になり得る。
根拠は[KataGo v1.17.1の解析仕様](https://github.com/lightvector/KataGo/blob/v1.17.1/docs/Analysis_Engine.md)。追加取得は通常対局へ適用しない。
詳細を閉じた盤面にも同じ注釈を枠内だけに描き、常に表示中の着手後局面を使う。全体盤の小窓には注釈を重ねない。
詳細を開くと再生は停止し、閉じても自動再開しない。本文と局面は固定し、AIの図示だけは解析到着時に更新する。比較候補の本文は開き直すと更新する。
ホイール・上下ボタン・↑↓・PageUp/PageDown・Home/Endで全文を読み、Escまたは閉じるボタンで戻る。

終了後の4択から「感想戦（概要）」を開くと、詳細と同サイズの4ページを表示する。
結果とハイライト、CPU戦略の推定、プレイヤーの好手、次局の改善点を順番に示す。
全棋譜を一度だけ再生して取り・救出・連結を集計し、手数を三等分した区間からCPUの傾向を説明する。
ハイライトは解析済みの最大目差変動、好手は損失1目以内で効果またはAI第一候補の根拠がある手、改善は損失1目以上で合法な比較候補がある手から選ぶ。自分のアタリは好手から除外する。
評価済み手数を明示し、全局の最善・最大を保証しない。確定結果とAI予測を分け、純碁の予測には近似を付ける。
通常解析のキャッシュを再利用し、区間別候補と盤面効果から不足局面を最大12件、全体60秒まで順次補う。
概要には大きな読み筋配列を追加取得しない。解析中もページ移動・終了でき、失敗時だけ明示的に再試行する。
終了・次局で未完了解析を取り消し、旧応答は破棄する。棋譜・プロフィール・戦績・棋力を再保存しない。

## 開発環境と資産の準備

Windows 11 / amd64、Go 1.26.4、既存のMinGW GCC 16.1.0を使う。
`dev.cmd` はGoのキャッシュ・一時ファイル・ツールチェーンをプロジェクトの `.cache` 内へ隔離する。
Goモジュール名は `github.com/Tremendous1192/NobiGo`。テスト用の `PLAYGO_*` 環境変数名は旧版との互換性のため維持する。
本番ビルドは `CGO_ENABLED=0`。race検査だけ、既存GCCを使用してcgoを有効にする。
初回準備はネット接続が必要。Go・モデルなどのダウンロード先はプロジェクト内で、システム設定は変更しない。
資産取得にはWindowsの `curl.exe` を使い、HTTPS・3分・256MiBの上限とSHA-256照合を適用する。検証済みcurlは8.21.0。

VS CodeのCommand Prompt、または通常のcmdで実行する。

```cmd
cd /d C:\dev\NobiGo\src
dev.cmd version
dev.cmd env GOCACHE GOMODCACHE GOPATH GOTOOLCHAIN
dev.cmd mod download
dev.cmd run ./tools/assets
dev.cmd run ./tools/enginebuild
dev.cmd run ./cmd -root C:\dev\NobiGo\src
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
cd /d C:\dev\NobiGo\src
check.cmd
dev.cmd test -race -count=1 ./...
dev.cmd mod verify
dev.cmd run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
```

`check.cmd` はgofmt、vet、test、buildを順に実行する。
無関係な変更が混在する場合、gofmtは変更パッケージへ限定し、vet・test・buildは全体へ実行する。
テストはプロジェクト内の一時データを使用する。実エンジン検査は通常のテストから除外している。

```cmd
cd /d C:\dev\NobiGo\src
set "PLAYGO_ENGINE_ROOT=C:\dev\NobiGo\src"
dev.cmd test -count=1 -run TestRealEngine -v ./engine
set "PLAYGO_ENGINE_ROOT="
dev.cmd run ./cmd -root C:\dev\NobiGo\src -verify C:\dev\NobiGo\.work\acceptance.json
```

受入測定は全ルール・路数・難易度の90条件と、19路のCPU先手・プレイヤーパス6条件で行う。
固定乱数の合法手から序盤・中盤・終盤相当の盤面を作る。
プレイヤー着手の受付からCPU着手の `Draw` 完了までを記録し、7.5秒超過または解析失敗で失敗終了する。
OSコンポジターやモニターの表示遅延は含まない。初回モデル準備時間は別項目へ記録する。
専用の一時プロフィールだけを使用し、終了時に削除する。実プレイヤーとの対戦成績・難易度の校正試験とは別である。
v0.2.0では、CPU着手前は操作受付から、着手後は新局面の初回描画から、最初の有効な色分け描画までを別記録する。
[校正測定](../codex/ownership-calibration-v0.2.0.json)の最大3,529msに25%の余裕を加え、500ms単位で切り上げた4,500msを色分けの検証上限とする。
CPU着手は7,500ms以内。確定採点は色分け測定対象外で、途中局面が先に進んだ場合は未観測と記録する。
色分け上限は確認済みPCでの回帰基準であり、すべてのPCでの保証値ではない。
続けて日本・中国・純碁の7路/19路の6条件で、解析キャッシュなしの概要を最終描画まで測る。
概要は最大12局面・60秒以内、終了後の次局CPU先手は7.5秒以内を確認する。

画面確認には次のコマンドを使う。`startup` を `game` / `game-wait` / `game-pass` / `scoring` / `result` / `choices` / `review` / `review-player` / `review-19` に替えられる。
詳細の確認は `review-detail` / `review-detail-cpu` / `review-detail-combined` / `review-detail-wait` / `review-detail-error` / `review-detail-long` / `review-detail-bottom` / `review-detail-min`。
初回準備の表示例は `startup-check` / `startup-download` / `startup-verify` / `startup-load` / `startup-error` / `startup-repair` / `startup-cancel`。
対局上部は `game-header-7` / `game-header-9` / `game-header-11` / `game-header-13` / `game-header-15` / `game-header-19`、
`game-header-chinese` / `game-header-jungo` / `game-header-long` / `game-header-min` で棋力・石色・名前の表示を確認する。
追加の確認例は `game-white-jungo` / `game-extremes` / `game-gap` と、
`review-detail-before` / `review-detail-outside` / `review-detail-pass` / `review-detail-empty`。
全対応路数は `review-detail-7` / `review-detail-9` / `review-detail-11` / `review-detail-13` / `review-detail-15` / `review-detail-19`。
AI推定の合成例は `review-ai` / `review-ai-19` / `review-detail-ai` / `review-detail-ai-risk`。
概要の合成例は `summary-1`～`summary-4`、`summary-wait` / `summary-error` / `summary-missing` / `summary-white` / `summary-jungo` / `summary-draw` / `summary-empty` / `summary-noresult` / `summary-long` / `summary-bottom` / `summary-min`。
概要の全対応路数は `summary-7` / `summary-9` / `summary-11` / `summary-13` / `summary-15` / `summary-19`。
詳細を閉じた後の範囲枠は `review-focus-moved`（移動後の19路）/ `review-focus-before`（着手前を選んで閉じた7路）。
`game-header-min` / `review-detail-min` / `summary-min` は885×600で起動し、同サイズへ縮小した描画を保存する。OSや異なるDPIの見え方を保証する画像ではない。
合成局面であることを画面に明記し、プロフィールを読み書きしない。

```cmd
cd /d C:\dev\NobiGo\src
dev.cmd run ./cmd -preview startup -screenshot C:\dev\NobiGo\.work\startup.png
```

## EXEと配布ZIP

```cmd
cd /d C:\dev\NobiGo\src
dev.cmd run ./tools/package
dev.cmd run ./tools/verify -download-model
```

`/dist/NobiGo-v0.7.0-windows-amd64.zip` と `.sha256` を生成する。
GUI EXEは `-trimpath -buildvcs=false -ldflags=-H=windowsgui` で作成し、個人パスを除去する。
デバッグ用情報の一括削除は行わない。署名は付けていない。
ZIPにはEXE、CPUエンジン、設定、日本語説明、空のprofile、モデルを含むライセンス、出所・ハッシュを含める。
モデルは同梱しない。初回起動時に `assets.lock.json` の固定モデルだけをHTTPSで取得し、SHA-256照合後に `engine/runtime/model.bin.gz` へ保存する。
取得はWindowsの `curl.exe` を使用し、最大3分・256MiB。検証済みモデルがある起動では通信せず、自動更新もしない。
破損時は「モデルを再取得」で明示修復する。失敗時は既存モデルを保持し、一時ファイルを回収する。
取得と照合、エンジン読込みは別段階で表示・測定する。読込み期限は独立した30秒、CPU着手7.5秒の計測には含めない。
Microsoft VC、CUDA、OpenCL、OpenSSLのDLLや、ユーザーのプロフィールは含めない。
同梱Eigenソースは未変更で、MPLのソース提供にも対応する。
`tools/verify -download-model` は明示的に実ダウンロードを許可する統合検査。ZIPを専用一時フォルダーへ展開し、全ハッシュ・空のprofile・75画面を確認する。
配布EXEの `-prepare <出力JSON>` で初回取得と次回起動を測る。このオプションはプロフィールを読み書きせず、準備後に終了する。
取得後の起動・CPU着手96条件・概要6条件は、子プロセスだけPATHを空にし、接続できないプロキシを設定して実行する。物理的なネット切断試験とは区別する。
同梱ファイルは `checksums.json`、追加されたモデルは `assets.lock.json` で照合し、ZIP・同梱ファイルの不変を確認する。
全体の停止期限は25分。自分で作った展開先は成功・失敗とも終了時に回収する。
画像は `.work/package-v0.7.0-*.png`、測定値は `/codex/acceptance-v0.7.0.json` に残す。
初回取得と再起動の測定は `/codex/preparation-v0.7.0.json` に記録する。旧版の測定値は比較用に保持する。

Git管理対象は `/dist/NobiGo-v0.7.0-windows-amd64.zip` だけで、100MiB未満を生成時に検査する。
`*.sha256` はGit管理外だが生成を続ける。検証時は存在すれば照合必須、なければZIP内全ファイルとモデルの照合を行う。
次のリリースでは `.gitignore` の許可名を更新し、追跡済みの旧ZIPは対象を確認して `git rm --cached -- dist/<旧ZIP名>` で追跡だけ解除する。
この操作はリポジトリルートで行う。旧ZIPの実ファイルと過去の履歴は保持する。

## 採点・推定値・保存の限界

純碁の最終得点は盤上石数で厳密に計算する。途中のCPU・勝率・目差・比較候補にはKataGoの近似評価を使う。
助言の盤面変化は実際の前後局面から求める。未解析の優劣・数手先の結果・CPUの意図は断定しない。
日本・中国では死石候補を表示し、プレイヤーが確認・修正して確定する。
セキ候補は共有する空点と二眼の静的検査からの提案であり、複雑な死活の完全判定ではない。
推定棋力は公式段級位と対応を保証しない。難易度の目標勝数は複数局での調整目安である。
プロフィールはID別JSON、改名時もID維持、Revisionで競合検出、保存前バックアップと同一ディレクトリ置換を行う。
PlayGoから移行する場合は両方のアプリを終了し、旧版の `profile` 全体をバックアップして、新規展開したNobiGoの空の `profile` へコピーする。ID・JSON形式は変更しない。既存データ同士を上書き・統合しない。
完了した結果要約は最新1000局を保持し、通算成績は残す。対局中の棋譜は終了後に復元しない。

最低CPU・メモリは未定。今回のPC構成と実測結果は検証記録へ記載し、その環境での結果として公開する。
日本語IMEの実操作、専用GPUなしの別PC、異なるDPI・ディスプレイでの見え方は手動確認が必要。
検証結果は [実行記録](../codex/task-status.json)、[v0.7.0の受入測定](../codex/acceptance-v0.7.0.json)、[検証範囲](../codex/validation.md) を参照する。
