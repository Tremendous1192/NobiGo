// 作成日: 2026-10-19。初回起動専用のモデル取得・照合を行う。対局中のPrepareからは呼ばない。
package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const modelLimit = 256 << 20

var ErrModelInvalid = errors.New("model is invalid; explicit repair required")

// ModelStage は起動時の作業段階。通知は呼出し元の作業goroutine内で行う。
type ModelStage int

const (
	ModelChecking ModelStage = iota
	ModelDownloading
	ModelVerifying
)

type modelAsset struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	SHA256      string `json:"sha256"`
	Destination string `json:"destination"`
	Kind        string `json:"kind"`
}

// readModelAsset は固定書式のlockから唯一のモデルを選び、取得先と保存先を検証する。
func readModelAsset(root string) (modelAsset, error) {
	f, err := os.Open(filepath.Join(root, "assets.lock.json"))
	if err != nil {
		return modelAsset{}, err
	}
	data, readErr := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err = errors.Join(readErr, f.Close()); err != nil {
		return modelAsset{}, err
	}
	if len(data) > 1<<20 {
		return modelAsset{}, errors.New("asset lock exceeds limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var assets []modelAsset
	if err = decoder.Decode(&assets); err != nil {
		return modelAsset{}, err
	}
	var tail json.RawMessage
	if !errors.Is(decoder.Decode(&tail), io.EOF) || len(assets) == 0 || len(assets) > 64 {
		return modelAsset{}, errors.New("invalid asset lock")
	}
	var model modelAsset
	seen := make(map[string]bool)
	for _, a := range assets {
		if a.Name == "" || seen[a.Name] {
			return modelAsset{}, errors.New("invalid asset name")
		}
		seen[a.Name] = true
		if a.Name == "model.bin.gz" {
			model = a
		}
	}
	u, err := url.Parse(model.URL)
	hash, hashErr := hex.DecodeString(model.SHA256)
	if err != nil || u.Scheme != "https" || u.Host != "media.katagotraining.org" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/uploaded/networks/models/") || !strings.HasSuffix(u.Path, ".bin.gz") || hashErr != nil || len(hash) != sha256.Size || model.SHA256 != strings.ToLower(model.SHA256) || model.Destination != "engine/runtime/model.bin.gz" || model.Kind != "file" {
		return modelAsset{}, errors.New("invalid model source")
	}
	return model, nil
}

// ValidateModelSource は配布時に取得定義だけを検証する。通信・書込みは行わない。
func ValidateModelSource(root string) error {
	_, err := readModelAsset(root)
	return err
}

// modelDirectory はモデル保存先にリンクがないことを確認し、必要な場合だけ作成する。
func modelDirectory(root string, create bool) (string, error) {
	dir := root
	for _, name := range []string{"engine", "runtime"} {
		dir = filepath.Join(dir, name)
		info, err := os.Lstat(dir)
		if errors.Is(err, os.ErrNotExist) && create {
			if err = os.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
				return "", err
			}
			info, err = os.Lstat(dir)
		}
		if err != nil {
			return "", err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("model directory must not be a link")
		}
	}
	return dir, nil
}

