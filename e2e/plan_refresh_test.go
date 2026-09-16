//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanArtifactModes(t *testing.T) {
	t.Parallel()
	s := newSuiteInReport(t, "plan-artifact-modes")
	s.newHead("plan-mode-baseline")
	s.preview("plan-mode-baseline-preview", counts{Add: 1})
	s.github.approve("", "", "")
	s.apply("plan-mode-baseline-apply", false)

	configPath := filepath.Join(s.root, ".reeve", "tofu.yaml")
	s.setPlanLocking(configPath, false)
	s.newHead("plan-locking-off")
	s.replaceInput("initial", "unlocked")
	m, _ := s.run("unlocked-preview", "preview", 0)
	stack := s.stack(m)
	s.require(stack.Counts == (counts{Change: 1}), "Unlocked preview counts: %+v", stack.Counts)
	s.require(stack.PlanKey == "", "Unlocked preview stored plan %q", stack.PlanKey)
	s.github.approve("", "", "")
	m, r := s.run("unlocked-apply", "apply", 0)
	s.require(s.stack(m).Status == "planned", "Unlocked apply did not complete")
	s.require(hasCommand(r.EngineCommands, "plan "), "Unlocked apply did not compute a fresh plan: %v", r.EngineCommands)
	s.require(hasCommand(r.EngineCommands, "apply "), "Unlocked apply did not execute: %v", r.EngineCommands)
	s.require(s.appliedMarker(), "Unlocked apply did not write an applied marker")
	s.audit(r, "success")
	s.locksReleased()

	s.setPlanLocking(configPath, true)
	s.newHead("missing-plan-fallback")
	s.replaceInput("unlocked", "missing-plan")
	m, _ = s.run("missing-plan-preview", "preview", 0)
	stack = s.stack(m)
	s.require(stack.PlanKey != "", "Locked preview did not store a plan")
	s.check(os.Remove(filepath.Join(s.root, ".reeve-state", stack.PlanKey)))
	s.github.approve("", "", "")
	m, r = s.run("missing-plan-apply", "apply", 0)
	s.require(s.stack(m).Status == "planned", "Missing-plan fallback did not complete")
	s.require(hasCommand(r.EngineCommands, "plan "), "Missing-plan fallback did not re-plan: %v", r.EngineCommands)
	s.require(hasCommand(r.EngineCommands, "apply "), "Missing-plan fallback did not apply: %v", r.EngineCommands)
	s.audit(r, "success")
	s.locksReleased()
}

func TestRefreshModes(t *testing.T) {
	t.Parallel()
	s := newSuiteInReport(t, "refresh-modes")
	s.newHead("refresh-baseline")
	s.preview("refresh-baseline-preview", counts{Add: 1})
	s.github.approve("", "", "")
	s.apply("refresh-baseline-apply", false)

	m, dryRun := s.runArgs("refresh-dry-run", "refresh", 0, "--dry-run")
	s.require(m == nil, "Dry-run refresh unexpectedly wrote a run manifest")
	s.require(hasCommand(dryRun.EngineCommands, "plan -refresh-only "), "Dry-run refresh did not read live state: %v", dryRun.EngineCommands)
	s.require(!hasCommand(dryRun.EngineCommands, "apply "), "Dry-run refresh wrote state: %v", dryRun.EngineCommands)
	s.require(len(s.files(filepath.Join(s.root, ".reeve-state", "audit"), dryRun.RunID+".json")) == 0,
		"Dry-run refresh wrote a durable audit entry")

	s.newHead("writing-refresh")
	m, writing := s.run("refresh-write", "refresh", 0)
	s.require(m == nil, "Writing refresh unexpectedly wrote a run manifest")
	s.require(hasCommand(writing.EngineCommands, "plan -refresh-only "), "Writing refresh did not inspect live state: %v", writing.EngineCommands)
	s.require(!hasCommand(writing.EngineCommands, "apply "), "No-op refresh executed an apply: %v", writing.EngineCommands)
	s.audit(writing, "success")
	s.locksReleased()

	s.newHead("apply-with-refresh")
	s.replaceInput("initial", "refreshed")
	s.preview("refresh-apply-preview", counts{Change: 1})
	s.github.approve("", "", "")
	m, refreshed := s.runArgs("refresh-apply", "apply", 0, "--refresh")
	s.require(s.stack(m).Status == "planned", "Apply with refresh did not complete")
	s.require(hasCommand(refreshed.EngineCommands, "plan "), "Apply with refresh reused the locked plan: %v", refreshed.EngineCommands)
	s.require(hasCommand(refreshed.EngineCommands, "apply "), "Apply with refresh did not execute: %v", refreshed.EngineCommands)
	s.require(s.appliedMarker(), "Apply with refresh did not write an applied marker")
	s.audit(refreshed, "success")
	s.locksReleased()
}

func TestStaleSavedPlanFailsClosed(t *testing.T) {
	t.Parallel()
	s := newSuiteInReport(t, "stale-plan")
	s.newHead("stale-plan-baseline")
	s.preview("stale-plan-baseline-preview", counts{Add: 1})
	s.github.approve("", "", "")
	s.apply("stale-plan-baseline-apply", false)

	s.newHead("stale-plan")
	s.replaceInput("initial", "planned")
	s.preview("stale-plan-preview", counts{Change: 1})
	s.replaceInput("planned", "out-of-band")
	// #nosec G204 -- The test invokes the configured engine in its disposable fixture.
	cmd := exec.CommandContext(t.Context(), s.engine, "apply", "-auto-approve", "-input=false", "-no-color")
	cmd.Dir, cmd.Env = s.module, s.env
	output, err := cmd.CombinedOutput()
	s.require(err == nil, "Out-of-band state change failed: %v\n%s", err, output)
	s.replaceInput("out-of-band", "planned")

	s.github.approve("", "", "")
	m, r := s.run("stale-plan-apply", "apply", 1)
	s.require(s.stack(m).Status == "error", "Stale plan did not fail")
	s.require(hasCommand(r.EngineCommands, "apply "), "Stale saved plan was not attempted: %v", r.EngineCommands)
	s.require(!hasCommand(r.EngineCommands, "plan "), "Stale saved plan silently re-planned: %v", r.EngineCommands)
	s.require(!s.appliedMarker(), "Stale plan wrote an applied marker")
	s.audit(r, "failed")
	s.locksReleased()
}

func (s *suite) setPlanLocking(configPath string, enabled bool) {
	s.t.Helper()
	oldValue, newValue := "plan_locking: true", "plan_locking: false"
	if enabled {
		oldValue, newValue = newValue, oldValue
	}
	config := string(s.read(configPath))
	s.require(strings.Contains(config, oldValue), "Engine config does not contain %q", oldValue)
	s.write(configPath, []byte(strings.Replace(config, oldValue, newValue, 1)), 0600)
}

func (s *suite) replaceInput(oldValue, newValue string) {
	s.t.Helper()
	path := filepath.Join(s.module, "main.tf")
	source := string(s.read(path))
	oldQuoted, newQuoted := `"`+oldValue+`"`, `"`+newValue+`"`
	s.require(strings.Contains(source, oldQuoted), "Fixture does not contain %s", oldQuoted)
	s.write(path, []byte(strings.Replace(source, oldQuoted, newQuoted, 1)), 0600)
}

func hasCommand(commands []string, prefix string) bool {
	for _, command := range commands {
		if strings.HasPrefix(command, prefix) {
			return true
		}
	}
	return false
}
