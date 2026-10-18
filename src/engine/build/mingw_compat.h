/**
 * @file mingw_compat.h
 * @date 2026-10-18
 * @brief 旧MSVCRT向けMinGWで欠けるKataGoの異常終了関数を補う。通常の終了処理には使わない。
 */
#pragma once

#include <cstdlib>

#if defined(__MINGW32__) && !defined(_UCRT)
/**
 * @brief KataGoの致命的エラーを、静的オブジェクトの破棄を待たずに終了する。
 * @param status プロセス終了コード。
 */
[[noreturn]] inline void quick_exit(int status) {
  // このKataGo版はat_quick_exitを登録しないため、旧MSVCRTでは同じ即時終了を_Exitで補える。
  _Exit(status);
}
#endif
