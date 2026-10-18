// 作成日: 2026-10-18。KataGoのJSON解析プロセスを所有する。Closeで通信goroutineと子プロセスを回収する。

// Package engine はCPU版KataGoとの期限付き通信を提供する。
package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const columns = "ABCDEFGHJKLMNOPQRST"
const MaxResponseBytes = 8 << 20

type Ply struct {
	Color string
	Point int
}
type Position struct {
	Size  int
	Rules string
	Komi  float64
	Moves []Ply
}
type Candidate struct {
	Point     int
	Visits    int
	Winrate   float64
	ScoreLead float64
	Prior     float64
}

// Analysis は黒視点の評価。配列の所有権は呼出し元へ移る。
type Analysis struct {
	ScoreLead float64
	Winrate   float64
	Ownership []float64
	Policy    []float64
	Moves     []Candidate
	Partial   bool
}
type request struct {
	ID          string      `json:"id"`
	Action      string      `json:"action,omitempty"`
	TerminateID string      `json:"terminateId,omitempty"`
	Rules       string      `json:"rules,omitempty"`
	Komi        float64     `json:"komi"`
	SizeX       int         `json:"boardXSize,omitempty"`
	SizeY       int         `json:"boardYSize,omitempty"`
	Moves       [][2]string `json:"moves"`
	Visits      int         `json:"maxVisits,omitempty"`
	Ownership   bool        `json:"includeOwnership,omitempty"`
	Policy      bool        `json:"includePolicy,omitempty"`
	Interval    float64     `json:"reportDuringSearchEvery,omitempty"`
}
type moveInfo struct {
	Move      string  `json:"move"`
	Visits    int     `json:"visits"`
	Winrate   float64 `json:"winrate"`
	ScoreLead float64 `json:"scoreLead"`
	Prior     float64 `json:"prior"`
}
type wireResponse struct {
	ID        string            `json:"id"`
	Error     string            `json:"error"`
	During    bool              `json:"isDuringSearch"`
	NoResults bool              `json:"noResults"`
	Models    []json.RawMessage `json:"models"`
	Root      *moveInfo         `json:"rootInfo"`
	Moves     []moveInfo        `json:"moveInfos"`
	Ownership []float64         `json:"ownership"`
	Policy    []float64         `json:"policy"`
}
type packet struct {
	response wireResponse
	err      error
}
type writeRequest struct {
	data []byte
	done chan error
}

// Client は1件ずつ解析する。並行呼出しはcontext付きセマフォで待ち、Closeは冪等。
type Client struct {
	cmd       *exec.Cmd
	input     io.WriteCloser
	output    io.ReadCloser
	responses chan packet
	writes    chan writeRequest
	slot      chan struct{}
	stop      chan struct{}
	readDone  chan struct{}
	writeDone chan struct{}
	exited    chan struct{}
	cancel    context.CancelFunc
	closeOnce sync.Once
	closeErr  error
	nextID    uint64 // slot取得中の呼出しだけが変更する。
}

// Start は親contextの存続期間でプロセスを起動する。呼出し元は成功後にCloseする。
func Start(ctx context.Context, root string) (*Client, error) {
	life, cancel := context.WithCancel(ctx)
	return startRoot(ctx, life, cancel, root)
}

// startRoot は起動期限とプロセス寿命を別々に扱う。
func startRoot(ctx, life context.Context, cancel context.CancelFunc, root string) (*Client, error) {
	base, err := filepath.Abs(root)
	if err != nil {
		cancel()
		return nil, err
	}
	cmd := exec.CommandContext(life, filepath.Join(base, "engine", "runtime", "katago.exe"), "analysis", "-config", filepath.Join(base, "engine", "analysis.cfg"), "-model", filepath.Join(base, "engine", "runtime", "model.bin.gz"))
	cmd.Dir = filepath.Join(base, "engine", "runtime")
	cmd.WaitDelay = time.Second
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return startCommand(ctx, cmd, cancel)
}

