// 作成日: 2026-10-18。生成ZIPを新しい一時フォルダーへ展開し、ハッシュ・画面・実CPU着手を確認する。

// Verify はsrcから実行し、配布物の受入測定をcodex/acceptance-v0.3.0.jsonへ記録する。実プレイヤー情報は扱わない。
package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// main は検査失敗を非ゼロ終了へ変換する。
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run は自分で作った展開先だけを所有し、成功・失敗とも最後に回収する。
func run() (result error) {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil || !strings.Contains(string(mod), "module github.com/Tremendous1192/PlayGo\n") {
		return errors.New("run from PlayGo/src")
	}
	work := filepath.Join(root, "..", ".work")
	if err = os.MkdirAll(work, 0700); err != nil {
		return err
	}
	temp, err := os.MkdirTemp(work, "package-verify-*")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, os.RemoveAll(temp)) }()
	archive := filepath.Join(root, "..", "dist", "PlayGo-v0.3.0-windows-amd64.zip")
	want, err := os.ReadFile(archive + ".sha256")
	if err != nil {
		return err
	}
	hash, err := fileHash(archive)
	if err != nil {
		return err
	}
	if fields := strings.Fields(string(want)); len(fields) != 2 || fields[0] != hash || fields[1] != filepath.Base(archive) {
		return errors.New("ZIP checksum mismatch")
	}
	if err = extract(archive, temp); err != nil {
		return err
	}
	appDir := filepath.Join(temp, "PlayGo")
	if err = verifyContents(appDir); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	exe := filepath.Join(appDir, "PlayGo.exe")
	for _, mode := range []string{"startup", "game", "game-wait", "game-pass", "scoring", "result", "choices", "review", "review-player", "review-19", "review-detail", "review-detail-cpu", "review-detail-combined", "review-detail-wait", "review-detail-error", "review-detail-long", "review-detail-bottom", "review-detail-min"} {
		cmd := exec.CommandContext(ctx, exe, "-preview", mode, "-screenshot", filepath.Join(work, "package-v0.3.0-"+mode+".png"))
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err = cmd.Run(); err != nil {
			return fmt.Errorf("package screen %s: %w", mode, err)
		}
	}
	cmd := exec.CommandContext(ctx, exe, "-verify", filepath.Join(root, "..", "codex", "acceptance-v0.3.0.json"))
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err = cmd.Run(); err != nil {
		return fmt.Errorf("package acceptance: %w", err)
	}
	if err = verifyContents(appDir); err != nil {
		return err
	}
	fmt.Println("package verification passed; source ZIP and extracted files remained unchanged")
	return nil
}

// fileHash は容量の大きいモデルを読み流して照合する。
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

// extract はPlayGo配下の通常ファイルだけを展開し、個人データと外部DLLを拒否する。
func extract(archive, root string) (result error) {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, z.Close()) }()
	if len(z.File) > 2000 {
		return errors.New("too many package entries")
	}
	seen := make(map[string]bool)
	for _, f := range z.File {
		name := strings.TrimSuffix(f.Name, "/")
		key := strings.ToLower(name)
		if seen[key] || !filepath.IsLocal(name) || strings.ContainsAny(name, ":\\") || name != "PlayGo" && !strings.HasPrefix(name, "PlayGo/") {
			return errors.New("invalid package path")
		}
		seen[key] = true
		target := filepath.Join(root, filepath.FromSlash(name))
		if f.FileInfo().IsDir() {
			if err = os.MkdirAll(target, 0700); err != nil {
				return err
			}
			continue
		}
		if !f.Mode().IsRegular() || f.UncompressedSize64 > 256<<20 || strings.HasPrefix(key, "playgo/profile/") || strings.HasSuffix(key, ".dll") {
			return errors.New("unexpected package content")
		}
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		if err = extractFile(f, target); err != nil {
			return err
		}
	}
	return nil
}

// extractFile は読込み・書込み両方の終了失敗も報告する。
func extractFile(f *zip.File, target string) (result error) {
	in, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, in.Close()) }()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, io.LimitReader(in, (256<<20)+1))
	closeErr := out.Close()
	if n > 256<<20 {
		return errors.Join(errors.New("package entry exceeds limit"), err, closeErr)
	}
	return errors.Join(err, closeErr)
}

// verifyContents は全ファイルの照合と空のprofile、必須配布物の存在を検証する。
func verifyContents(root string) error {
	data, err := os.ReadFile(filepath.Join(root, "checksums.json"))
	if err != nil {
		return err
	}
	var hashes map[string]string
	if err = json.Unmarshal(data, &hashes); err != nil {
		return err
	}
	for _, required := range []string{"PlayGo.exe", "README.txt", "LICENSE.txt", "engine/analysis.cfg", "engine/build-provenance.json", "engine/runtime/katago.exe", "engine/runtime/model.bin.gz", "licenses/THIRD_PARTY.md", "licenses/Eigen-3.4.0-source.zip"} {
		if len(hashes[required]) != 64 {
			return fmt.Errorf("missing required package file: %s", required)
		}
	}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		if name == "checksums.json" {
			return nil
		}
		hash, err := fileHash(path)
		if err != nil {
			return err
		}
		if hash != hashes[name] {
			return fmt.Errorf("package file mismatch: %s", name)
		}
		delete(hashes, name)
		return nil
	})
	if err != nil {
		return err
	}
	if len(hashes) != 0 {
		return errors.New("checksummed files are missing")
	}
	entries, err := os.ReadDir(filepath.Join(root, "profile"))
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("package profile is not empty")
	}
	return nil
}
