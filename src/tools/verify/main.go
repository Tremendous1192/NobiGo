// 作成日: 2026-10-18。生成ZIPを新しい一時フォルダーへ展開し、ハッシュ・画面・実CPU着手を確認する。

// Verify はsrcから明示的な-download-model指定で実行し、初回取得と配布物の受入測定を記録する。
package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Tremendous1192/NobiGo/engine"
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
	download := flag.Bool("download-model", false, "allow the packaged application to download its pinned model for first-start verification")
	flag.Parse()
	if !*download {
		return errors.New("first-start verification requires -download-model")
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	// WindowsのチェックアウトでCRLFになっても同じモジュールとして扱う。
	if err != nil || !strings.Contains(strings.ReplaceAll(string(mod), "\r\n", "\n"), "module github.com/Tremendous1192/NobiGo\n") {
		return errors.New("run from NobiGo/src")
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
	archive := filepath.Join(root, "..", "dist", "NobiGo-v0.7.1-windows-amd64.zip")
	hash, err := verifyArchiveHash(archive)
	if err != nil {
		return err
	}
	if err = extract(archive, temp); err != nil {
		return err
	}
	appDir := filepath.Join(temp, "NobiGo")
	if err = verifyContents(appDir, false); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	exe := filepath.Join(appDir, "NobiGo.exe")
	for _, mode := range []string{"startup", "startup-check", "startup-download", "startup-verify", "startup-load", "startup-error", "startup-repair", "startup-cancel", "game", "game-header-7", "game-header-9", "game-header-11", "game-header-13", "game-header-15", "game-header-19", "game-header-chinese", "game-header-jungo", "game-header-long", "game-header-min", "game-wait", "game-pass", "scoring", "result", "choices", "review", "review-player", "review-19", "review-detail", "review-detail-cpu", "review-detail-combined", "review-detail-wait", "review-detail-error", "review-detail-long", "review-detail-bottom", "review-detail-min", "game-white-jungo", "game-extremes", "game-gap", "review-detail-before", "review-detail-outside", "review-detail-pass", "review-detail-empty", "review-detail-7", "review-detail-9", "review-detail-11", "review-detail-13", "review-detail-15", "review-detail-19", "review-focus-moved", "review-focus-before", "review-ai", "review-ai-19", "review-detail-ai", "review-detail-ai-risk",
		"review-detail-rescue", "review-detail-atari", "review-detail-jungo-empty", "review-detail-jungo-pass",
		"summary-1", "summary-2", "summary-3", "summary-4", "summary-wait", "summary-error", "summary-missing", "summary-white", "summary-jungo", "summary-draw", "summary-empty", "summary-noresult", "summary-long", "summary-bottom", "summary-min", "summary-7", "summary-9", "summary-11", "summary-13", "summary-15", "summary-19", "summary-rescue", "summary-compare", "summary-compare-bottom", "summary-combined", "summary-cpu-actions", "summary-jungo-rescue", "summary-white-rescue", "summary-chinese", "summary-pass-candidate"} {
		cmd := exec.CommandContext(ctx, exe, "-preview", mode, "-screenshot", filepath.Join(work, "package-v0.7.1-"+mode+".png"))
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err = cmd.Run(); err != nil {
			return fmt.Errorf("package screen %s: %w", mode, err)
		}
	}
	initial, err := measurePreparation(ctx, exe, filepath.Join(work, "preparation-v0.7.1-initial.json"), false)
	if err != nil {
		return err
	}
	if !initial.Downloaded {
		return errors.New("first setup did not download the model")
	}
	if err = verifyContents(appDir, true); err != nil {
		return err
	}
	cached, err := measurePreparation(ctx, exe, filepath.Join(work, "preparation-v0.7.1-offline.json"), true)
	if err != nil {
		return err
	}
	if cached.Downloaded {
		return errors.New("cached setup attempted model download")
	}
	cmd := exec.CommandContext(ctx, exe, "-verify", filepath.Join(root, "..", "codex", "acceptance-v0.7.1.json"))
	cmd.Env = offlineEnvironment()
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err = cmd.Run(); err != nil {
		return fmt.Errorf("package acceptance: %w", err)
	}
	if err = verifyContents(appDir, true); err != nil {
		return err
	}
	finalHash, err := fileHash(archive)
	if err != nil {
		return err
	}
	if finalHash != hash {
		return errors.New("source ZIP changed during verification")
	}
	data, err := json.MarshalIndent(struct {
		Initial startupMeasurement `json:"initial"`
		Offline startupMeasurement `json:"offline"`
		ZIPHash string             `json:"zipSHA256"`
		Method  string             `json:"method"`
	}{initial, cached, hash, "packaged GUI preparation; cached startup and CPU/review acceptance with empty PATH and unreachable proxies; not a physical network disconnection"}, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(root, "..", "codex", "preparation-v0.7.1.json"), append(data, '\n'), 0600); err != nil {
		return err
	}
	fmt.Println("package verification passed; pinned model installed, packaged files and source ZIP unchanged")
	return nil
}

type startupMeasurement struct {
	ModelMS    int64 `json:"modelPreparationMilliseconds"`
	EngineMS   int64 `json:"engineLoadMilliseconds"`
	TotalMS    int64 `json:"totalMilliseconds"`
	Downloaded bool  `json:"downloaded"`
	Passed     bool  `json:"passed"`
}

// measurePreparation は実EXEの通常準備を測り、プロフィールの読書きなしで終了させる。
func measurePreparation(ctx context.Context, exe, report string, offline bool) (startupMeasurement, error) {
	cmd := exec.CommandContext(ctx, exe, "-prepare", report)
	if offline {
		cmd.Env = offlineEnvironment()
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return startupMeasurement{}, fmt.Errorf("package preparation: %w", err)
	}
	data, err := os.ReadFile(report)
	if err != nil {
		return startupMeasurement{}, err
	}
	var m startupMeasurement
	if err = json.Unmarshal(data, &m); err != nil {
		return m, err
	}
	if !m.Passed || m.ModelMS < 0 || m.EngineMS < 0 || m.TotalMS < 0 {
		return m, errors.New("invalid preparation measurement")
	}
	return m, nil
}

// offlineEnvironment は子EXEだけでcurlの探索とプロキシ通信を遮断する。システム設定は変えない。
func offlineEnvironment() []string {
	env := make([]string, 0, len(os.Environ())+5)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "PATH", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY":
			continue
		}
		env = append(env, entry)
	}
	return append(env, "PATH=", "HTTP_PROXY=http://127.0.0.1:1", "HTTPS_PROXY=http://127.0.0.1:1", "ALL_PROXY=http://127.0.0.1:1", "NO_PROXY=")
}

