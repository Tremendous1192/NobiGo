# 配布資産と出所

作成日: 2026-10-18。PlayGo v0.1.0 の配布物に含める著作権表示を整理する。

PlayGo本体はリポジトリのMITライセンスに従う。第三者の資産には各ライセンスが適用される。
取得URLとSHA-256は `assets.lock.json`、生成したエンジンのSHA-256は `engine/build-provenance.json` に記録する。

| 資産 | 版・出所 | 配布時の表示 |
| --- | --- | --- |
| KataGo | [v1.17.1](https://github.com/lightvector/KataGo/tree/v1.17.1) | `licenses/KataGo/`。MITと同梱ライブラリの表示を保持 |
| モデル | [kata1-b18c384nbt-s9996604416-d4316597426](https://katagotraining.org/networks/) | `licenses/KataGo-model.html`。公式モデルの許諾文を保存 |
| Eigen | [3.4.0](https://gitlab.com/libeigen/eigen/-/tree/3.4.0) | `Eigen-MPL2.txt` と未変更の `Eigen-3.4.0-source.zip` |
| zlib | [1.3.1](https://github.com/madler/zlib/tree/v1.3.1) | `zlib.txt` |
| MinGW / GCCランタイム | 既存のMinGW GCC 16.1.0 | `MinGW*.txt`、`winpthreads.txt`、`GCC-GPL3.txt`、`GCC-Runtime-Exception.txt` |
| Noto Sans JP | [Google Fontsの固定コミット](https://github.com/google/fonts/tree/47d54ca1ebb14fd864695f1d39dc734f39017c33/ofl/notosansjp) | `NotoSansJP-OFL.txt`。フォントは未変更でEXEに埋込み |
| Go / Ebitengineほか | Go 1.26.4、Ebitengine 2.9.9、`go.mod` / `go.sum` | `Go.txt`、`Go-modules/`。ビルド時の実モジュールから収集 |

KataGoはCPU用Eigenバックエンド、SSE2、分散学習無効、libzipなしでビルドする。
旧MSVCRTを使うMinGW向けに、PlayGoの `engine/build/mingw_compat.h` を追加してビルドする。
元のKataGoソースは変更しない。GCC・zlib・スレッドランタイムは静的リンクし、Windows標準DLLへの依存だけを許可する。
MicrosoftのVCランタイム、CUDA、OpenCL、OpenSSLのDLLは配布ZIPに含めない。

AVX2版ではGCC/Windowsの戻り値アラインメント問題に一致する異常終了を再現したため、SSE2を採用した。
根拠は実際のGDBバックトレースと [GCC PR54412の報告](https://gcc.gnu.org/pipermail/gcc-patches/2026-May/717912.html)。

初期性能比較には公式Windows版を使用したが、最終配布物とは別のバイナリである。
最終性能は再ビルドしたエンジンで改めて測定する。

再取得・生成は `tools/assets`、`tools/enginebuild`、`tools/package` が行う。
ライセンス原文・ハッシュ・生成記録へ手でヘッダーを挿入しない。
