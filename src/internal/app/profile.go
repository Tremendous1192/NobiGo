// 作成日: 2026-10-18。プロフィールをID別JSONへ保存する。表示名はパスやログへ使用しない。
package app

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const ProfileVersion = 1
const MaxResults = 1000
const maxProfileBytes = 4 << 20

var ErrStoreBusy = errors.New("profile store is locked")

type Settings struct {
	Rule       Rule `json:"rule"`
	Size       int  `json:"size"`
	Difficulty int  `json:"difficulty"`
	Side       int  `json:"side"` // 0:先攻、1:後攻、2:ランダム。
}
type Adjustment struct {
	Rule       Rule    `json:"rule"`
	Size       int     `json:"size"`
	Difficulty int     `json:"difficulty"`
	Offset     float64 `json:"offset"`
}
type GameResult struct {
	ID         string   `json:"id"`
	Settings   Settings `json:"settings"`
	Player     Color    `json:"player"`
	BlackScore float64  `json:"blackScore"`
	WhiteScore float64  `json:"whiteScore"`
	NoResult   bool     `json:"noResult"`
	MoveCount  int      `json:"moveCount"`
	UndoCount  int      `json:"undoCount"`
	MeanLoss   float64  `json:"meanLoss"`
	FinishedAt string   `json:"finishedAt"`
}
type Profile struct {
	Version     int          `json:"version"`
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Revision    uint64       `json:"revision"`
	Ranks       [3]float64   `json:"ranks"`
	Last        Settings     `json:"last"`
	GamesPlayed int          `json:"gamesPlayed"`
	Wins        int          `json:"wins"`
	Draws       int          `json:"draws"`
	Results     []GameResult `json:"results"`
	Adjustments []Adjustment `json:"adjustments"`
}

// DefaultSettings は初心者用の初期設定を返す。
func DefaultSettings() Settings { return Settings{Rule: Japanese, Size: 9, Difficulty: 1, Side: 2} }

// Validate は設定の範囲を確認する。
func (s Settings) Validate() error {
	if !ValidRule(s.Rule) || !ValidSize(s.Size) || s.Difficulty < 0 || s.Difficulty > 4 || s.Side < 0 || s.Side > 2 {
		return errors.New("invalid game settings")
	}
	return nil
}

// NewID はファイル名に使用できるランダムIDを生成する。
func NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// validID はパス要素を含まない小文字16進IDだけを認める。
func validID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, r := range id {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// validateName は表示名の長さと制御文字を検査する。
func validateName(name string) error {
	if !utf8.ValidString(name) || strings.TrimSpace(name) != name || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 32 {
		return errors.New("name must contain 1 to 32 characters without surrounding spaces")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return errors.New("name contains control characters")
		}
	}
	return nil
}

// finiteRange は有限の範囲内数値だけを認める。
func finiteRange(n, low, high float64) bool {
	return !math.IsNaN(n) && !math.IsInf(n, 0) && n >= low && n <= high
}

// Validate は保存形式の境界を検査する。resultsとadjustmentsのnull・省略は空履歴として扱う。
func (p Profile) Validate() error {
	if p.Version != ProfileVersion || !validID(p.ID) || p.Revision < 1 {
		return errors.New("unsupported profile version or invalid id/revision")
	}
	if err := validateName(p.Name); err != nil {
		return err
	}
	if err := p.Last.Validate(); err != nil {
		return err
	}
	for _, rank := range p.Ranks {
		if !finiteRange(rank, 0, 38) {
			return errors.New("invalid rank")
		}
	}
	if p.GamesPlayed < 0 || p.Wins < 0 || p.Draws < 0 || p.Wins+p.Draws > p.GamesPlayed || len(p.Results) > MaxResults || len(p.Adjustments) > 90 {
		return errors.New("invalid profile counters")
	}
	ids := make(map[string]bool)
	for _, g := range p.Results {
		if !validID(g.ID) || ids[g.ID] || g.Settings.Validate() != nil || (g.Player != Black && g.Player != White) || !finiteRange(g.BlackScore, 0, 20000) || !finiteRange(g.WhiteScore, 0, 20000) || !finiteRange(g.MeanLoss, 0, 20000) || g.MoveCount < 0 || g.MoveCount > MaxMoves || g.UndoCount < 0 {
			return errors.New("invalid result")
		}
		ids[g.ID] = true
	}
	settings := make(map[Settings]bool)
	for _, a := range p.Adjustments {
		s := Settings{Rule: a.Rule, Size: a.Size, Difficulty: a.Difficulty}
		if s.Validate() != nil || !finiteRange(a.Offset, -1, 1) || settings[s] {
			return errors.New("invalid adjustment")
		}
		settings[s] = true
	}
	return nil
}

