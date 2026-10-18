// 作成日: 2026-10-18。使い捨てディレクトリで保存・排他・復旧と入力境界を検証する。
package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// testStore はテスト専用保存先を用意する。
func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestProfileLifecycle はIDを維持する改名、設定復元、バックアップと削除退避を検証する。
func TestProfileLifecycle(t *testing.T) {
	s := testStore(t)
	p, err := s.Create("練習用")
	if err != nil {
		t.Fatal(err)
	}
	id := p.ID
	p.Name = "練習用2"
	p.Last.Size = 19
	p.Last.Rule = Jungo
	p.Ranks[2] = 3
	if err = s.Save(&p); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load(id)
	if err != nil || loaded.Name != p.Name || loaded.Last.Size != 19 || loaded.Ranks[2] != 3 {
		t.Fatalf("roundtrip failed: %v", err)
	}
	backup, err := os.ReadFile(filepath.Join(s.Dir, id+".bak"))
	if err != nil || !bytes.Contains(backup, []byte("練習用")) {
		t.Fatal("backup missing")
	}
	if err = s.Delete(id); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Load(id); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("deleted profile still loads")
	}
	if _, err = os.Stat(filepath.Join(s.Dir, id+".deleted")); err != nil {
		t.Fatal("deleted backup missing")
	}
}

// TestProfileValidation は表示名、重複名、パス逸脱と保存値の範囲を検証する。
func TestProfileValidation(t *testing.T) {
	s := testStore(t)
	for _, name := range []string{"", " leading", "trailing ", "line\nbreak"} {
		if _, err := s.Create(name); err == nil {
			t.Fatalf("accepted invalid name %q", name)
		}
	}
	p, err := s.Create("Player")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Create("PLAYER"); err == nil {
		t.Fatal("accepted duplicate name")
	}
	if _, err = s.Load("../outside"); err == nil {
		t.Fatal("accepted traversal")
	}
	p.Ranks[0] = 39
	if err = s.Save(&p); err == nil {
		t.Fatal("accepted invalid rank")
	}
}

// TestProfileLockAndRevision は他の所有者のロックと新しい内容を保持する。
func TestProfileLockAndRevision(t *testing.T) {
	s := testStore(t)
	p, err := s.Create("Player")
	if err != nil {
		t.Fatal(err)
	}
	stale := p
	lock := filepath.Join(s.Dir, "store.lock")
	if err = os.WriteFile(lock, []byte("test owner"), 0600); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(s.Save(&p), ErrStoreBusy) {
		t.Fatal("existing lock ignored")
	}
	if data, err := os.ReadFile(lock); err != nil || string(data) != "test owner" {
		t.Fatal("foreign lock modified")
	}
	if err = os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	p.Name = "New name"
	if err = s.Save(&p); err != nil {
		t.Fatal(err)
	}
	stale.Name = "Stale name"
	if err = s.Save(&stale); err == nil {
		t.Fatal("stale revision overwrote data")
	}
	current, err := s.Load(p.ID)
	if err != nil || current.Name != "New name" {
		t.Fatal("new revision not preserved")
	}
}

// TestCorruptProfileRecovery は破損した1人を分離しバックアップから復旧できることを検証する。
func TestCorruptProfileRecovery(t *testing.T) {
	s := testStore(t)
	p, err := s.Create("Player A")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Create("Player B"); err != nil {
		t.Fatal(err)
	}
	p.Name = "Renamed"
	if err = s.Save(&p); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Dir, p.ID+".json")
	if err = os.WriteFile(path, []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	valid, broken, err := s.List()
	if err != nil || len(valid) != 1 || len(broken) != 1 || broken[0] != p.ID {
		t.Fatal("corrupt profile was not isolated")
	}
	if err = s.Restore(p.ID); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.Load(p.ID)
	if err != nil || recovered.Name != "Player A" {
		t.Fatal("recovery failed")
	}
	if data, err := os.ReadFile(filepath.Join(s.Dir, p.ID+".recovery")); err != nil || string(data) != "{broken" {
		t.Fatal("corrupt original was not preserved")
	}
	if err = os.WriteFile(path, []byte("{another failure"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = s.Restore(p.ID); err == nil {
		t.Fatal("existing recovery file was overwritten")
	}
	for name, want := range map[string]string{p.ID + ".json": "{another failure", p.ID + ".recovery": "{broken"} {
		if data, err := os.ReadFile(filepath.Join(s.Dir, name)); err != nil || string(data) != want {
			t.Fatalf("recovery refusal did not preserve %s", name)
		}
	}
}

// TestAtomicWriteFailure は置換失敗時に既存データを維持し一時ファイルを回収する。
func TestAtomicWriteFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	fault := errors.New("injected replacement failure")
	if err := atomicWrite(path, []byte("new"), func(string, string) error { return fault }); !errors.Is(err, fault) {
		t.Fatal("replacement failure was hidden")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "old" {
		t.Fatal("old content changed")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("temporary file leaked")
	}
}

// TestProfileParserBoundaries は未知フィールド、余分なJSON、不正な配列長と巨大入力を拒否する。
func TestProfileParserBoundaries(t *testing.T) {
	s := testStore(t)
	p, err := s.Create("Player")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	cases := [][]byte{append(append([]byte(nil), data...), []byte(" {}")...), bytes.Replace(data, []byte(`"version":1`), []byte(`"extra":true,"version":1`), 1), bytes.Replace(data, []byte(`"ranks":[0,0,0]`), []byte(`"ranks":[0,0,0,0]`), 1), bytes.Repeat([]byte(" "), maxProfileBytes+1)}
	for i, input := range cases {
		if _, err := decodeProfile(input, p.ID); err == nil {
			t.Fatalf("accepted invalid input %d", i)
		}
	}
}
