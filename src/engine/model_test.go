// 作成日: 2026-10-19。モデル準備の通信条件・保存・取消・競合を使い捨てデータで検証する。
package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// modelFixture は実ネットワークを使わないモデル定義を保存する。
func modelFixture(t *testing.T) (string, []byte) {
	t.Helper()
	root, data := t.TempDir(), []byte("verified model fixture")
	a := modelAsset{Name: "model.bin.gz", URL: "https://media.katagotraining.org/uploaded/networks/models/test.bin.gz", SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), Destination: "engine/runtime/model.bin.gz", Kind: "file"}
	encoded, err := json.Marshal([]modelAsset{a})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "assets.lock.json"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	return root, data
}

// assertModelCleanup は自分のロックと一時ファイルが残らないことを検証する。
func assertModelCleanup(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, "engine", "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "model.bin.gz" {
			t.Fatalf("unexpected preparation residue: %s", e.Name())
		}
	}
}

// TestModelFirstDownloadAndOfflineReuse は初回だけ取得し、次回は通信せず照合できることを検証する。
func TestModelFirstDownloadAndOfflineReuse(t *testing.T) {
	root, data := modelFixture(t)
	calls := 0
	var stages []ModelStage
	fetch := func(ctx context.Context, source, target string) error {
		calls++
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("missing download deadline")
		}
		return os.WriteFile(target, data, 0600)
	}
	if err := prepareModel(context.Background(), root, false, func(s ModelStage) { stages = append(stages, s) }, fetch); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(stages, []ModelStage{ModelChecking, ModelDownloading, ModelVerifying}) {
		t.Fatalf("unexpected stages: %v", stages)
	}
	if err := prepareModel(context.Background(), root, false, nil, func(context.Context, string, string) error {
		t.Fatal("offline startup attempted network")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || VerifyModel(context.Background(), root) != nil {
		t.Fatal("model was not reused")
	}
	assertModelCleanup(t, root)
}

// TestModelFailurePreservesExisting は明示修復・破損応答・通信失敗・空応答・保存失敗を区別する。
func TestModelFailurePreservesExisting(t *testing.T) {
	for _, mode := range []string{"explicit repair", "bad hash", "network", "empty", "save failure"} {
		t.Run(mode, func(t *testing.T) {
			root, data := modelFixture(t)
			dir, err := modelDirectory(root, true)
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(dir, "model.bin.gz")
			old := []byte("corrupt model")
			if err = os.WriteFile(target, old, 0600); err != nil {
				t.Fatal(err)
			}
			called := false
			fetch := func(_ context.Context, _ string, temp string) error {
				called = true
				switch mode {
				case "network":
					return errors.New("offline")
				case "empty":
					return nil
				case "bad hash":
					return os.WriteFile(temp, []byte("wrong download"), 0600)
				case "save failure":
					// 自分の一時ファイルだけを除去し、確定前のI/O失敗を再現する。
					return os.Remove(temp)
				}
				return os.WriteFile(temp, data, 0600)
			}
			err = prepareModel(context.Background(), root, mode != "explicit repair", nil, fetch)
			if err == nil || mode == "explicit repair" && (!errors.Is(err, ErrModelInvalid) || called) {
				t.Fatalf("unexpected repair result: %v", err)
			}
			got, err := os.ReadFile(target)
			if err != nil || string(got) != string(old) {
				t.Fatal("failed preparation changed existing model")
			}
			assertModelCleanup(t, root)
			if err = prepareModel(context.Background(), root, true, nil, func(_ context.Context, _, path string) error { return os.WriteFile(path, data, 0600) }); err != nil {
				t.Fatal(err)
			}
			if err = VerifyModel(context.Background(), root); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestModelCancellationAndLock は取得を一つに限定し、取消後に自分の資源だけを回収する。
func TestModelCancellationAndLock(t *testing.T) {
	root, _ := modelFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, done := make(chan struct{}), make(chan error, 1)
	// このテストが所有し、明示取消後にdoneを待つ。startedで競合発生点を固定する。
	go func() {
		done <- prepareModel(ctx, root, false, nil, func(ctx context.Context, _, _ string) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		})
	}()
	<-started
	err := prepareModel(context.Background(), root, false, nil, func(context.Context, string, string) error {
		t.Error("concurrent download started")
		return nil
	})
	if err == nil {
		t.Error("model lock was ignored")
	}
	if _, err = os.Stat(filepath.Join(root, "engine", "runtime", "model.lock")); err != nil {
		t.Error("another owner's lock was removed")
	}
	cancel()
	if err = <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	assertModelCleanup(t, root)
}

// TestModelSourceValidation は不正な取得先・保存先・ハッシュ・JSONを通信前に拒否する。
func TestModelSourceValidation(t *testing.T) {
	for _, mode := range []string{"http", "host", "destination", "hash", "kind", "missing", "duplicate", "unknown", "trailing", "null", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			root, _ := modelFixture(t)
			a, err := readModelAsset(root)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "http":
				a.URL = strings.Replace(a.URL, "https:", "http:", 1)
			case "host":
				a.URL = "https://example.com/uploaded/networks/models/test.bin.gz"
			case "destination":
				a.Destination = "../model.bin.gz"
			case "hash":
				a.SHA256 = strings.Repeat("x", 64)
			case "kind":
				a.Kind = "source"
			case "missing":
				a.Name = "other"
			}
			list := []modelAsset{a}
			if mode == "duplicate" {
				list = append(list, a)
			}
			data, err := json.Marshal(list)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "unknown":
				data = []byte(`[{"unknown":true}]`)
			case "trailing":
				data = append(data, []byte(` {}`)...)
			case "null":
				data = []byte("null")
			case "oversize":
				data = []byte(strings.Repeat(" ", (1<<20)+1))
			}
			if err = os.WriteFile(filepath.Join(root, "assets.lock.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			err = prepareModel(context.Background(), root, false, nil, func(context.Context, string, string) error { t.Error("invalid source fetched"); return nil })
			if err == nil {
				t.Fatal("invalid source accepted")
			}
		})
	}
}

// TestModelSizeAndMetadataBounds は空・上限超過のモデルと応答メタデータを拒否する。
func TestModelSizeAndMetadataBounds(t *testing.T) {
	root, _ := modelFixture(t)
	dir, err := modelDirectory(root, true)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "model.bin.gz")
	for _, size := range []int64{0, modelLimit + 1} {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = errors.Join(f.Truncate(size), f.Close()); err != nil {
			t.Fatal(err)
		}
		if err = VerifyModel(context.Background(), root); !errors.Is(err, ErrModelInvalid) {
			t.Fatalf("invalid size accepted: %v", err)
		}
	}
	b := &modelMetadata{}
	if _, err = b.Write(make([]byte, 1024)); err != nil {
		t.Fatal(err)
	}
	if _, err = b.Write([]byte{0}); err == nil {
		t.Fatal("unbounded metadata")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = prepareModel(ctx, root, true, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation ignored: %v", err)
	}
}

// TestModelResponseLineEndings は実Windows curlのCRLFを受理し、失敗状態・HTML・欠損を拒否する。
func TestModelResponseLineEndings(t *testing.T) {
	for _, tc := range []struct {
		response string
		valid    bool
	}{
		{"200\r\napplication/octet-stream", true}, {"200\napplication/octet-stream", true},
		{"200\r\napplication/gzip", true}, {"404\r\napplication/octet-stream", false},
		{"206\napplication/octet-stream", false}, {"200\ntext/html; charset=utf-8", false},
		{"200\r\n", false}, {"200", false}, {"\napplication/octet-stream", false},
	} {
		if err := validateModelResponse([]byte(tc.response)); (err == nil) != tc.valid {
			t.Fatalf("unexpected response validation for %q: %v", tc.response, err)
		}
	}
}
