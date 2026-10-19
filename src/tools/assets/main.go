// 作成日: 2026-10-18。固定URLとSHA-256で配布資産を準備する。書込み先はプロジェクト内に限定する。

// Assets はsrcから実行し、assets.lock.jsonに固定された資産だけを取得・検証して展開する。
package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type asset struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	SHA256      string `json:"sha256"`
	Destination string `json:"destination"`
	Kind        string `json:"kind"`
}

// seeds は初回記録用の出所。再取得は生成済みlockを使い、変更時はURLとハッシュをレビューする。
func seeds() []asset {
	return []asset{
		{"model.bin.gz", "https://media.katagotraining.org/uploaded/networks/models/kata1/kata1-b18c384nbt-s9996604416-d4316597426.bin.gz", "9d7a6afed8ff5b74894727e156f04f0cd36060a24824892008fbb6e0cba51f1d", "engine/runtime/model.bin.gz", "file"},
		{"NotoSansJP.ttf", "https://raw.githubusercontent.com/google/fonts/47d54ca1ebb14fd864695f1d39dc734f39017c33/ofl/notosansjp/NotoSansJP%5Bwght%5D.ttf", "c2f3b4d463500a2ddcd3849cded1fceeb9fd6d1c32e6cbecd568453ba50fc68f", "internal/gui/assets/NotoSansJP.ttf", "file"},
		{"font-OFL.txt", "https://raw.githubusercontent.com/google/fonts/47d54ca1ebb14fd864695f1d39dc734f39017c33/ofl/notosansjp/OFL.txt", "", "internal/gui/assets/OFL.txt", "file"},
		{"katago-source.zip", "https://codeload.github.com/lightvector/KataGo/zip/refs/tags/v1.17.1", "5d2fe983a81b10d0cc577666bb484b51a7cbb2fc436d45bb42fffb418cd6f531", "licenses/KataGo", "source"},
		{"model-license.html", "https://katagotraining.org/network_license/", "", "licenses/KataGo-model.html", "file"},
		{"eigen-MPL2.txt", "https://gitlab.com/libeigen/eigen/-/raw/3.4.0/COPYING.MPL2", "", "licenses/Eigen-MPL2.txt", "file"},
		{"zlib-license.txt", "https://raw.githubusercontent.com/madler/zlib/v1.3.1/LICENSE", "", "licenses/zlib.txt", "file"},
		{"cmake.zip", "https://github.com/Kitware/CMake/releases/download/v3.31.8/cmake-3.31.8-windows-x86_64.zip", "", "native", "native"},
		{"eigen.zip", "https://gitlab.com/libeigen/eigen/-/archive/3.4.0/eigen-3.4.0.zip", "", "native", "native"},
		{"zlib-source.zip", "https://codeload.github.com/madler/zlib/zip/refs/tags/v1.3.1", "", "native", "native"},
		{"gcc-exception.txt", "https://raw.githubusercontent.com/gcc-mirror/gcc/releases/gcc-16.1.0/COPYING.RUNTIME", "", "licenses/GCC-Runtime-Exception.txt", "file"},
		{"gcc-GPL3.txt", "https://raw.githubusercontent.com/gcc-mirror/gcc/releases/gcc-16.1.0/COPYING3", "", "licenses/GCC-GPL3.txt", "file"},
	}
}