// Store はプロフィールディレクトリを所有する。更新は短時間の排他ロックで直列化する。
type Store struct{ Dir string }

// OpenStore は保存ディレクトリを作成する。書込み不可なら起動側へエラーを返す。
func OpenStore(dir string) (*Store, error) {
	full, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(full, 0700); err != nil {
		return nil, fmt.Errorf("create profile directory: %w", err)
	}
	return &Store{Dir: full}, nil
}

// path は検証済みIDだけをディレクトリ内のパスへ変換する。
func (s *Store) path(id, ext string) (string, error) {
	if !validID(id) {
		return "", errors.New("invalid profile id")
	}
	return filepath.Join(s.Dir, id+ext), nil
}

// decodeProfile は上限サイズ・未知フィールド・末尾データ・ID不一致を検査する。
func decodeProfile(data []byte, id string) (Profile, error) {
	var p Profile
	if len(data) > maxProfileBytes {
		return p, errors.New("profile exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		return p, fmt.Errorf("decode profile: %w", err)
	}
	var shape struct {
		Ranks []float64 `json:"ranks"`
	}
	if err := json.Unmarshal(data, &shape); err != nil || len(shape.Ranks) != 3 {
		return p, errors.New("expected three rule ranks")
	}
	var tail json.RawMessage
	if err := decoder.Decode(&tail); err != io.EOF {
		return p, errors.New("trailing profile data")
	}
	if p.ID != id {
		return p, errors.New("profile id does not match file")
	}
	return p, p.Validate()
}

// readBounded は巨大ファイルを全読込みせず拒否する。
func readBounded(path string) (data []byte, result error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("profile must be a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { result = errors.Join(result, f.Close()) }()
	data, err = io.ReadAll(io.LimitReader(f, maxProfileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxProfileBytes {
		return nil, errors.New("profile exceeds size limit")
	}
	return data, nil
}

// Load は指定プロフィールを所有コピーとして返す。
func (s *Store) Load(id string) (Profile, error) {
	path, err := s.path(id, ".json")
	if err != nil {
		return Profile{}, err
	}
	data, err := readBounded(path)
	if err != nil {
		return Profile{}, err
	}
	return decodeProfile(data, id)
}

// List は有効なプロフィールと、読めないIDだけを返す。壊れたファイルを無断で上書きしない。
func (s *Store) List() ([]Profile, []string, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, nil, err
	}
	var profiles []Profile
	var broken []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !validID(id) {
			continue
		}
		p, err := s.Load(id)
		if err != nil {
			broken = append(broken, id)
		} else {
			profiles = append(profiles, p)
		}
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	return profiles, broken, nil
}

// locked は取得したロックだけを解放する。既存ロックは他の所有者のものとして保持する。
func (s *Store) locked(action func() error) (result error) {
	path := filepath.Join(s.Dir, "store.lock")
	lock, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return ErrStoreBusy
	}
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, lock.Close(), os.Remove(path)) }()
	return action()
}

