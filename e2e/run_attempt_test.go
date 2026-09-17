//go:build e2e

package e2e

import (
	"context"
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

func TestRunAttemptsKeepArtifactsDistinct(t *testing.T) {
	t.Parallel()
	s := newSuiteInReport(t, "run-attempts")
	s.newHead("run-attempts")
	sha := s.github.head()

	first := s.runWithIdentity("preview-attempt-1.log", "preview", 500, 1, 0)
	second := s.runWithIdentity("preview-attempt-2.log", "preview", 500, 2, 0)
	s.require(first.RunID == "run-500-1-"+sha, "first preview run ID = %q", first.RunID)
	s.require(second.RunID == "run-500-2-"+sha, "second preview run ID = %q", second.RunID)

	firstManifest := readRunManifest(t, s, first.RunID)
	secondManifest := readRunManifest(t, s, second.RunID)
	firstPlan := s.stack(firstManifest).PlanKey
	secondPlan := s.stack(secondManifest).PlanKey
	s.require(firstPlan != "" && secondPlan != "" && firstPlan != secondPlan,
		"rerun plans are not distinct: first=%q second=%q", firstPlan, secondPlan)
	s.require(exists(filepath.Join(s.root, ".reeve-state", firstPlan)), "first-attempt plan is missing")
	s.require(exists(filepath.Join(s.root, ".reeve-state", secondPlan)), "second-attempt plan is missing")

	// Corrupt only the older attempt. A successful saved-plan apply proves
	// the coherent snapshot selected the newer rerun instead of mixing artifacts.
	s.write(filepath.Join(s.root, ".reeve-state", firstPlan), []byte("invalid older plan\n"), 0o600)
	s.github.approve("", "", "")
	apply := s.runWithIdentity("apply-attempt-2.log", "apply", 501, 2, 0)
	s.require(apply.RunID == "apply-501-2-"+sha, "apply run ID = %q", apply.RunID)
	applyManifest := readRunManifest(t, s, apply.RunID)
	s.require(s.stack(applyManifest).Status == "planned", "attempt-aware apply did not complete")
	s.require(hasCommand(apply.EngineCommands, "apply "), "attempt-aware apply did not execute the saved plan")
	s.require(!hasCommand(apply.EngineCommands, "plan "), "attempt-aware apply re-planned instead of using the newer attempt")
	s.audit(apply, "success")
	s.locksReleased()

	refresh := s.runWithIdentity("refresh-attempt-2.log", "refresh", 502, 2, 0)
	s.require(refresh.RunID == "refresh-502-2-"+sha, "refresh run ID = %q", refresh.RunID)
	s.audit(refresh, "success")
	s.locksReleased()

	var unexpected []string
	s.github.edit(func(g *githubState) { unexpected = append(unexpected, g.unexpected...) })
	s.require(len(unexpected) == 0, "Unexpected API calls: %v", unexpected)
}

func (s *suite) runWithIdentity(logName, command string, runNumber, runAttempt, expectedExit int) record {
	s.t.Helper()
	requestsBefore := s.github.requestSnapshot()
	sha := s.github.head()
	prRequestKey := fmt.Sprintf("GET /repos/%s/pulls/%d", s.repo, s.pr)
	prReadsBefore := 0
	s.github.edit(func(g *githubState) { prReadsBefore = g.requests[prRequestKey] })
	prefix := command
	if command == "preview" {
		prefix = "run"
	}
	r := record{
		Scenario: strings.TrimSuffix(logName, ".log"),
		RunID:    fmt.Sprintf("%s-%d-%d-%s", prefix, runNumber, runAttempt, sha),
		Log:      logName,
	}
	args := []string{
		"run", command, "--root", s.root, "--repo", s.repo, "--pr", strconv.Itoa(s.pr),
		"--sha", sha, "--run-number", strconv.Itoa(runNumber), "--run-attempt", strconv.Itoa(runAttempt),
	}
	if command == "apply" || command == "refresh" {
		args = append(args, "--actor", s.actor)
	}
	if command == "apply" {
		args = append(args, "--trigger-source", "comment")
	}

	before := len(s.commands())
	ctx, cancel := context.WithTimeout(s.t.Context(), 120*time.Second)
	defer cancel()
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
	var output tailBuffer
	cmd.Stdout, cmd.Stderr = &output, &output
	started := time.Now()
	err := cmd.Run()
	r.Seconds = time.Since(started).Seconds()
	r.ExitCode = -1
	if cmd.ProcessState != nil {
		r.ExitCode = cmd.ProcessState.ExitCode()
	}
	r.EngineCommands = s.commands()[before:]
	r.APIRequests = requestDelta(requestsBefore, s.github.requestSnapshot())
	s.write(filepath.Join(s.report, logName), output.data, 0o600)
	s.require(ctx.Err() == nil, "%s timed out", r.Scenario)
	var exitErr *exec.ExitError
	s.require(err == nil || errors.As(err, &exitErr), "%s failed: %v", r.Scenario, err)
	s.require(r.ExitCode == expectedExit, "%s exit = %d, want %d", r.Scenario, r.ExitCode, expectedExit)
	prReadsAfter := 0
	s.github.edit(func(g *githubState) { prReadsAfter = g.requests[prRequestKey] })
	s.require(prReadsAfter-prReadsBefore == 1, "%s fetched PR metadata %d times, want one coherent snapshot",
		r.Scenario, prReadsAfter-prReadsBefore)
	manifestPath := filepath.Join(s.root, ".reeve-state", "runs", fmt.Sprintf("pr-%d", s.pr), r.RunID, "manifest.json")
	r.Manifest = exists(manifestPath)
	s.results = append(s.results, r)
	return r
}