// main は失敗時に非ゼロで終了する。
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run は初回記録と通常の照合を区別する。lockを無断更新しない。
func run() error {
	record := flag.Bool("record", false, "record reviewed sources while preserving existing checksums for unchanged URLs")
	flag.Parse()
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	module, err := os.ReadFile(filepath.Join(root, "go.mod"))
	// WindowsのチェックアウトでCRLFになっても同じモジュールとして扱う。
	if err != nil || !bytes.Contains(bytes.ReplaceAll(module, []byte("\r\n"), []byte("\n")), []byte("module github.com/Tremendous1192/NobiGo\n")) {
		return errors.New("run from NobiGo/src")
	}
	lock := filepath.Join(root, "assets.lock.json")
	list := seeds()
	if *record {
		if data, readErr := os.ReadFile(lock); readErr == nil {
			var previous []asset
			if err = json.Unmarshal(data, &previous); err != nil {
				return err
			}
			for i := range list {
				for _, old := range previous {
					if list[i].URL == old.URL {
						list[i].SHA256 = old.SHA256
						break
					}
				}
			}
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return readErr
		}
	} else {
		f, err := os.Open(lock)
		if err != nil {
			return err
		}
		dec := json.NewDecoder(io.LimitReader(f, 1<<20))
		dec.DisallowUnknownFields()
		err = dec.Decode(&list)
		if err == nil {
			var tail json.RawMessage
			if tailErr := dec.Decode(&tail); !errors.Is(tailErr, io.EOF) {
				err = errors.New("asset lock has trailing data")
			}
		}
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			return errors.Join(err, closeErr)
		}
	}
	if len(list) == 0 || len(list) > 64 {
		return errors.New("invalid asset count")
	}
	for i := range list {
		a := &list[i]
		if !filepath.IsLocal(a.Name) || a.Name == "." || filepath.Base(a.Name) != a.Name || strings.ContainsAny(a.Name, "/\\:") || !strings.HasPrefix(a.URL, "https://") {
			return errors.New("invalid asset source")
		}
		if !*record && len(a.SHA256) != 64 {
			return errors.New("missing asset hash")
		}
		dest, err := contained(root, a.Destination)
		if err != nil {
			return err
		}
		if a.Kind == "native" {
			dest, err = contained(filepath.Join(root, "..", ".cache"), a.Destination)
			if err != nil {
				return err
			}
		}
		cache := filepath.Join(root, "..", ".cache", "downloads", a.Name)
		if a.Kind == "file" && a.SHA256 != "" {
			if hash, err := hashFile(dest); err == nil && hash == a.SHA256 {
				fmt.Println("verified", a.Destination)
				continue
			}
		}
		if _, err = os.Stat(cache); errors.Is(err, os.ErrNotExist) {
			if err = download(a.URL, cache); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		hash, err := hashFile(cache)
		if err != nil {
			return err
		}
		if a.SHA256 != "" && hash != a.SHA256 {
			return fmt.Errorf("asset checksum mismatch: %s", a.Name)
		}
		a.SHA256 = hash
		switch a.Kind {
		case "file":
			if err = copyFile(cache, dest); err != nil {
				return err
			}
		case "source", "native":
			if err = extract(cache, dest, a.Kind); err != nil {
				return err
			}
			if a.Kind == "source" {
				if err = extract(cache, filepath.Join(root, "..", ".cache", "native"), "native"); err != nil {
					return err
				}
			}
		default:
			return errors.New("unknown asset kind")
		}
		fmt.Println("verified", a.Destination)
	}
	if *record {
		data, err := json.MarshalIndent(list, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(lock, append(data, '\n'), 0600)
	}
	return nil
}

// contained は相対名のディレクトリ外参照を拒否する。
func contained(root, name string) (string, error) {
	if !filepath.IsLocal(name) || strings.Contains(name, ":") {
		return "", errors.New("asset path escapes root")
	}
	return filepath.Join(root, filepath.FromSlash(name)), nil
}

// hashFile は大きいモデルをストリームで照合する。
func hashFile(path string) (hash string, result error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { result = errors.Join(result, f.Close()) }()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// download は最大256MiB・3分で取得し、一時ファイルから確定する。
func download(url, path string) (result error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if !strings.HasPrefix(url, "https://") {
		return errors.New("download requires https")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".download-*")
	if err != nil {
		return err
	}
	name := f.Name()
	closed := false
	defer func() {
		if !closed {
			result = errors.Join(result, f.Close())
		}
		if err := os.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, err)
		}
	}()
	err = f.Close()
	closed = true
	if err != nil {
		return err
	}
	// 既定GoのHTTP処理に既知の脆弱性があるため、Windowsの更新対象であるcurlを使う。
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "curl.exe", "--silent", "--show-error", "--fail", "--location", "--proto", "=https", "--proto-redir", "=https", "--max-time", "180", "--max-filesize", "268435456", "--output", name, "--write-out", "%{content_type}", url)
	output := &limitedOutput{}
	command.Stdout = output
	command.Stderr = os.Stderr
	if err = command.Run(); err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	if strings.TrimSpace(output.String()) == "" {
		return errors.New("download has no content type")
	}
	f, err = os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	closed = false
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() > 256<<20 {
		return errors.New("download exceeds size limit")
	}
	if err = f.Sync(); err != nil {
		return err
	}
	err = f.Close()
	closed = true
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}

// limitedOutput はcurlの応答メタデータを1KiBまでに制限する。
type limitedOutput struct{ data []byte }

// String はサイズ制限済みのメタデータを返す。
func (b *limitedOutput) String() string { return string(b.data) }

// Write は上限超過時に取得を失敗させる。
func (b *limitedOutput) Write(p []byte) (int, error) {
	if len(b.data)+len(p) > 1024 {
		return 0, errors.New("download metadata exceeds limit")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

// copyFile は資産を順番に書き出し、クローズ失敗も呼出し元へ返す。
func copyFile(source, dest string) (result error) {
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, in.Close()) }()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return errors.Join(err, out.Close())
}

// extract はアーカイブ内の許可したファイルだけを展開する。任意パスやシンボリックリンクを拒否する。
func extract(path, dest, kind string) (result error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, z.Close()) }()
	if len(z.File) > 10000 {
		return errors.New("too many archive entries")
	}
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := f.Name
		headerOnly := false
		if kind == "source" {
			name = strings.TrimPrefix(name, "KataGo-1.17.1/")
			allowed := name == "LICENSE" || name == "CONTRIBUTORS"
			for _, p := range []string{"cpp/external/filesystem-1.5.8/", "cpp/external/half-2.2.0/", "cpp/external/httplib/", "cpp/external/tclap-1.2.5/"} {
				if strings.HasPrefix(name, p) && (strings.HasPrefix(filepath.Base(name), "LICENSE") || filepath.Base(name) == "COPYING") {
					allowed = true
				}
			}
			if name == "cpp/core/sha2.cpp" || name == "cpp/external/nlohmann_json/json.hpp" {
				allowed = true
				headerOnly = true
				name += ".license.txt"
			}
			if !allowed {
				continue
			}
		}
		if !f.Mode().IsRegular() || f.UncompressedSize64 > 64<<20 {
			return errors.New("invalid archive entry")
		}
		target, err := contained(dest, name)
		if err != nil {
			return err
		}
		reader, err := f.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(reader, (64<<20)+1))
		closeErr := reader.Close()
		if err != nil || closeErr != nil {
			return errors.Join(err, closeErr)
		}
		if headerOnly {
			start := bytes.Index(data, []byte("/*"))
			if start < 0 {
				return errors.New("license header not found")
			}
			end := bytes.Index(data[start:], []byte("*/"))
			if end < 0 || end > 16384 {
				return errors.New("license header not found")
			}
			data = data[start : start+end+2]
		}
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		if err = os.WriteFile(target, data, 0600); err != nil {
			return err
		}
	}
	return nil
}
