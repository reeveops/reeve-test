//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

type applyProcess struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
	output *tailBuffer
}

type persistedLock struct {
	Holder *struct {
		RunID     string `json:"run_id"`
		ExpiresAt string `json:"expires_at"`
	} `json:"holder"`
	Queue []json.RawMessage `json:"queue"`
}

func TestConcurrentApplyHeartbeatKeepsLiveHolder(t *testing.T) {
	s := newSuiteInReport(t, "concurrent-apply")
	s.newHead("concurrent-apply")
	s.github.approve("", "", "")

	sharedPath := filepath.Join(s.root, ".reeve", "shared.yaml")
	shared := strings.Replace(string(s.read(sharedPath)), "ttl: 1m", "ttl: 2s", 1)
	s.require(strings.Contains(shared, "ttl: 2s"), "locking ttl was not replaced")
	s.write(sharedPath, []byte(shared), 0o600)

	originalWrapper := filepath.Join(s.root, "tofu-traced")
	slowWrapper := filepath.Join(s.root, "tofu-slow-apply")
	s.write(slowWrapper, []byte(slowApplyScript(originalWrapper)), 0o700)
	enginePath := filepath.Join(s.root, ".reeve", "tofu.yaml")
	engineConfig := strings.Replace(string(s.read(enginePath)), originalWrapper, slowWrapper, 1)
	s.require(strings.Contains(engineConfig, slowWrapper), "engine wrapper was not replaced")
	s.write(enginePath, []byte(engineConfig), 0o600)

	s.preview("concurrent-preview", counts{Add: 1})
	sha := s.github.head()
	firstRunID := "apply-200-1-" + sha[:7]
	first := startApplyProcess(t, s, 200, 1)
	lockPath := filepath.Join(s.root, ".reeve-state", "locks", "lifecycle", "default.json")
	initial := waitForLock(t, lockPath, func(lock persistedLock) bool {
		return lock.Holder != nil && lock.Holder.RunID == firstRunID
	})
	initialExpiry := parseExpiry(t, initial)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !time.Now().After(initialExpiry.Add(100*time.Millisecond)) {
		time.Sleep(25 * time.Millisecond)
	}
	extended := waitForLock(t, lockPath, func(lock persistedLock) bool {
		if lock.Holder == nil || lock.Holder.RunID != firstRunID {
			return false
		}
		expiresAt, err := time.Parse(time.RFC3339, lock.Holder.ExpiresAt)
		return err == nil && expiresAt.After(initialExpiry) && expiresAt.After(time.Now())
	})
	if !parseExpiry(t, extended).After(initialExpiry) {
		t.Fatal("heartbeat did not extend the original lease")
	}

	secondRunID := "apply-201-1-" + sha[:7]
	second := startApplyProcess(t, s, 201, 1)
	waitApplyProcess(t, s, "concurrent-second.log", second, 0)
	secondManifest := readRunManifest(t, s, secondRunID)
	secondStack := s.stack(secondManifest)
	s.require(secondStack.Status == "blocked", "concurrent apply status = %q, want blocked", secondStack.Status)

	waitApplyProcess(t, s, "concurrent-first.log", first, 0)
	firstManifest := readRunManifest(t, s, firstRunID)
	firstStack := s.stack(firstManifest)
	s.require(firstStack.Status == "planned", "holder apply status = %q, want planned", firstStack.Status)

	applyCommands := 0
	for _, command := range s.commands() {
		if strings.HasPrefix(command, "apply ") {
			applyCommands++
		}
	}
	s.require(applyCommands == 1, "engine apply commands = %d, want 1", applyCommands)
	s.locksReleased()
}

func startApplyProcess(t *testing.T, s *suite, runNumber, runAttempt int) *applyProcess {
	t.Helper()
	sha := s.github.head()
	args := []string{
		"run", "apply", "--root", s.root, "--repo", s.repo, "--pr", strconv.Itoa(s.pr),
		"--sha", sha, "--run-number", strconv.Itoa(runNumber), "--run-attempt", strconv.Itoa(runAttempt),
		"--actor", s.actor, "--trigger-source", "comment",
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	// #nosec G204 -- The test invokes the configured Reeve binary with fixed fixture arguments.
	cmd := exec.CommandContext(ctx, s.reeve, args...)
	cmd.Dir, cmd.Env = s.root, s.env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 5 * time.Second
	output := &tailBuffer{}
	cmd.Stdout, cmd.Stderr = output, output
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(cancel)
	return &applyProcess{cmd: cmd, cancel: cancel, output: output}
}

func waitApplyProcess(t *testing.T, s *suite, logName string, process *applyProcess, expected int) {
	t.Helper()
	err := process.cmd.Wait()
	process.cancel()
	s.write(filepath.Join(s.report, logName), process.output.data, 0o600)
	exitCode := -1
	if process.cmd.ProcessState != nil {
		exitCode = process.cmd.ProcessState.ExitCode()
	}
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		t.Fatalf("apply process failed: %v", err)
	}
	if exitCode != expected {
		t.Fatalf("apply process exit = %d, want %d; see %s", exitCode, expected, logName)
	}
}

func waitForLock(t *testing.T, path string, accept func(persistedLock) bool) persistedLock {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			var lock persistedLock
			if json.Unmarshal(data, &lock) == nil && accept(lock) {
				return lock
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("lock condition not met for %s", path)
	return persistedLock{}
}

func parseExpiry(t *testing.T, lock persistedLock) time.Time {
	t.Helper()
	if lock.Holder == nil {
		t.Fatal("lock has no holder")
	}
	expiresAt, err := time.Parse(time.RFC3339, lock.Holder.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	return expiresAt
}

func readRunManifest(t *testing.T, s *suite, runID string) *manifest {
	t.Helper()
	path := filepath.Join(s.root, ".reeve-state", "runs", "pr-1", runID, "manifest.json")
	result := &manifest{}
	s.readJSON(path, result)
	return result
}

func slowApplyScript(target string) string {
	quoted := "'" + strings.ReplaceAll(target, "'", "'\"'\"'") + "'"
	return "#!/bin/sh\nset -eu\nif [ \"${1:-}\" = apply ]; then\n  sleep 6\nfi\nexec " + quoted + " \"$@\"\n"
}