// startCommand はプロトコル確認までを起動処理として待つ。
func startCommand(ctx context.Context, cmd *exec.Cmd, cancel context.CancelFunc) (*Client, error) {
	in, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, errors.Join(err, in.Close())
	}
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		cancel()
		return nil, errors.Join(err, in.Close(), out.Close())
	}
	c := &Client{cmd: cmd, input: in, output: out, responses: make(chan packet, 16), writes: make(chan writeRequest, 1), slot: make(chan struct{}, 1), stop: make(chan struct{}), readDone: make(chan struct{}), writeDone: make(chan struct{}), exited: make(chan struct{}), cancel: cancel}
	c.slot <- struct{}{}
	// Clientが3作業を所有する。出力は16件で逆圧をかけ、停止で全待機を解除しCloseが完了を待つ。
	go c.readLoop()
	go c.writeLoop()
	go c.waitProcess()
	startup, stop := context.WithTimeout(ctx, 30*time.Second)
	defer stop()
	if err = c.write(startup, request{ID: "ready", Action: "query_models"}); err != nil {
		return nil, errors.Join(err, c.Close())
	}
	for {
		select {
		case p, ok := <-c.responses:
			if !ok {
				return nil, errors.Join(errors.New("engine closed during startup"), c.Close())
			}
			if p.err != nil {
				return nil, errors.Join(p.err, c.Close())
			}
			if p.response.ID != "ready" {
				continue
			}
			if p.response.Error != "" || len(p.response.Models) == 0 || len(p.response.Models) > 16 {
				return nil, errors.Join(errors.New("engine model readiness failed"), c.Close())
			}
			return c, nil
		case <-startup.Done():
			return nil, errors.Join(fmt.Errorf("start engine: %w", startup.Err()), c.Close())
		}
	}
}

// readLoop は応答サイズを制限する。未知フィールドはKataGoの前方互換性のため受理する。
func (c *Client) readLoop() {
	defer close(c.readDone)
	defer close(c.responses)
	scanner := bufio.NewScanner(c.output)
	scanner.Buffer(make([]byte, 4096), MaxResponseBytes)
	for scanner.Scan() {
		var response wireResponse
		err := json.Unmarshal(scanner.Bytes(), &response)
		if err == nil && len(response.ID) > 128 {
			err = errors.New("engine response id exceeds limit")
		}
		select {
		case c.responses <- packet{response, err}:
		case <-c.stop:
			return
		}
		if err != nil {
			return
		}
	}
	if err := scanner.Err(); err != nil {
		select {
		case c.responses <- packet{err: fmt.Errorf("read engine: %w", err)}:
		case <-c.stop:
		}
	}
}

// writeLoop は標準入力を単独所有し、ackを1件バッファして取消後にも終了できる。
func (c *Client) writeLoop() {
	defer close(c.writeDone)
	for {
		select {
		case job := <-c.writes:
			n, err := c.input.Write(job.data)
			if err == nil && n != len(job.data) {
				err = io.ErrShortWrite
			}
			job.done <- err
		case <-c.stop:
			return
		}
	}
}

// waitProcess は子プロセスの終了を一度だけ回収する。異常終了は通信側がEOFとして報告する。
func (c *Client) waitProcess() {
	// Closeによる強制終了も通常経路なのでExitErrorを重複報告しない。
	_ = c.cmd.Wait()
	close(c.exited)
}

// write は有限サイズの要求を送信し、取消時は待機を解除する。
func (c *Client) write(ctx context.Context, q request) error {
	data, err := json.Marshal(q)
	if err != nil {
		return err
	}
	if len(data) > 512<<10 {
		return errors.New("engine request exceeds limit")
	}
	job := writeRequest{data: append(data, '\n'), done: make(chan error, 1)}
	select {
	case c.writes <- job:
	case <-ctx.Done():
		return ctx.Err()
	case <-c.stop:
		return errors.New("engine is closed")
	case <-c.exited:
		return errors.New("engine exited")
	}
	select {
	case err = <-job.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-c.stop:
		return errors.New("engine is closed")
	case <-c.exited:
		return errors.New("engine exited")
	}
}