// verifyArchiveHash はSHAファイルがあれば必ず照合する。Git配布で欠けてもZIP内照合は別途必須。
func verifyArchiveHash(archive string) (string, error) {
	hash, err := fileHash(archive)
	if err != nil {
		return "", err
	}
	want, err := os.ReadFile(archive + ".sha256")
	if errors.Is(err, os.ErrNotExist) {
		return hash, nil
	}
	if err != nil {
		return "", err
	}
	if fields := strings.Fields(string(want)); len(fields) != 2 || fields[0] != hash || fields[1] != filepath.Base(archive) {
		return "", errors.New("ZIP checksum mismatch")
	}
	return hash, nil
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

// extract はNobiGo配下の通常ファイルだけを展開し、個人データと外部DLLを拒否する。
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
		// 接頭辞だけではNobiGo/../...がアプリ用ディレクトリの外へ出る。
		if seen[key] || path.Clean(name) != name || !filepath.IsLocal(name) || strings.ContainsAny(name, ":\\") || name != "NobiGo" && !strings.HasPrefix(name, "NobiGo/") {
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
		if !f.Mode().IsRegular() || f.UncompressedSize64 > 256<<20 || strings.HasPrefix(key, "nobigo/profile/") || strings.HasSuffix(key, ".dll") || key == "nobigo/engine/runtime/model.bin.gz" {
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

// verifyContents は配布物を不変のmanifest、取得後のモデルを資産lockで独立に照合する。
func verifyContents(root string, modelReady bool) error {
	data, err := os.ReadFile(filepath.Join(root, "checksums.json"))
	if err != nil {
		return err
	}
	var hashes map[string]string
	if err = json.Unmarshal(data, &hashes); err != nil {
		return err
	}
	if _, ok := hashes["engine/runtime/model.bin.gz"]; ok {
		return errors.New("model must not be part of package manifest")
	}
	for _, required := range []string{"NobiGo.exe", "README.txt", "LICENSE.txt", "assets.lock.json", "engine/analysis.cfg", "engine/build-provenance.json", "engine/runtime/katago.exe", "licenses/KataGo-model.html", "licenses/THIRD_PARTY.md", "licenses/Eigen-3.4.0-source.zip"} {
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
		if !entry.Type().IsRegular() {
			return errors.New("package file is not regular")
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		if name == "checksums.json" {
			return nil
		}
		if name == "engine/runtime/model.bin.gz" {
			if !modelReady {
				return errors.New("model unexpectedly bundled")
			}
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
	if err = engine.ValidateModelSource(root); err != nil {
		return err
	}
	if modelReady {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return engine.VerifyModel(ctx, root)
	}
	return nil
}
