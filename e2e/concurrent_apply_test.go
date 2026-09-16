//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		PR        int    `json:"pr"`
		RunID     string `json:"run_id"`
		ExpiresAt string `json:"expires_at"`
		Promoted  bool   `json:"promoted"`
	} `json:"holder"`
	Queue []struct {
		PR    int    `json:"pr"`
		RunID string `json:"run_id"`
	} `json:"queue"`
}

func TestConcurrentApplyHeartbeatKeepsLiveHolder(t *testing.T) {
	t.Parallel()
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

func TestApplyQueuePromotesPRsInFIFOOrder(t *testing.T) {
	t.Parallel()
	s := newSuiteInReport(t, "fifo-promotion")
	configPath := filepath.Join(s.root, ".reeve", "tofu.yaml")
	s.setPlanLocking(configPath, false)

	heads := map[int]string{}
	for _, pr := range []int{2, 3, 1} {
		s.pr = pr
		s.newHead("fifo-pr-" + strconv.Itoa(pr))
		heads[pr] = s.github.head()
		m, _ := s.run("fifo-pr-"+strconv.Itoa(pr)+"-preview", "preview", 0)
		stack := s.stack(m)
		s.require(stack.Status == "planned" && stack.Counts == (counts{Add: 1}), "PR %d preview did not plan the fixture", pr)
		s.require(stack.PlanKey == "", "PR %d preview stored a plan with plan locking disabled", pr)
	}

	originalWrapper := filepath.Join(s.root, "tofu-traced")
	slowWrapper := filepath.Join(s.root, "tofu-fifo-slow-apply")
	s.write(slowWrapper, []byte(slowApplyScript(originalWrapper)), 0o700)
	originalConfig := string(s.read(configPath))
	slowConfig := strings.Replace(originalConfig, originalWrapper, slowWrapper, 1)
	s.require(strings.Contains(slowConfig, slowWrapper), "engine wrapper was not replaced")
	s.write(configPath, []byte(slowConfig), 0o600)

	s.pr = 1
	s.github.edit(func(g *githubState) { g.sha = heads[1] })
	s.github.approve("", "", "")
	firstRunID := "apply-300-1-" + heads[1][:7]
	first := startApplyProcess(t, s, 300, 1)
	lockPath := filepath.Join(s.root, ".reeve-state", "locks", "lifecycle", "default.json")
	waitForLock(t, lockPath, func(lock persistedLock) bool {
		return lock.Holder != nil && lock.Holder.PR == 1 && lock.Holder.RunID == firstRunID
	})
	// The running process already owns its engine configuration. Restore the
	// normal wrapper so promoted applicants finish without an artificial delay.
	s.write(configPath, []byte(originalConfig), 0o600)

	for _, queued := range []struct {
		pr, run int
	}{{pr: 2, run: 301}, {pr: 3, run: 302}} {
		s.pr = queued.pr
		s.github.edit(func(g *githubState) { g.sha = heads[queued.pr] })
		s.github.approve("", "", "")
		process := startApplyProcess(t, s, queued.run, 1)
		waitApplyProcess(t, s, fmt.Sprintf("fifo-pr-%d-blocked.log", queued.pr), process, 0)
		runID := fmt.Sprintf("apply-%d-1-%s", queued.run, heads[queued.pr][:7])
		m := readRunManifestForPR(t, s, queued.pr, runID)
		s.require(s.stack(m).Status == "blocked", "PR %d did not block behind the active holder", queued.pr)
		waitForLock(t, lockPath, func(lock persistedLock) bool {
			if len(lock.Queue) != queued.pr-1 {
				return false
			}
			for i, pr := range []int{2, 3}[:queued.pr-1] {
				if lock.Queue[i].PR != pr {
					return false
				}
			}
			return true
		})
	}

	waitApplyProcess(t, s, "fifo-pr-1-holder.log", first, 0)
	waitForLock(t, lockPath, func(lock persistedLock) bool {
		return lock.Holder != nil && lock.Holder.PR == 2 && lock.Holder.Promoted && len(lock.Queue) == 1 && lock.Queue[0].PR == 3
	})

	for _, promoted := range []struct {
		pr, run, next int
	}{{pr: 2, run: 303, next: 3}, {pr: 3, run: 304}} {
		s.pr = promoted.pr
		s.github.edit(func(g *githubState) { g.sha = heads[promoted.pr] })
		s.github.approve("", "", "")
		process := startApplyProcess(t, s, promoted.run, 1)
		waitApplyProcess(t, s, fmt.Sprintf("fifo-pr-%d-promoted.log", promoted.pr), process, 0)
		runID := fmt.Sprintf("apply-%d-1-%s", promoted.run, heads[promoted.pr][:7])
		m := readRunManifestForPR(t, s, promoted.pr, runID)
		s.require(s.stack(m).Status != "blocked", "promoted PR %d did not adopt its reservation", promoted.pr)
		if promoted.next != 0 {
			waitForLock(t, lockPath, func(lock persistedLock) bool {
				return lock.Holder != nil && lock.Holder.PR == promoted.next && lock.Holder.Promoted && len(lock.Queue) == 0
			})
		}
	}

	s.locksReleased()
	var unexpected []string
	s.github.edit(func(g *githubState) { unexpected = append(unexpected, g.unexpected...) })
	s.require(len(unexpected) == 0, "Unexpected API calls: %v", unexpected)
}

func TestExpiredHolderIsEvictedByApply(t *testing.T) {
	t.Parallel()
	s := newSuiteInReport(t, "expired-holder")
	s.newHead("expired-holder")
	s.preview("expired-holder-preview", counts{Add: 1})
	s.github.approve("", "", "")

	lockPath := filepath.Join(s.root, ".reeve-state", "locks", "lifecycle", "default.json")
	s.check(os.MkdirAll(filepath.Dir(lockPath), 0o700))
	expired := map[string]any{
		"project": "lifecycle", "stack": "default",
		"holder": map[string]any{
			"pr": 99, "commit_sha": strings.Repeat("9", 40), "run_id": "expired-run", "actor": "expired-holder",
			"acquired_at": time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
			"expires_at":  time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		},
		"queue": []any{}, "updated_at": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(expired)
	s.check(err)
	s.write(lockPath, data, 0o600)

	s.apply("expired-holder-apply", false)
}

func TestCancelledApplyReleasesLockAndPersistsFailure(t *testing.T) {
	t.Parallel()
	s := newSuiteInReport(t, "cancelled-apply")
	s.newHead("cancelled-apply")
	s.preview("cancelled-apply-preview", counts{Add: 1})
	s.github.approve("", "", "")

	originalWrapper := filepath.Join(s.root, "tofu-traced")
	slowWrapper := filepath.Join(s.root, "tofu-cancelled-apply")
	s.write(slowWrapper, []byte(slowApplyScriptWithDelay(originalWrapper, 60*time.Second)), 0o700)
	configPath := filepath.Join(s.root, ".reeve", "tofu.yaml")
	config := strings.Replace(string(s.read(configPath)), originalWrapper, slowWrapper, 1)
	s.require(strings.Contains(config, slowWrapper), "engine wrapper was not replaced")
	s.write(configPath, []byte(config), 0o600)

	sha := s.github.head()
	runID := "apply-400-1-" + sha[:7]
	process := startApplyProcess(t, s, 400, 1)
	lockPath := filepath.Join(s.root, ".reeve-state", "locks", "lifecycle", "default.json")
	waitForLock(t, lockPath, func(lock persistedLock) bool {
		return lock.Holder != nil && lock.Holder.RunID == runID
	})
	s.require(process.cmd.Process.Signal(syscall.SIGTERM) == nil, "failed to signal the apply process")
	waitApplyProcess(t, s, "cancelled-apply.log", process, 1)

	m := readRunManifest(t, s, runID)
	s.require(s.stack(m).Status == "error", "cancelled apply did not persist an error result")
	s.require(!s.appliedMarker(), "cancelled apply wrote an applied marker")
	s.audit(record{Scenario: "cancelled-apply", RunID: runID}, "failed")
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
	return readRunManifestForPR(t, s, s.pr, runID)
}

func readRunManifestForPR(t *testing.T, s *suite, pr int, runID string) *manifest {
	t.Helper()
	path := filepath.Join(s.root, ".reeve-state", "runs", fmt.Sprintf("pr-%d", pr), runID, "manifest.json")
	result := &manifest{}
	s.readJSON(path, result)
	return result
}

func slowApplyScript(target string) string {
	return slowApplyScriptWithDelay(target, 6*time.Second)
}

func slowApplyScriptWithDelay(target string, delay time.Duration) string {
	quoted := "'" + strings.ReplaceAll(target, "'", "'\"'\"'") + "'"
	return "#!/bin/sh\nset -eu\nif [ \"${1:-}\" = apply ]; then\n  sleep " + strconv.Itoa(int(delay.Seconds())) + "\nfi\nexec " + quoted + " \"$@\"\n"
}