// Close はプロセスを停止して全作業の終了を待つ。複数呼出しは同じ結果を返す。
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		close(c.stop)
		c.cancel()
		// パイプの両端を閉じて、停止した子プロセスに対するRead/Writeを解除する。
		inErr := c.input.Close()
		outErr := c.output.Close()
		<-c.writeDone
		<-c.readDone
		<-c.exited
		if errors.Is(inErr, io.ErrClosedPipe) || errors.Is(inErr, os.ErrClosed) {
			inErr = nil
		}
		if errors.Is(outErr, io.ErrClosedPipe) || errors.Is(outErr, os.ErrClosed) {
			outErr = nil
		}
		// os/execが先に閉じたパイプは既に回収済みなのでCloseの失敗として扱わない。
		c.closeErr = errors.Join(inErr, outErr)
	})
	return c.closeErr
}

// Vertex は盤上番号をGTP座標へ変換する。
func Vertex(point, size int) (string, error) {
	if size < 1 || size > 19 || point < -1 || point >= size*size {
		return "", errors.New("invalid vertex")
	}
	if point == -1 {
		return "pass", nil
	}
	return string(columns[point%size]) + strconv.Itoa(size-point/size), nil
}

// Point はGTP座標を盤上番号へ戻す。
func Point(vertex string, size int) (int, error) {
	if vertex == "pass" {
		return -1, nil
	}
	if size < 1 || size > 19 || len(vertex) < 2 || len(vertex) > 3 {
		return 0, errors.New("invalid engine move")
	}
	x := strings.IndexByte(columns, vertex[0])
	y, err := strconv.Atoi(vertex[1:])
	if err != nil || x < 0 || x >= size || y < 1 || y > size {
		return 0, errors.New("engine move outside board")
	}
	return (size-y)*size + x, nil
}

// makeRequest は局面の境界を確認して独立した要求を作る。
func makeRequest(p Position, id string) (request, error) {
	q := request{ID: id, Rules: p.Rules, Komi: p.Komi, SizeX: p.Size, SizeY: p.Size, Moves: make([][2]string, 0, len(p.Moves)), Visits: 64, Ownership: true, Policy: true, Interval: .25}
	if p.Size < 7 || p.Size > 19 || p.Size%2 == 0 || len(p.Moves) > 4096 || !finite(p.Komi, -100, 100) || (p.Rules != "japanese" && p.Rules != "chinese" && p.Rules != "stone-scoring") {
		return q, errors.New("invalid engine position")
	}
	for i, m := range p.Moves {
		expected := "B"
		if i%2 == 1 {
			expected = "W"
		}
		if m.Color != expected {
			return q, errors.New("invalid engine move color")
		}
		v, err := Vertex(m.Point, p.Size)
		if err != nil {
			return q, err
		}
		q.Moves = append(q.Moves, [2]string{m.Color, v})
	}
	return q, nil
}

// finite は受信数値の範囲を検査する。
func finite(n, low, high float64) bool {
	return !math.IsNaN(n) && !math.IsInf(n, 0) && n >= low && n <= high
}

