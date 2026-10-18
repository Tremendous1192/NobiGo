// 作成日: 2026-10-18。日本語・結合文字・選択範囲の編集を検証する。IME実操作とは分けて扱う。
package gui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"testing"
)

// TestEditName はUTF-8と表示上の文字を壊さず削除・移動することを検証する。
func TestEditName(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		start, end  int
		key         ebiten.Key
		want        string
		cursor      int
	}{
		{"japanese", "純碁", 6, 6, ebiten.KeyBackspace, "純", 3},
		{"combining", "か\u3099", 6, 6, ebiten.KeyBackspace, "", 0},
		{"emoji", "A👩‍💻", len("A👩‍💻"), len("A👩‍💻"), ebiten.KeyBackspace, "A", 1},
		{"selection", "純碁", 0, 3, ebiten.KeyDelete, "碁", 0},
		{"forward", "純碁", 0, 0, ebiten.KeyDelete, "碁", 0},
		{"left", "純碁", 3, 3, ebiten.KeyLeft, "純碁", 0},
		{"right", "純碁", 0, 0, ebiten.KeyRight, "純碁", 3},
		{"empty", "", 0, 0, ebiten.KeyBackspace, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, cursor := editName(tc.value, tc.start, tc.end, tc.key)
			if got != tc.want || cursor != tc.cursor {
				t.Fatalf("got %q %d", got, cursor)
			}
		})
	}
}
