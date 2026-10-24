// 作成日: 2026-10-18。レビュー済み資産だけを初心者用ZIPへまとめる。プロフィールや開発キャッシュは含めない。

// Package はsrcから実行し、モデルを初回取得するZIPをdistへ出力する。
package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Tremendous1192/NobiGo/engine"
)

// main は配布物の確定後に終了する。
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run は一時ステージを所有し、成功時だけZIPを置き換える。
func run() (result error) {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	mod, err := os.ReadFile("go.mod")
	// WindowsのチェックアウトでCRLFになっても同じモジュールとして扱う。
	if err != nil || !strings.Contains(strings.ReplaceAll(string(mod), "\r\n", "\n"), "module github.com/Tremendous1192/NobiGo\n") {
		return errors.New("run from NobiGo/src")
	}
	if err = verifyAssets(root); err != nil {
		return err
	}
	dist := filepath.Join(root, "..", "dist")
	if err = os.MkdirAll(dist, 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(dist, ".package-*")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, os.RemoveAll(stage)) }()
	appDir := filepath.Join(stage, "NobiGo")
	if err = os.MkdirAll(appDir, 0700); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-buildvcs=false", "-ldflags=-H=windowsgui", "-o", filepath.Join(appDir, "NobiGo.exe"), "./cmd")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		return err
	}
	var provenance struct{ SHA256 string }
	data, err := os.ReadFile(filepath.Join(root, "engine", "build-provenance.json"))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, &provenance); err != nil {
		return err
	}
	binary, err := os.ReadFile(filepath.Join(root, "engine", "runtime", "katago.exe"))
	if err != nil {
		return err
	}
	if provenance.SHA256 != fmt.Sprintf("%x", sha256.Sum256(binary)) {
		return errors.New("engine does not match build provenance; run enginebuild")
	}
	files := map[string]string{
		"README.txt": "README.txt", "../LICENSE": "LICENSE.txt", "assets.lock.json": "assets.lock.json",
		"engine/runtime/katago.exe": "engine/runtime/katago.exe", "engine/analysis.cfg": "engine/analysis.cfg", "engine/build-provenance.json": "engine/build-provenance.json",
		"internal/gui/assets/OFL.txt": "licenses/NotoSansJP-OFL.txt", "licenses/KataGo-model.html": "licenses/KataGo-model.html", "licenses/Eigen-MPL2.txt": "licenses/Eigen-MPL2.txt", "licenses/zlib.txt": "licenses/zlib.txt",
		"licenses/MinGW-runtime.txt": "licenses/MinGW-runtime.txt", "licenses/MinGW.txt": "licenses/MinGW.txt", "licenses/winpthreads.txt": "licenses/winpthreads.txt", "licenses/GCC-GPL3.txt": "licenses/GCC-GPL3.txt", "licenses/GCC-Runtime-Exception.txt": "licenses/GCC-Runtime-Exception.txt",
		"THIRD_PARTY.md": "licenses/THIRD_PARTY.md", "../.cache/downloads/eigen.zip": "licenses/Eigen-3.4.0-source.zip",
	}
	for source, target := range files {
		if err = copyFile(filepath.Join(root, filepath.FromSlash(source)), filepath.Join(appDir, filepath.FromSlash(target))); err != nil {
			return err
		}
	}
	if err = copyTree(filepath.Join(root, "licenses", "KataGo"), filepath.Join(appDir, "licenses", "KataGo")); err != nil {
		return err
	}
	if err = goLicenses(ctx, appDir); err != nil {
		return err
	}
	if err = os.Mkdir(filepath.Join(appDir, "profile"), 0700); err != nil {
		return err
	}
	checksums := map[string]string{}
	if err = filepath.WalkDir(appDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(appDir, path)
		if err != nil {
			return err
		}
		hash, err := fileHash(path)
		if err != nil {
			return err
		}
		checksums[filepath.ToSlash(rel)] = hash
		return nil
	}); err != nil {
		return err
	}
	data, err = json.MarshalIndent(checksums, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(appDir, "checksums.json"), append(data, '\n'), 0600); err != nil {
		return err
	}
	archive := filepath.Join(stage, "NobiGo-v0.7.1-windows-amd64.zip")
	if err = makeZip(appDir, archive); err != nil {
		return err
	}
	info, err := os.Stat(archive)
	if err != nil {
		return err
	}
	if info.Size() >= 100<<20 {
		return errors.New("package exceeds regular Git distribution limit")
	}
	hash, err := fileHash(archive)
	if err != nil {
		return err
	}
	target := filepath.Join(dist, filepath.Base(archive))
	if err = os.Rename(archive, target); err != nil {
		return err
	}
	if err = os.WriteFile(target+".sha256", []byte(hash+"  "+filepath.Base(target)+"\n"), 0600); err != nil {
		return err
	}
	info, err = os.Stat(target)
	if err != nil {
		return err
	}
	fmt.Printf("created %s (%d bytes)\nsha256 %s\n", filepath.Base(target), info.Size(), hash)
	return nil
}

