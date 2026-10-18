// 作成日: 2026-10-18。資産展開のパス境界と取得メタデータの上限を検証する。
package main

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestArchiveContainment は親参照・絶対パスを拒否し正当な階層だけを展開する。
func TestArchiveContainment(t *testing.T) {
	for _, name := range []string{"../outside.txt", "C:/outside.txt", "safe/file.txt"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			var buf bytes.Buffer
			writer := zip.NewWriter(&buf)
			entry, err := writer.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = entry.Write([]byte("content")); err != nil {
				t.Fatal(err)
			}
			if err = writer.Close(); err != nil {
				t.Fatal(err)
			}
			archive := filepath.Join(root, "input.zip")
			if err = os.WriteFile(archive, buf.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			err = extract(archive, filepath.Join(root, "output"), "native")
			if name != "safe/file.txt" {
				if err == nil {
					t.Fatal("unsafe archive accepted")
				}
				if _, statErr := os.Stat(filepath.Join(root, "outside.txt")); !os.IsNotExist(statErr) {
					t.Fatal("archive escaped output")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(root, "output", "safe", "file.txt"))
			if err != nil || string(data) != "content" {
				t.Fatal("valid archive was not extracted")
			}
		})
	}
}

// TestDownloadBounds はHTTP・巨大なメタデータを拒否する。
func TestDownloadBounds(t *testing.T) {
	if err := download("http://example.invalid/data", filepath.Join(t.TempDir(), "data")); err == nil {
		t.Fatal("insecure URL accepted")
	}
	var output limitedOutput
	if _, err := io.Copy(&output, bytes.NewReader(bytes.Repeat([]byte("a"), 1024))); err != nil {
		t.Fatal(err)
	}
	if _, err := output.Write([]byte("b")); err == nil {
		t.Fatal("metadata limit ignored")
	}
	if len(output.String()) != 1024 {
		t.Fatal("metadata changed after refusal")
	}
}

// TestRealDownload は明示実行時だけcurlのHTTPS取得と固定ハッシュを検証する。
func TestRealDownload(t *testing.T) {
	if os.Getenv("PLAYGO_DOWNLOAD_TEST") != "1" {
		t.Skip("set PLAYGO_DOWNLOAD_TEST=1 for network integration")
	}
	path := filepath.Join(t.TempDir(), "font.ttf")
	a := seeds()[1]
	if err := download(a.URL, path); err != nil {
		t.Fatal(err)
	}
	hash, err := hashFile(path)
	if err != nil || hash != a.SHA256 {
		t.Fatalf("download checksum mismatch: %v", err)
	}
}
