//go:build ignore

// Explicitly built Linux research helper; not imported by production packages.
package main

import (
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
	"strconv"
	"sync"
	"syscall"
	"time"
)

type limitedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
	cancel   context.CancelFunc
}

// Write bounds captured compiler output and cancels excessive output.
//
// Version:
//   - 2026-09-22: Added.
func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.buffer.Len() {
		b.exceeded = true
		b.cancel()
		return 0, fmt.Errorf("failed to capture compiler output: size=too_long")
	}
	return b.buffer.Write(p)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "__exec" {
		if err := child(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func child() error {
	if len(os.Args) < 6 {
		return fmt.Errorf("failed to execute compiler: arguments=too_short")
	}
	memory, err := strconv.ParseUint(os.Args[2], 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse memory limit: %w", err)
	}
	cpu, err := strconv.ParseUint(os.Args[3], 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse cpu limit: %w", err)
	}
	if memory > 0 {
		if err := syscall.Setrlimit(syscall.RLIMIT_AS, &syscall.Rlimit{Cur: memory, Max: memory}); err != nil {
			return fmt.Errorf("failed to limit compiler memory: %w", err)
		}
	}
	if cpu > 0 {
		if err := syscall.Setrlimit(syscall.RLIMIT_CPU, &syscall.Rlimit{Cur: cpu, Max: cpu}); err != nil {
			return fmt.Errorf("failed to limit compiler cpu: %w", err)
		}
	}
	if err := syscall.Exec(os.Args[4], os.Args[4:], []string{"PATH=/usr/bin:/bin", "HOME=/tmp", "LANG=C"}); err != nil {
		return fmt.Errorf("failed to execute compiler: %w", err)
	}
	return nil
}

func run() error {
	timeout := flag.Duration("timeout", 15*time.Second, "wall time limit")
	memory := flag.Uint64("memory-mib", 512, "child address space limit; zero disables")
	cpu := flag.Uint64("cpu-seconds", 5, "child cpu time limit")
	outputLimit := flag.Int("output-bytes", 8*1024*1024, "stdout size limit")
	cancelAfter := flag.Duration("cancel-after", 0, "explicit cancellation delay")
	flag.Parse()
	if flag.NArg() < 1 {
		return fmt.Errorf("failed to supervise compiler: command=empty")
	}
	input, err := io.ReadAll(io.LimitReader(os.Stdin, 2*1024*1024+1))
	if err != nil {
		return fmt.Errorf("failed to read compiler input: %w", err)
	}
	if len(input) > 2*1024*1024 {
		return fmt.Errorf("failed to read compiler input: input=too_long")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	if *cancelAfter > 0 {
		timer := time.AfterFunc(*cancelAfter, cancel)
		defer timer.Stop()
	}
	args := append([]string{"__exec", strconv.FormatUint(*memory*1024*1024, 10), strconv.FormatUint(*cpu, 10)}, flag.Args()...)
	cmd := exec.CommandContext(ctx, os.Args[0], args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var mu sync.Mutex
	cmd.Cancel = func() error {
		mu.Lock()
		defer mu.Unlock()
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = time.Second
	out := &limitedBuffer{limit: *outputLimit, cancel: cancel}
	stderr := &limitedBuffer{limit: 64 * 1024, cancel: cancel}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = bytes.NewReader(input), out, stderr
	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start)
	report := map[string]any{"elapsedMs": float64(elapsed.Microseconds()) / 1000,
		"exitCode": -1, "stdoutBytes": out.buffer.Len(), "stderrBytes": stderr.buffer.Len(),
		"outputLimitExceeded": out.exceeded || stderr.exceeded,
		"deadlineExceeded":    errors.Is(ctx.Err(), context.DeadlineExceeded),
		"canceled":            errors.Is(ctx.Err(), context.Canceled), "success": runErr == nil}
	if cmd.ProcessState != nil {
		report["exitCode"] = cmd.ProcessState.ExitCode()
		if r, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage); ok {
			report["maxRssKiB"] = r.Maxrss
			report["cpuUserMs"] = float64(r.Utime.Nano()) / 1e6
			report["cpuSystemMs"] = float64(r.Stime.Nano()) / 1e6
		}
		if s, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && s.Signaled() {
			report["signal"] = s.Signal().String()
		}
	}
	digest := sha256.Sum256(out.buffer.Bytes())
	report["stdoutSHA256"] = hex.EncodeToString(digest[:])
	if json.Valid(out.buffer.Bytes()) {
		report["output"] = json.RawMessage(out.buffer.Bytes())
	}
	if runErr != nil {
		report["error"] = runErr.Error()
	}
	if stderr.buffer.Len() > 0 {
		report["stderr"] = stderr.buffer.String()
	}
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		return fmt.Errorf("failed to encode compiler report: %w", err)
	}
	return nil
}