// verifyAssets は取得定義・埋込みフォント・同梱ソースを検証する。モデル本体は配布に不要。
func verifyAssets(root string) error {
	if err := engine.ValidateModelSource(root); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(root, "assets.lock.json"))
	if err != nil {
		return err
	}
	var assets []struct{ Name, SHA256 string }
	if err = json.Unmarshal(data, &assets); err != nil {
		return err
	}
	required := map[string]string{"NotoSansJP.ttf": "internal/gui/assets/NotoSansJP.ttf", "eigen.zip": "../.cache/downloads/eigen.zip"}
	for _, asset := range assets {
		path, ok := required[asset.Name]
		if !ok {
			continue
		}
		hash, err := fileHash(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return err
		}
		if hash != asset.SHA256 {
			return fmt.Errorf("asset checksum mismatch: %s", asset.Name)
		}
		delete(required, asset.Name)
	}
	if len(required) != 0 {
		return errors.New("required package assets are absent from lock")
	}
	return nil
}

// fileHash はファイルを読み流してSHA-256を返す。
func fileHash(path string) (hash string, result error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { result = errors.Join(result, f.Close()) }()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// copyFile は通常ファイルだけをコピーして両方のCloseを確認する。
func copyFile(source, target string) (result error) {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("package input is not a regular file")
	}
	if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, in.Close()) }()
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return errors.Join(err, out.Close())
}

// copyTree はライセンス用ディレクトリだけをたどる。
func copyTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		return copyFile(path, filepath.Join(target, rel))
	})
}

// goLicenses はEXEが使うモジュールの著作権表示とGo本体のLICENSEを同梱する。
func goLicenses(ctx context.Context, dir string) error {
	data, err := exec.CommandContext(ctx, "go", "list", "-deps", "-f={{if .Module}}{{.Module.Path}}{{end}}", "./cmd").Output()
	if err != nil {
		return err
	}
	names := strings.Fields(string(data))
	slices.Sort(names)
	names = slices.Compact(names)
	args := append([]string{"list", "-m", "-json"}, names...)
	data, err = exec.CommandContext(ctx, "go", args...).Output()
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	for decoder.More() {
		var m struct {
			Path, Version, Dir string
			Main               bool
		}
		if err = decoder.Decode(&m); err != nil {
			return err
		}
		if m.Main {
			continue
		}
		if m.Dir == "" {
			return fmt.Errorf("missing module source: %s", m.Path)
		}
		if err = filepath.WalkDir(m.Dir, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			name := strings.ToUpper(d.Name())
			if !(strings.HasPrefix(name, "LICENSE") || strings.HasPrefix(name, "COPYING") || strings.HasPrefix(name, "NOTICE") || strings.HasPrefix(name, "AUTHORS")) {
				return nil
			}
			rel, err := filepath.Rel(m.Dir, path)
			if err != nil {
				return err
			}
			return copyFile(path, filepath.Join(dir, "licenses", "Go-modules", filepath.FromSlash(m.Path+"@"+m.Version), rel))
		}); err != nil {
			return err
		}
	}
	data, err = exec.CommandContext(ctx, "go", "env", "GOROOT").Output()
	if err != nil {
		return err
	}
	return copyFile(filepath.Join(strings.TrimSpace(string(data)), "LICENSE"), filepath.Join(dir, "licenses", "Go.txt"))
}

// makeZip は順序と日時を固定し、生成物だけをNobiGoディレクトリ配下へ格納する。
func makeZip(source, target string) (result error) {
	f, err := os.Create(target)
	if err != nil {
		return err
	}
	z := zip.NewWriter(f)
	defer func() { result = errors.Join(result, z.Close(), f.Close()) }()
	var paths []string
	if err = filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, path)
		return nil
	}); err != nil {
		return err
	}
	slices.Sort(paths)
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		name := "NobiGo/"
		if rel != "." {
			name += filepath.ToSlash(rel)
		}
		if info.IsDir() && !strings.HasSuffix(name, "/") {
			name += "/"
		}
		header := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)}
		if info.IsDir() {
			header.Method = zip.Store
		}
		writer, err := z.CreateHeader(header)
		if err != nil {
			return err
		}
		if info.IsDir() {
			continue
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(writer, in)
		closeErr := in.Close()
		if err != nil || closeErr != nil {
			return errors.Join(err, closeErr)
		}
	}
	return nil
}
