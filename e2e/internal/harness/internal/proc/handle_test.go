package proc

import (
	"bufio"
	"context"
	"errors"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestSignalAndWaitCanceledThenReaps(t *testing.T) {
	cmd := exec.Command("sh", "-c", "trap '' TERM; printf 'ready\\n'; while :; do sleep 60; done")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("create child stdout pipe: %v", err)
	}
	if startErr := cmd.Start(); startErr != nil {
		t.Fatalf("start child: %v", startErr)
	}
	handle := Reap(cmd, Hooks{})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = handle.SignalAndWait(ctx, syscall.SIGKILL, 0)
	})

	ready := make(chan error, 1)
	go func() {
		line, readErr := bufio.NewReader(stdout).ReadString('\n')
		if readErr == nil && line != "ready\n" {
			readErr = errors.New("child emitted an unexpected readiness line")
		}
		ready <- readErr
	}()
	select {
	case readyErr := <-ready:
		if readyErr != nil {
			t.Fatalf("wait for child readiness: %v", readyErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("child did not become ready")
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	err = handle.SignalAndWait(canceled, syscall.SIGTERM, time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled SignalAndWait error = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("canceled SignalAndWait took %v", elapsed)
	}

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer waitCancel()
	if err := handle.SignalAndWait(waitCtx, syscall.SIGTERM, 0); err != nil {
		t.Fatalf("subsequent SignalAndWait: %v", err)
	}
	if handle.Err() == nil {
		t.Fatal("reaped SIGKILL child has no wait error")
	}
}

func TestSignalAndWaitSIGKILLWaitsForReap(t *testing.T) {
	cmd := exec.Command("sh", "-c", "while :; do sleep 60; done")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}

	allowWait := make(chan struct{})
	handle := Reap(cmd, Hooks{BeforeWait: func() { <-allowWait }})
	t.Cleanup(func() {
		closeIfOpen(allowWait)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = handle.SignalAndWait(ctx, syscall.SIGKILL, 0)
	})

	result := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		result <- handle.SignalAndWait(ctx, syscall.SIGKILL, 20*time.Millisecond)
	}()

	select {
	case err := <-result:
		t.Fatalf("SignalAndWait returned before reap: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(allowWait)
	if err := <-result; err != nil {
		t.Fatalf("SignalAndWait with SIGKILL and grace: %v", err)
	}
}

func closeIfOpen(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}