// modelHash は通常ファイルだけを上限付きで照合する。取消は各読込みの間に確認する。
func modelHash(ctx context.Context, path string) (hash string, result error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("model must be a regular file")
	}
	if info.Size() <= 0 || info.Size() > modelLimit {
		return "", ErrModelInvalid
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { result = errors.Join(result, f.Close()) }()
	h := sha256.New()
	buffer := make([]byte, 64<<10)
	var total int64
	for {
		if err = ctx.Err(); err != nil {
			return "", err
		}
		n, readErr := f.Read(buffer)
		total += int64(n)
		if total > modelLimit {
			return "", errors.New("model exceeds limit")
		}
		if _, err = h.Write(buffer[:n]); err != nil {
			return "", err
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifyModel は保存済みモデルを照合する。欠損・破損時も通信・書込みは行わない。
func VerifyModel(ctx context.Context, root string) error {
	a, err := readModelAsset(root)
	if err != nil {
		return err
	}
	dir, err := modelDirectory(root, false)
	if err != nil {
		return err
	}
	hash, err := modelHash(ctx, filepath.Join(dir, a.Name))
	if err != nil {
		return err
	}
	if hash != a.SHA256 {
		return ErrModelInvalid
	}
	return nil
}

// PrepareModel は起動前だけ呼ぶ。repairは破損ファイルの置換許可。notifyは非待機で実装する。
// 作業は呼出し側が所有し、ctxの取消後に完了を待つ。同じ保存先への取得はロックで排他する。
func PrepareModel(ctx context.Context, root string, repair bool, notify func(ModelStage)) error {
	return prepareModel(ctx, root, repair, notify, fetchModel)
}

// prepareModel は検証済みの一時ファイルだけを確定し、失敗時に既存モデルを保持する。
func prepareModel(ctx context.Context, root string, repair bool, notify func(ModelStage), fetch func(context.Context, string, string) error) (result error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	report := func(stage ModelStage) {
		if notify != nil {
			notify(stage)
		}
	}
	report(ModelChecking)
	a, err := readModelAsset(root)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	dir, err := modelDirectory(root, true)
	if err != nil {
		return err
	}
	target := filepath.Join(dir, a.Name)
	hash, err := modelHash(ctx, target)
	if err == nil && hash == a.SHA256 {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) && !(repair && errors.Is(err, ErrModelInvalid)) {
		return err
	}
	if err == nil && !repair {
		return ErrModelInvalid
	}
	lockPath := filepath.Join(dir, "model.lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("lock model preparation: %w", err)
	}
	defer func() { result = errors.Join(result, lock.Close(), os.Remove(lockPath)) }()
	// 別プロセスが照合とロック取得の間に準備を終える場合がある。
	if hash, err = modelHash(ctx, target); err == nil && hash == a.SHA256 {
		return nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) && !(repair && errors.Is(err, ErrModelInvalid)) {
		return err
	} else if err == nil && !repair {
		return ErrModelInvalid
	}
	f, err := os.CreateTemp(dir, ".model-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer func() {
		if err := os.Remove(temp); err != nil && !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, err)
		}
	}()
	if err = f.Close(); err != nil {
		return err
	}
	report(ModelDownloading)
	if err = fetch(ctx, a.URL, temp); err != nil {
		return fmt.Errorf("download model: %w", err)
	}
	report(ModelVerifying)
	hash, err = modelHash(ctx, temp)
	if err != nil {
		return err
	}
	if hash != a.SHA256 {
		return errors.New("downloaded model checksum mismatch")
	}
	f, err = os.OpenFile(temp, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if err = errors.Join(f.Sync(), f.Close()); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return os.Rename(temp, target)
}

// fetchModel は既存の資産取得と同じWindows curlを使い、HTTPS・3分・256MiBに制限する。
func fetchModel(ctx context.Context, source, target string) error {
	cmd := exec.CommandContext(ctx, "curl.exe", "--disable", "--silent", "--fail", "--location", "--proto", "=https", "--proto-redir", "=https", "--max-time", "180", "--max-filesize", "268435456", "--output", target, "--write-out", "%{http_code}\n%{content_type}", source)
	cmd.WaitDelay = time.Second
	out := &modelMetadata{}
	cmd.Stdout = out
	if err := cmd.Run(); err != nil {
		return errors.Join(err, ctx.Err())
	}
	return validateModelResponse(out.data)
}

// validateModelResponse はWindows curlのCRLFも正規化して状態と内容種別を検査する。
func validateModelResponse(data []byte) error {
	parts := strings.SplitN(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n", 2)
	if len(parts) != 2 || parts[0] != "200" || strings.TrimSpace(parts[1]) == "" || strings.Contains(strings.ToLower(parts[1]), "text/html") {
		return errors.New("invalid model download response")
	}
	return nil
}

type modelMetadata struct{ data []byte }

// Write は外部プロセスの応答メタデータを1KiBに制限する。
func (b *modelMetadata) Write(p []byte) (int, error) {
	if len(b.data)+len(p) > 1024 {
		return 0, errors.New("download metadata exceeds limit")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}
