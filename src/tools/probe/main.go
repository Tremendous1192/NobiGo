// 作成日: 2026-10-18。同梱CPUエンジンの初期化と盤サイズ別応答時間を測る。

// Probe はローカルエンジンだけを起動し、測定結果をJSON行で標準出力へ返す。
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type query struct {
	ID        string      `json:"id"`
	Action    string      `json:"action,omitempty"`
	Rules     string      `json:"rules,omitempty"`
	Komi      float64     `json:"komi"`
	SizeX     int         `json:"boardXSize,omitempty"`
	SizeY     int         `json:"boardYSize,omitempty"`
	Moves     [][2]string `json:"moves"`
	Visits    int         `json:"maxVisits,omitempty"`
	Ownership bool        `json:"includeOwnership,omitempty"`
	Policy    bool        `json:"includePolicy,omitempty"`
}
type response struct {
	ID    string `json:"id"`
	Error string `json:"error"`
	Root  struct {
		Visits int `json:"visits"`
	} `json:"rootInfo"`
	Moves []struct {
		Move string `json:"move"`
	} `json:"moveInfos"`
	Models []json.RawMessage `json:"models"`
}
type measurement struct {
	Kind         string `json:"kind"`
	Size         int    `json:"size,omitempty"`
	Rules        string `json:"rules,omitempty"`
	Milliseconds int64  `json:"milliseconds"`
	Visits       int    `json:"visits,omitempty"`
}

// main は終了コードを変換する。
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run は全測定の期限を設け、終了時に子プロセスを回収する。
func run() (result error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	executable, err := filepath.Abs("engine/runtime/katago.exe")
	if err != nil {
		return err
	}
	model, err := filepath.Abs("engine/runtime/model.bin.gz")
	if err != nil {
		return err
	}
	if len(os.Args) == 2 {
		model, err = filepath.Abs(os.Args[1])
		if err != nil {
			return err
		}
	}
	config, err := filepath.Abs("engine/analysis.cfg")
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, executable, "analysis", "-model", model, "-config", config)
	cmd.Dir = filepath.Dir(executable)
	cmd.Stderr = os.Stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return errors.Join(err, in.Close())
	}
	start := time.Now()
	if err = cmd.Start(); err != nil {
		return errors.Join(err, in.Close(), out.Close())
	}
	defer func() { result = errors.Join(result, in.Close(), cmd.Wait()) }()
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 4096), 8<<20)
	encoder := json.NewEncoder(in)
	if err = encoder.Encode(query{ID: "ready", Action: "query_models"}); err != nil {
		return err
	}
	if !scanner.Scan() {
		return errors.New("engine ended before ready")
	}
	var ready response
	if err = json.Unmarshal(scanner.Bytes(), &ready); err != nil {
		return err
	}
	if len(ready.Models) == 0 {
		return fmt.Errorf("engine readiness failed: %s", ready.Error)
	}
	output := json.NewEncoder(os.Stdout)
	if err = output.Encode(measurement{Kind: "startup", Milliseconds: time.Since(start).Milliseconds()}); err != nil {
		return err
	}
	for _, size := range []int{7, 9, 11, 13, 15, 19} {
		for _, rule := range []string{"japanese", "chinese", "stone-scoring"} {
			komi := 6.5
			if rule == "chinese" {
				komi = 7.5
			}
			if rule == "stone-scoring" {
				komi = 0
			}
			q := query{ID: fmt.Sprintf("%s-%d", rule, size), Rules: rule, Komi: komi, SizeX: size, SizeY: size, Moves: [][2]string{}, Visits: 64, Ownership: true, Policy: true}
			start = time.Now()
			if err = encoder.Encode(q); err != nil {
				return err
			}
			if !scanner.Scan() {
				return errors.Join(errors.New("engine ended before response"), scanner.Err())
			}
			var r response
			if err = json.Unmarshal(scanner.Bytes(), &r); err != nil {
				return err
			}
			if r.ID != q.ID || r.Error != "" || len(r.Moves) == 0 {
				return fmt.Errorf("invalid response: %s", r.Error)
			}
			elapsed := time.Since(start)
			if err = output.Encode(measurement{Kind: "analysis", Size: size, Rules: rule, Milliseconds: elapsed.Milliseconds(), Visits: r.Root.Visits}); err != nil {
				return err
			}
			if elapsed > 5*time.Second {
				return errors.New("response exceeded five seconds")
			}
		}
	}
	return nil
}
