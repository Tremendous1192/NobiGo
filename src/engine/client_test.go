// 作成日: 2026-10-18。ローカルの偽子プロセスで通信・取消・不正応答・終了回収を検証する。
package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestMain は専用環境変数がある子プロセスだけを偽エンジンとして動かす。
func TestMain(m *testing.M) {
	if mode := os.Getenv("PLAYGO_TEST_ENGINE"); mode != "" {
		if err := helperEngine(mode); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// helperEngine は決まった応答だけを返し、外部アクセスやファイル更新を行わない。
func helperEngine(mode string) error {
	scan := bufio.NewScanner(os.Stdin)
	encode := json.NewEncoder(os.Stdout)
	count := 0
	for scan.Scan() {
		var q request
		if err := json.Unmarshal(scan.Bytes(), &q); err != nil {
			return err
		}
		if q.Action == "query_models" {
			if err := encode.Encode(wireResponse{ID: q.ID, Models: []json.RawMessage{json.RawMessage(`{}`)}}); err != nil {
				return err
			}
			continue
		}
		if q.Action == "terminate" {
			if err := encode.Encode(wireResponse{ID: q.TerminateID, NoResults: true}); err != nil {
				return err
			}
			continue
		}
		count++
		if mode == "hold" && count == 1 {
			continue
		}
		if mode == "malformed" {
			_, err := fmt.Fprintln(os.Stdout, "{broken")
			return err
		}
		if mode == "exit" {
			return nil
		}
		r := wireResponse{ID: q.ID, Root: &moveInfo{Winrate: .6, ScoreLead: 2}, Moves: []moveInfo{{Move: "D4", Visits: 5, Winrate: .6, ScoreLead: 2, Prior: .1}}, Ownership: make([]float64, q.SizeX*q.SizeY), Policy: make([]float64, q.SizeX*q.SizeY+1)}
		if mode == "short" {
			r.Ownership = nil
		}
		if mode == "partial" && count == 1 {
			r.During = true
			if err := encode.Encode(r); err != nil {
				return err
			}
			continue
		}
		if err := encode.Encode(r); err != nil {
			return err
		}
	}
	return scan.Err()
}

// fakeClient はOSの子プロセスとして偽エンジンを起動し、テスト終了時に回収する。
func fakeClient(t *testing.T, mode string) *Client {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, exe)
	cmd.Env = append(os.Environ(), "PLAYGO_TEST_ENGINE="+mode)
	cmd.WaitDelay = time.Second
	client, err := startCommand(context.Background(), cmd, cancel)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	return client
}

// testPosition はネットワークに依存しない空盤を返す。
func testPosition() Position { return Position{Size: 9, Rules: "japanese", Komi: 6.5} }

// TestVertexRoundTrip は全対応サイズの座標変換と不正座標を検証する。
func TestVertexRoundTrip(t *testing.T) {
	for _, size := range []int{7, 9, 11, 13, 15, 19} {
		for p := -1; p < size*size; p++ {
			v, err := Vertex(p, size)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Point(v, size)
			if err != nil || got != p {
				t.Fatalf("roundtrip: %d %s %d %v", p, v, got, err)
			}
		}
	}
	for _, v := range []string{"I1", "A0", "T20", "resign", "A-1", ""} {
		if _, err := Point(v, 19); err == nil {
			t.Fatalf("accepted %s", v)
		}
	}
}

// TestAnalyzeAndClose は解析値の変換とCloseの冪等性を検証する。
func TestAnalyzeAndClose(t *testing.T) {
	c := fakeClient(t, "normal")
	a, err := c.Analyze(context.Background(), testPosition())
	if err != nil || a.ScoreLead != 2 || a.Winrate != .6 || len(a.Ownership) != 81 {
		t.Fatalf("analysis failed: %+v %v", a, err)
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Analyze(context.Background(), testPosition()); err == nil {
		t.Fatal("closed engine accepted query")
	}
}

// TestQueryCancellationLeavesClientUsable は未応答の要求を取消し次の解析が成功することを検証する。
func TestQueryCancellationLeavesClientUsable(t *testing.T) {
	c := fakeClient(t, "hold")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := c.Analyze(ctx, testPosition()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline: %v", err)
	}
	if _, err := c.Analyze(context.Background(), testPosition()); err != nil {
		t.Fatalf("client unusable after cancellation: %v", err)
	}
}

// TestDeadlineCanReturnValidPartial は期限内に届いた有効な途中評価だけを利用することを検証する。
func TestDeadlineCanReturnValidPartial(t *testing.T) {
	c := fakeClient(t, "partial")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	a, err := c.Analyze(ctx, testPosition())
	if err != nil || !a.Partial || len(a.Moves) != 1 {
		t.Fatalf("partial response not used: %v", err)
	}
}

// TestMalformedAndMissingOutput は壊れたJSON・欠損配列・異常終了を成功と扱わないことを検証する。
func TestMalformedAndMissingOutput(t *testing.T) {
	for _, mode := range []string{"malformed", "short", "exit"} {
		t.Run(mode, func(t *testing.T) {
			c := fakeClient(t, mode)
			if _, err := c.Analyze(context.Background(), testPosition()); err == nil {
				t.Fatal("invalid response accepted")
			}
		})
	}
}

// TestInvalidEngineInput は局面と数値の検証が送信前に行われることを検証する。
func TestInvalidEngineInput(t *testing.T) {
	p := testPosition()
	p.Moves = []Ply{{Color: "W", Point: 0}}
	if _, err := makeRequest(p, "1"); err == nil {
		t.Fatal("wrong color accepted")
	}
	r := wireResponse{Root: &moveInfo{Winrate: 2}, Moves: []moveInfo{{Move: "A1"}}, Ownership: make([]float64, 81), Policy: make([]float64, 82)}
	if _, err := decodeAnalysis(r, 9); err == nil {
		t.Fatal("invalid winrate accepted")
	}
}
