// 作成日: 2026-10-18。KataGoの公式ソースをCPU専用・静的ランタイムでビルドする。外部ツールのインストールは行わない。

// Enginebuild は準備済み資産と既存MinGWを使い、プロジェクト内にエンジンと出所記録を生成する。
package main

import (
	"context"
	"crypto/sha256"
	"debug/pe"
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

// main は完了してから終了コードを返す。
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run は4並列までのホストビルドを行い、Windows標準DLL以外への依存を拒否する。
func run() error {
	compilerRoot := flag.String("mingw", `C:\mingw64`, "existing MinGW installation (read-only)")
	flag.Parse()
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	mod, err := os.ReadFile("go.mod")
	if err != nil || !strings.Contains(string(mod), "module github.com/Tremendous1192/PlayGo\n") {
		return errors.New("run from PlayGo/src")
	}
	cache := filepath.Join(root, "..", ".cache", "native")
	cmake := filepath.Join(cache, "cmake-3.31.8-windows-x86_64", "bin", "cmake.exe")
	cc := filepath.Join(*compilerRoot, "bin", "gcc.exe")
	cxx := filepath.Join(*compilerRoot, "bin", "g++.exe")
	makeTool := filepath.Join(*compilerRoot, "bin", "mingw32-make.exe")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	version, err := exec.CommandContext(ctx, cxx, "-dumpfullversion").Output()
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(version)) != "16.1.0" {
		return errors.New("expected reviewed MinGW GCC 16.1.0")
	}
	zSource := filepath.Join(cache, "zlib-1.3.1")
	zBuild := filepath.Join(cache, "build-zlib")
	kBuild := filepath.Join(cache, "build-katago")
	common := []string{"-G", "MinGW Makefiles", "-DCMAKE_BUILD_TYPE=Release", "-DCMAKE_C_COMPILER=" + cc, "-DCMAKE_CXX_COMPILER=" + cxx, "-DCMAKE_MAKE_PROGRAM=" + makeTool}
	args := append([]string{"-S", zSource, "-B", zBuild}, common...)
	args = append(args, "-DZLIB_BUILD_EXAMPLES=OFF")
	if err = command(ctx, cache, cmake, args...); err != nil {
		return err
	}
	if err = command(ctx, cache, cmake, "--build", zBuild, "--target", "zlibstatic", "--parallel", "4"); err != nil {
		return err
	}
	args = append([]string{"-S", filepath.Join(cache, "KataGo-1.17.1", "cpp"), "-B", kBuild}, common...)
	// GCC PR54412相当のAVX戻り値の未整列アクセスを再現したため、128bitのSSE2を使う。
	args = append(args, "-DUSE_BACKEND=EIGEN", "-DUSE_AVX2=0", "-DNO_GIT_REVISION=1", "-DBUILD_DISTRIBUTED=0", "-DEIGEN3_INCLUDE_DIRS="+filepath.Join(cache, "eigen-3.4.0"), "-DZLIB_LIBRARY="+filepath.Join(zBuild, "libzlibstatic.a"), "-DZLIB_INCLUDE_DIR="+zSource, "-DCMAKE_EXE_LINKER_FLAGS=-static -static-libgcc -static-libstdc++", "-DCMAKE_CXX_FLAGS=-I\""+filepath.ToSlash(zBuild)+"\" -include \""+filepath.ToSlash(filepath.Join(root, "engine", "build", "mingw_compat.h"))+"\"", "-DCMAKE_IGNORE_PATH="+filepath.Join(*compilerRoot, "lib"))
	if err = command(ctx, cache, cmake, args...); err != nil {
		return err
	}
	if err = command(ctx, cache, cmake, "--build", kBuild, "--target", "katago", "--parallel", "4"); err != nil {
		return err
	}
	binary := filepath.Join(kBuild, "katago.exe")
	if err = checkImports(binary); err != nil {
		return err
	}
	dest := filepath.Join(root, "engine", "runtime", "katago.exe")
	if err = copyFile(binary, dest); err != nil {
		return err
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		return err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	provenance := struct {
		Version, Compiler, Backend, Source, SHA256 string
		Flags                                      []string
	}{"1.17.1", "MinGW GCC 16.1.0", "Eigen 3.4.0 / SSE2 / CPU", "https://github.com/lightvector/KataGo/tree/v1.17.1", hash, []string{"BUILD_DISTRIBUTED=0", "NO_LIBZIP", "NO_GIT_REVISION=1", "USE_AVX2=0 (GCC PR54412)", "static runtime", "zlib 1.3.1", "mingw_compat.h"}}
	encoded, err := json.MarshalIndent(provenance, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(root, "engine", "build-provenance.json"), append(encoded, '\n'), 0600); err != nil {
		return err
	}
	for _, pair := range [][2]string{{"mingw-w64/COPYING.MinGW-w64-runtime.txt", "MinGW-runtime.txt"}, {"mingw-w64/COPYING.MinGW-w64.txt", "MinGW.txt"}, {"winpthreads/COPYING", "winpthreads.txt"}, {"gcc/COPYING.RUNTIME", "GCC-Runtime-Exception.txt"}, {"gcc/COPYING3", "GCC-GPL3.txt"}} {
		if err = copyFile(filepath.Join(*compilerRoot, "licenses", filepath.FromSlash(pair[0])), filepath.Join(root, "licenses", pair[1])); err != nil {
			return err
		}
	}
	fmt.Println("built engine/runtime/katago.exe", hash)
	return nil
}

// command は1コマンドずつ実行し、有限ログをプロジェクト内に保存する。
func command(ctx context.Context, dir, program string, args ...string) error {
	log, err := os.CreateTemp(dir, "build-*.log")
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Dir = dir
	cmd.Stdout = log
	cmd.Stderr = log
	err = cmd.Run()
	closeErr := log.Close()
	if err != nil {
		data, readErr := os.ReadFile(log.Name())
		if readErr == nil {
			if len(data) > 6000 {
				data = data[len(data)-6000:]
			}
			fmt.Fprintln(os.Stderr, string(data))
		}
		return errors.Join(err, closeErr, readErr)
	}
	fmt.Println("passed", filepath.Base(program), strings.Join(args[:min(3, len(args))], " "))
	return closeErr
}

// checkImports は標準Windows DLLだけへの依存を確認する。
func checkImports(path string) (result error) {
	f, err := pe.Open(path)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, f.Close()) }()
	symbols, err := f.ImportedSymbols()
	if err != nil {
		return err
	}
	allowed := map[string]bool{"kernel32.dll": true, "msvcrt.dll": true, "ucrtbase.dll": true, "advapi32.dll": true, "user32.dll": true, "shell32.dll": true, "ws2_32.dll": true, "bcrypt.dll": true, "shlwapi.dll": true, "ntdll.dll": true}
	for _, s := range symbols {
		_, lib, ok := strings.Cut(s, ":")
		lib = strings.ToLower(lib)
		if ok && !allowed[lib] && !strings.HasPrefix(lib, "api-ms-win-") {
			return fmt.Errorf("non-system dependency: %s", lib)
		}
	}
	return nil
}

// copyFile は生成したファイルだけを指定先へ保存する。
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