// decodeAnalysis は欠損・配列長・数値・座標を検査する。
func decodeAnalysis(r wireResponse, size int) (Analysis, error) {
	a := Analysis{Ownership: r.Ownership, Policy: r.Policy, Partial: r.During}
	if r.Error != "" {
		return a, fmt.Errorf("engine rejected query: %.256s", r.Error)
	}
	if r.Root == nil || len(r.Moves) == 0 || len(r.Moves) > size*size+1 || len(r.Ownership) != size*size || len(r.Policy) != size*size+1 {
		return a, errors.New("incomplete engine analysis")
	}
	if !finite(r.Root.Winrate, 0, 1) || !finite(r.Root.ScoreLead, -20000, 20000) {
		return a, errors.New("invalid root evaluation")
	}
	a.ScoreLead = r.Root.ScoreLead
	a.Winrate = r.Root.Winrate
	for _, v := range a.Ownership {
		if !finite(v, -1, 1) {
			return a, errors.New("invalid ownership")
		}
	}
	for _, v := range a.Policy {
		if !finite(v, -1, 1) {
			return a, errors.New("invalid policy")
		}
	}
	for _, m := range r.Moves {
		point, err := Point(m.Move, size)
		if err != nil {
			return a, err
		}
		if m.Visits < 0 || !finite(m.Winrate, 0, 1) || !finite(m.ScoreLead, -20000, 20000) || !finite(m.Prior, 0, 1) {
			return a, errors.New("invalid move evaluation")
		}
		a.Moves = append(a.Moves, Candidate{point, m.Visits, m.Winrate, m.ScoreLead, m.Prior})
	}
	return a, nil
}

// cancelQuery は終了通知まで待つ。応答不能ならプロセスを回収し再起動を呼出し元に委ねる。
func (c *Client) cancelQuery(id string) {
	// CPUで実行中の推論バッチは即座には中断できないため、通常の取消を故障扱いしない猶予を置く。
	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()
	if err := c.write(ctx, request{ID: "cancel", Action: "terminate", TerminateID: id}); err != nil {
		_ = c.Close()
		return
	}
	for {
		select {
		case p, ok := <-c.responses:
			if !ok || p.err != nil {
				_ = c.Close()
				return
			}
			if p.response.ID == id && !p.response.During {
				return
			}
		case <-ctx.Done():
			_ = c.Close()
			return
		case <-c.stop:
			return
		}
	}
}

// Analyze は取消可能な直列解析を行う。期限時に有効な途中結果があれば返すが、通常の取消では返さない。
func (c *Client) Analyze(ctx context.Context, p Position) (Analysis, error) {
	return c.AnalyzeProgress(ctx, p, nil)
}

// AnalyzeProgress は有効な途中解析を同じgoroutineで通知する。notifyはnil可、待機や配列の変更は禁止。
// 通知後も探索を続け、戻り値だけを着手確定に使う。通知配列は以後変更せず、読取り専用として転送できる。
func (c *Client) AnalyzeProgress(ctx context.Context, p Position, notify func(Analysis)) (Analysis, error) {
	ctx, cancel := context.WithTimeout(ctx, 3500*time.Millisecond)
	defer cancel()
	select {
	case <-c.slot:
	case <-ctx.Done():
		return Analysis{}, ctx.Err()
	case <-c.stop:
		return Analysis{}, errors.New("engine is closed")
	}
	defer func() { c.slot <- struct{}{} }()
	c.nextID++
	id := strconv.FormatUint(c.nextID, 10)
	q, err := makeRequest(p, id)
	if err != nil {
		return Analysis{}, err
	}
	if err = c.write(ctx, q); err != nil {
		c.cancelQuery(id)
		return Analysis{}, err
	}
	var latest *Analysis
	for {
		select {
		case packet, ok := <-c.responses:
			if !ok {
				return Analysis{}, errors.New("engine closed output")
			}
			if packet.err != nil {
				return Analysis{}, packet.err
			}
			r := packet.response
			if r.ID != id {
				continue
			}
			a, err := decodeAnalysis(r, p.Size)
			if err != nil {
				return Analysis{}, err
			}
			if !r.During {
				return a, nil
			}
			latest = &a
			if notify != nil {
				notify(a)
			}
		case <-ctx.Done():
			c.cancelQuery(id)
			if errors.Is(ctx.Err(), context.DeadlineExceeded) && latest != nil {
				return *latest, nil
			}
			return Analysis{}, ctx.Err()
		case <-c.stop:
			return Analysis{}, errors.New("engine is closed")
		}
	}
}
