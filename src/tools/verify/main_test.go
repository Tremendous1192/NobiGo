// 作成日: 2026-10-19。改名後のZIP構造・個人データ除外・必須EXE名を、使い捨ての仮資産で検証する。
package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

type archiveEntry struct {
	name string
	mode fs.FileMode
}

// testArchive は外部資産を使わず検証用の小さなZIPを作る。
func testArchive(t *testing.T, entries []archiveEntry) string {
	t.Helper()
	var data bytes.Buffer
	w := zip.NewWriter(&data)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Store}
		header.SetMode(entry.mode)
		out, err := w.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if !entry.mode.IsDir() {
			if _, err = out.Write([]byte("fixture")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "package.zip")
	if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestExtractNobiGo は新しいルートと空の保存先だけを受理し、旧名・個人データ・不正パスを拒否する。
func TestExtractNobiGo(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []archiveEntry
		wantErr bool
	}{
		{"current root", []archiveEntry{{"NobiGo/", fs.ModeDir | 0700}, {"NobiGo/profile/", fs.ModeDir | 0700}, {"NobiGo/README.txt", 0600}}, false},
		{"old root", []archiveEntry{{"PlayGo/README.txt", 0600}}, true},
		{"other root", []archiveEntry{{"Other/README.txt", 0600}}, true},
		{"root traversal", []archiveEntry{{"NobiGo/../escape.txt", 0600}}, true},
		{"parent traversal", []archiveEntry{{"NobiGo/../../escape.txt", 0600}}, true},
		{"backslash", []archiveEntry{{`NobiGo\escape.txt`, 0600}}, true},
		{"stream", []archiveEntry{{"NobiGo/README.txt:stream", 0600}}, true},
		{"duplicate", []archiveEntry{{"NobiGo/README.txt", 0600}, {"NobiGo/readme.TXT", 0600}}, true},
		{"profile", []archiveEntry{{"NobiGo/profile/fixture.json", 0600}}, true},
		{"mixed case profile", []archiveEntry{{"NobiGo/PROFILE/fixture.json", 0600}}, true},
		{"external DLL", []archiveEntry{{"NobiGo/library.DLL", 0600}}, true},
		{"symlink", []archiveEntry{{"NobiGo/link", fs.ModeSymlink | 0700}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			err := extract(testArchive(t, tc.entries), root)
			if (err != nil) != tc.wantErr {
				t.Fatalf("extract error = %v, want error %v", err, tc.wantErr)
			}
			if !tc.wantErr {
				data, err := os.ReadFile(filepath.Join(root, "NobiGo", "README.txt"))
				if err != nil || string(data) != "fixture" {
					t.Fatalf("extracted content differs: %v", err)
				}
			}
		})
	}
}

// TestVerifyContentsRequiresNobiGo は旧名のEXEで必須ファイル検査を通過できないことを確認する。
func TestVerifyContentsRequiresNobiGo(t *testing.T) {
	for _, exe := range []string{"NobiGo.exe", "PlayGo.exe"} {
		t.Run(exe, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "profile"), 0700); err != nil {
				t.Fatal(err)
			}
			hashes := make(map[string]string)
			for _, name := range []string{exe, "README.txt", "LICENSE.txt", "engine/analysis.cfg", "engine/build-provenance.json", "engine/runtime/katago.exe", "engine/runtime/model.bin.gz", "licenses/THIRD_PARTY.md", "licenses/Eigen-3.4.0-source.zip"} {
				path := filepath.Join(root, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
				hash, err := fileHash(path)
				if err != nil {
					t.Fatal(err)
				}
				hashes[name] = hash
			}
			data, err := json.Marshal(hashes)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(root, "checksums.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			err = verifyContents(root)
			if (err != nil) != (exe != "NobiGo.exe") {
				t.Fatalf("verify error = %v for %s", err, exe)
			}
		})
	}
}
