// 作成日: 2026-10-18。ZIPのルート構造・空の保存先・内容の再現性を検証する。
package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestPackageArchive は利用者が展開する構造と元データの一致を検証する。
func TestPackageArchive(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(source, "profile"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "README.txt"), []byte("日本語の説明"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "package.zip")
	if err := makeZip(source, path); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
	}()
	if len(reader.File) != 3 {
		t.Fatalf("unexpected entries: %d", len(reader.File))
	}
	got := make(map[string]bool)
	for _, file := range reader.File {
		got[file.Name] = true
		if file.Name != "NobiGo/README.txt" {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(stream)
		closeErr := stream.Close()
		if err != nil || closeErr != nil || string(data) != "日本語の説明" {
			t.Fatal("archive data differs")
		}
	}
	for _, name := range []string{"NobiGo/", "NobiGo/profile/", "NobiGo/README.txt"} {
		if !got[name] {
			t.Fatalf("missing entry: %s", name)
		}
	}
	second := filepath.Join(root, "second.zip")
	if err = makeZip(source, second); err != nil {
		t.Fatal(err)
	}
	firstHash, err := fileHash(path)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := fileHash(second)
	if err != nil || firstHash != secondHash {
		t.Fatal("unchanged input produced a different archive")
	}
}
