// 作成日: 2026-10-18。盤面の交点と画面外の対応を検証する。
package gui

import (
	"math"
	"testing"
)

// TestBoardPoint は全対応路数の四隅・中央と領域外クリックを検証する。
func TestBoardPoint(t *testing.T) {
	for _, size := range []int{7, 9, 11, 13, 15, 19} {
		for _, point := range []int{0, size - 1, size * (size - 1), size*size - 1, size * size / 2} {
			step := 530.0 / float64(size-1)
			x, y := int(math.Round(70+float64(point%size)*step)), int(math.Round(155+float64(point/size)*step))
			if got := boardPoint(x, y, size); got != point {
				t.Fatalf("size=%d point=%d got=%d", size, point, got)
			}
		}
		if boardPoint(0, 0, size) != -2 || boardPoint(700, 700, size) != -2 {
			t.Fatal("outside accepted")
		}
	}
}
