//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMaintenanceLifecycle(t *testing.T) {
	s := newSuiteInReport(t, "maintenance")
	sharedPath := filepath.Join(s.root, ".reeve", "shared.yaml")
	shared := string(s.read(sharedPath)) + "\nretention:\n  max_age: 1h\n"
	s.write(sharedPath, []byte(shared), 0o600)

	lockDir := filepath.Join(s.root, ".reeve-state", "locks", "lifecycle")
	s.check(os.MkdirAll(lockDir, 0o700))
	now := time.Now().UTC()
	activePath := filepath.Join(lockDir, "active.json")
	expiredPath := filepath.Join(lockDir, "expired.json")
	malformedPath := filepath.Join(lockDir, "malformed.json")
	s.writeLock(activePath, maintenanceLock("lifecycle", "active", 1, "active-run", now.Add(time.Hour), nil))
	s.writeLock(expiredPath, maintenanceLock("lifecycle", "expired", 9, "expired-run", now.Add(-time.Hour), []map[string]any{{
		"pr": 2, "commit_sha": strings.Repeat("2", 40), "run_id": "queued-run", "actor": "queued-actor",
		"enqueued_at": now.Add(-30 * time.Minute).Format(time.RFC3339),
	}}))
	s.write(malformedPath, []byte("{not-json\n"), 0o600)

	oldArtifact := filepath.Join(s.root, ".reeve-state", "runs", "pr-9", "old", "manifest.json")
	freshArtifact := filepath.Join(s.root, ".reeve-state", "runs", "pr-9", "fresh", "manifest.json")
	s.check(os.MkdirAll(filepath.Dir(oldArtifact), 0o700))
	s.check(os.MkdirAll(filepath.Dir(freshArtifact), 0o700))
	s.write(oldArtifact, []byte("{}\n"), 0o600)
	s.write(freshArtifact, []byte("{}\n"), 0o600)
	past := now.Add(-2 * time.Hour)
	s.check(os.Chtimes(oldArtifact, past, past))

	activeBefore := s.read(activePath)
	activeInfoBefore, err := os.Stat(activePath)
	s.check(err)
	first := s.runMaintenance("maintenance-first.log")
	for _, want := range []string{"expired locks reaped: 1", "expired run artifacts pruned: 1"} {
		s.require(strings.Contains(first, want), "maintenance output missing %q: %s", want, first)
	}
	s.require(!exists(oldArtifact), "maintenance retained an expired run artifact")
	s.require(exists(freshArtifact), "maintenance deleted a fresh run artifact")
	s.require(string(s.read(malformedPath)) == "{not-json\n", "maintenance changed a malformed lock")

	activeInfoAfter, err := os.Stat(activePath)
	s.check(err)
	s.require(string(s.read(activePath)) == string(activeBefore), "maintenance rewrote an active lock")
	s.require(activeInfoAfter.ModTime().Equal(activeInfoBefore.ModTime()), "maintenance changed the active lock timestamp")

	var promoted persistedLock
	s.readJSON(expiredPath, &promoted)
	s.require(promoted.Holder != nil && promoted.Holder.PR == 2 && promoted.Holder.RunID == "queued-run" && promoted.Holder.Promoted,
		"maintenance did not promote the first queued PR: %+v", promoted.Holder)
	s.require(len(promoted.Queue) == 0, "maintenance left the promoted PR in the queue")

	promotedBefore := s.read(expiredPath)
	promotedInfoBefore, err := os.Stat(expiredPath)
	s.check(err)
	second := s.runMaintenance("maintenance-second.log")
	for _, want := range []string{"expired locks reaped: 0", "expired run artifacts pruned: 0"} {
		s.require(strings.Contains(second, want), "second maintenance output missing %q: %s", want, second)
	}
	promotedInfoAfter, err := os.Stat(expiredPath)
	s.check(err)
	s.require(string(s.read(expiredPath)) == string(promotedBefore), "no-op maintenance rewrote a promoted lock")
	s.require(promotedInfoAfter.ModTime().Equal(promotedInfoBefore.ModTime()), "no-op maintenance changed the promoted lock timestamp")
	s.require(len(s.commands()) == 0, "maintenance invoked the IaC engine: %v", s.commands())
}

func maintenanceLock(project, stack string, pr int, runID string, expiresAt time.Time, queue []map[string]any) map[string]any {
	now := time.Now().UTC()
	if queue == nil {
		queue = []map[string]any{}
	}
	return map[string]any{
		"project": project, "stack": stack,
		"holder": map[string]any{
			"pr": pr, "commit_sha": strings.Repeat("1", 40), "run_id": runID, "actor": "holder-actor",
			"acquired_at": now.Add(-time.Hour).Format(time.RFC3339), "expires_at": expiresAt.Format(time.RFC3339),
		},
		"queue": queue, "updated_at": now.Add(-time.Hour).Format(time.RFC3339),
	}
}

func (s *suite) writeLock(path string, lock map[string]any) {
	s.t.Helper()
	data, err := json.Marshal(lock)
	s.check(err)
	s.write(path, data, 0o600)
}

func (s *suite) runMaintenance(logName string) string {
	s.t.Helper()
	ctx, cancel := context.WithTimeout(s.t.Context(), 30*time.Second)
	defer cancel()
	// #nosec G204 -- The test invokes the configured Reeve binary against its disposable fixture.
	cmd := exec.CommandContext(ctx, s.reeve, "maintenance", "run", "--root", s.root)
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
	var output tailBuffer
	cmd.Stdout, cmd.Stderr = &output, &output
	err := cmd.Run()
	s.write(filepath.Join(s.report, logName), output.data, 0o600)
	s.require(ctx.Err() == nil, "maintenance timed out; see %s", logName)
	s.require(err == nil, "maintenance failed: %v; see %s", err, logName)
	return string(output.data)
}