// atomicWrite は同じディレクトリに書込み・同期・クローズしてから置換する。
func atomicWrite(path string, data []byte, replace func(string, string) error) (result error) {
	temp, err := os.CreateTemp(filepath.Dir(path), ".profile-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	closed := false
	defer func() {
		if !closed {
			result = errors.Join(result, temp.Close())
		}
		if err := os.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, err)
		}
	}()
	if _, err = temp.Write(data); err != nil {
		return err
	}
	if err = temp.Sync(); err != nil {
		return err
	}
	err = temp.Close()
	closed = true
	if err != nil {
		return err
	}
	return replace(name, path)
}

// write は上書き前の内容をバックアップする。呼出し元がロックを保持する。
func (s *Store) write(p Profile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	path, err := s.path(p.ID, ".json")
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > maxProfileBytes {
		return errors.New("profile exceeds size limit")
	}
	old, err := readBounded(path)
	if err == nil {
		backup, _ := s.path(p.ID, ".bak")
		if err = atomicWrite(backup, old, os.Rename); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return atomicWrite(path, data, os.Rename)
}

// uniqueName は他のプロフィールとの表示名重複を拒否する。
func (s *Store) uniqueName(name, id string) error {
	list, _, err := s.List()
	if err != nil {
		return err
	}
	for _, p := range list {
		if p.ID != id && strings.EqualFold(p.Name, name) {
			return errors.New("player name already exists")
		}
	}
	return validateName(name)
}

// Create は表示名から新規プロフィールを作る。
func (s *Store) Create(name string) (Profile, error) {
	id, err := NewID()
	if err != nil {
		return Profile{}, err
	}
	p := Profile{Version: ProfileVersion, ID: id, Name: name, Revision: 1, Last: DefaultSettings(), Results: []GameResult{}, Adjustments: []Adjustment{}}
	err = s.locked(func() error {
		if err := s.uniqueName(name, id); err != nil {
			return err
		}
		return s.write(p)
	})
	return p, err
}

// Save は古いRevisionによる上書きを拒否する。成功時だけ呼出し元のRevisionを進める。
func (s *Store) Save(p *Profile) error {
	if p == nil {
		return errors.New("nil profile")
	}
	copy := *p
	err := s.locked(func() error {
		current, err := s.Load(copy.ID)
		if err != nil {
			return err
		}
		if current.Revision != copy.Revision {
			return errors.New("profile changed in another session")
		}
		if copy.Revision == math.MaxUint64 {
			return errors.New("profile revision exhausted")
		}
		if err = s.uniqueName(copy.Name, copy.ID); err != nil {
			return err
		}
		copy.Revision++
		return s.write(copy)
	})
	if err == nil {
		p.Revision = copy.Revision
	}
	return err
}

// Delete はプロフィールを復旧可能な.deletedへ退避する。確認ダイアログ後だけ呼び出す。
func (s *Store) Delete(id string) error {
	return s.locked(func() error {
		source, err := s.path(id, ".json")
		if err != nil {
			return err
		}
		target, _ := s.path(id, ".deleted")
		if _, err = os.Stat(target); err == nil {
			return errors.New("deleted backup already exists")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return os.Rename(source, target)
	})
}

// Restore は検証済みバックアップで復旧し、現在の内容を.recoveryへ保存する。
func (s *Store) Restore(id string) error {
	return s.locked(func() error {
		backup, err := s.path(id, ".bak")
		if err != nil {
			return err
		}
		data, err := readBounded(backup)
		if err != nil {
			return err
		}
		if _, err = decodeProfile(data, id); err != nil {
			return err
		}
		path, _ := s.path(id, ".json")
		old, err := readBounded(path)
		if err == nil {
			recovery, _ := s.path(id, ".recovery")
			// 以前の破損内容も調査・手動復旧に必要なので、再度の復旧で上書きしない。
			if _, statErr := os.Stat(recovery); statErr == nil {
				return errors.New("recovery backup already exists")
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return statErr
			}
			if err = atomicWrite(recovery, old, os.Rename); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return atomicWrite(path, data, os.Rename)
	})
}
