// SPDX-License-Identifier: Apache-2.0

package ibclink

import (
	"bufio"
	"context"
	"errors"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSignalAndWaitCanceledThenReaps(t *testing.T) {
	cmd := exec.Command("sh", "-c", "trap '' TERM; printf 'ready\\n'; while :; do sleep 60; done")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err, "create child stdout pipe")
	require.NoError(t, cmd.Start(), "start child")
	handle := reapProcess(cmd, processHooks{})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = handle.signalAndWait(ctx, syscall.SIGKILL, 0)
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
		require.NoError(t, readyErr, "wait for child readiness")
	case <-time.After(5 * time.Second):
		require.FailNow(t, "child did not become ready")
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	err = handle.signalAndWait(canceled, syscall.SIGTERM, time.Minute)
	require.ErrorIs(t, err, context.Canceled, "canceled SignalAndWait")
	require.Less(t, time.Since(started), 2*time.Second, "canceled SignalAndWait took too long")

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer waitCancel()
	require.NoError(t, handle.signalAndWait(waitCtx, syscall.SIGTERM, 0), "subsequent SignalAndWait")
	require.Error(t, handle.err(), "reaped SIGKILL child has no wait error")
}

func TestSignalAndWaitSIGKILLWaitsForReap(t *testing.T) {
	cmd := exec.Command("sh", "-c", "while :; do sleep 60; done")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start(), "start child")

	allowWait := make(chan struct{})
	handle := reapProcess(cmd, processHooks{BeforeWait: func() { <-allowWait }})
	t.Cleanup(func() {
		closeIfOpen(allowWait)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = handle.signalAndWait(ctx, syscall.SIGKILL, 0)
	})

	result := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		result <- handle.signalAndWait(ctx, syscall.SIGKILL, 20*time.Millisecond)
	}()

	select {
	case err := <-result:
		require.FailNow(t, "SignalAndWait returned before reap", "%v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(allowWait)
	require.NoError(t, <-result, "SignalAndWait with SIGKILL and grace")
}

func closeIfOpen(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}
