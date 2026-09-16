//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPulumiLifecycle(t *testing.T) {
	t.Parallel()
	s := newPulumiSuite(t)
	s.newHead("pulumi-create")
	m, _ := s.run("pulumi-create-preview", "preview", 0)
	stack := s.stack(m)
	s.require(stack.Counts == (counts{Add: 2}), "Pulumi create counts: %+v", stack.Counts)
	s.require(stack.PlanKey != "", "Pulumi preview did not store its update plan")
	s.github.approve("", "", "")
	s.pulumiApply("pulumi-create-apply")

	m, _ = s.run("pulumi-converged-preview", "preview", 0)
	stack = s.stack(m)
	s.require(stack.Counts == (counts{}), "Pulumi converged counts: %+v", stack.Counts)
	s.require(stack.Status == "noop", "Pulumi converged preview status: %s", stack.Status)

	m, dryRun := s.runArgs("pulumi-refresh-dry-run", "refresh", 0, "--dry-run")
	s.require(m == nil, "Pulumi dry-run refresh unexpectedly wrote a run manifest")
	s.require(commandContains(dryRun.EngineCommands, "refresh ", "--preview-only"),
		"Pulumi dry-run refresh was not preview-only: %v", dryRun.EngineCommands)
	s.require(len(s.files(filepath.Join(s.root, ".reeve-state", "audit"), dryRun.RunID+".json")) == 0,
		"Pulumi dry-run refresh wrote a durable audit entry")

	s.newHead("pulumi-refresh")
	_, writing := s.run("pulumi-refresh-write", "refresh", 0)
	s.require(commandContains(writing.EngineCommands, "refresh ", "--yes"),
		"Pulumi writing refresh did not confirm its update: %v", writing.EngineCommands)
	s.audit(writing, "success")
	s.locksReleased()

	s.newHead("pulumi-delete")
	s.writePulumiProgram("")
	m, _ = s.run("pulumi-delete-preview", "preview", 0)
	stack = s.stack(m)
	s.require(stack.Counts == (counts{Delete: 1}), "Pulumi delete counts: %+v", stack.Counts)
	s.github.approve("", "", "")
	s.pulumiApply("pulumi-delete-apply")

	m, _ = s.run("pulumi-delete-converged", "preview", 0)
	stack = s.stack(m)
	s.require(stack.Counts == (counts{}), "Pulumi delete did not converge: %+v", stack.Counts)
}

func newPulumiSuite(t *testing.T) *suite {
	t.Helper()
	s := newSuiteInReport(t, "pulumi-lifecycle")
	t.Cleanup(func() {
		// Go module cache directories are read-only by default. Pulumi can
		// create one under Reeve's isolated CI home, so restore directory
		// write permission before testing.TempDir removes the fixture.
		_ = filepath.WalkDir(s.root, func(path string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				_ = os.Chmod(path, 0700)
			}
			return nil
		})
	})
	cwd, err := os.Getwd()
	s.check(err)
	repoRoot := filepath.Dir(cwd)
	s.engine = executable(t, repoRoot, *pulumiFlag)
	s.module = filepath.Join(s.root, "envs", "pulumi")
	s.check(os.MkdirAll(s.module, 0700))
	s.write(filepath.Join(s.module, "go.mod"), s.read(filepath.Join(repoRoot, "go.mod")), 0600)
	s.write(filepath.Join(s.module, "go.sum"), s.read(filepath.Join(repoRoot, "go.sum")), 0600)

	backend := filepath.Join(s.root, "pulumi-state")
	pulumiHome := filepath.Join(s.root, "pulumi-home")
	s.check(os.MkdirAll(backend, 0700))
	s.check(os.MkdirAll(pulumiHome, 0700))
	s.env = append(s.env,
		"PULUMI_HOME="+pulumiHome,
		"PULUMI_CONFIG_PASSPHRASE=reeve-e2e-passphrase",
		"PULUMI_SKIP_UPDATE_CHECK=true",
	)
	for _, name := range []string{"GOCACHE", "GOMODCACHE"} {
		// Share dependency and build caches, while the disposable HOME keeps
		// Pulumi credentials and state isolated from the developer machine.
		cmd := exec.CommandContext(t.Context(), "go", "env", name)
		value, envErr := cmd.Output()
		s.check(envErr)
		s.env = append(s.env, name+"="+strings.TrimSpace(string(value)))
	}

	binaryPath := filepath.Join(s.module, "reeve-e2e-pulumi")
	binaryJSON, err := json.Marshal(binaryPath)
	s.check(err)
	s.write(filepath.Join(s.module, "Pulumi.yaml"), []byte(fmt.Sprintf(`name: reeve-e2e-pulumi
runtime:
  name: go
  options:
    binary: %s
`, binaryJSON)), 0600)
	s.write(filepath.Join(s.module, "Pulumi.dev.yaml"), []byte("config: {}\n"), 0600)
	s.writePulumiProgram("item")

	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
	wrapper := filepath.Join(s.root, "pulumi-traced")
	s.write(wrapper, []byte("#!/bin/sh\nset -eu\nprintf '%s\\n' \"$*\" >> "+quote(s.trace)+"\nexec "+quote(s.engine)+" \"$@\"\n"), 0700)
	wrapperJSON, err := json.Marshal(wrapper)
	s.check(err)
	backendURL := "file://" + filepath.ToSlash(backend)
	backendJSON, err := json.Marshal(backendURL)
	s.check(err)
	config := filepath.Join(s.root, ".reeve")
	s.check(os.Remove(filepath.Join(config, "tofu.yaml")))
	s.write(filepath.Join(config, "pulumi.yaml"), []byte(fmt.Sprintf(`version: 1
config_type: engine
engine:
  type: pulumi
  binary:
    path: %s
  state:
    url: %s
    secrets_provider:
      type: passphrase
      passphrase: ${env:PULUMI_CONFIG_PASSPHRASE}
  plan_locking: true
  stacks:
    - project: reeve-e2e-pulumi
      path: envs/pulumi
      stacks: [dev]
  execution:
    preview_timeout: 2m
    apply_timeout: 2m
`, wrapperJSON, backendJSON)), 0600)
	s.github.edit(func(g *githubState) { g.changed = "envs/pulumi/main.go" })

	s.runPulumiSetup("login", backendURL)
	s.runPulumiSetup("stack", "init", "dev", "--cwd", s.module, "--secrets-provider", "passphrase", "--non-interactive")
	t.Logf("Pulumi: %s\nBackend: %s", s.engine, backendURL)
	return s
}

func (s *suite) writePulumiProgram(componentName string) {
	s.t.Helper()
	component := ""
	if componentName != "" {
		component = fmt.Sprintf(`
		var item thing
		if err := ctx.RegisterComponentResource("reeve:e2e:Thing", %q, &item); err != nil {
			return err
		}
`, componentName)
	}
	s.write(filepath.Join(s.module, "main.go"), []byte(fmt.Sprintf(`package main

import "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

type thing struct{ pulumi.ResourceState }

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {%s
		ctx.Export("fixture", pulumi.String(%q))
		return nil
	})
}
`, component, componentName)), 0600)
	goBinary, err := exec.LookPath("go")
	s.check(err)
	// #nosec G204 -- The test compiles its fixed disposable Pulumi fixture.
	cmd := exec.CommandContext(s.t.Context(), goBinary, "build", "-buildvcs=false", "-o", filepath.Join(s.module, "reeve-e2e-pulumi"), ".")
	cmd.Dir, cmd.Env = s.module, s.env
	output, err := cmd.CombinedOutput()
	s.require(err == nil, "Compile Pulumi fixture: %v\n%s", err, output)
}

func (s *suite) runPulumiSetup(args ...string) {
	s.t.Helper()
	// #nosec G204 -- The test invokes the configured Pulumi CLI with fixed setup arguments.
	cmd := exec.CommandContext(s.t.Context(), s.engine, args...)
	cmd.Dir, cmd.Env = s.root, s.env
	output, err := cmd.CombinedOutput()
	s.require(err == nil, "Pulumi setup failed: %v\n%s", err, output)
}

func (s *suite) pulumiApply(label string) {
	s.t.Helper()
	m, r := s.run(label, "apply", 0)
	s.require(s.stack(m).Status == "planned", "%s: unexpected apply status", label)
	s.require(commandContains(r.EngineCommands, "up ", "--plan="), "%s: Pulumi did not execute its saved plan: %v", label, r.EngineCommands)
	s.require(!hasCommand(r.EngineCommands, "preview "), "%s: saved-plan apply silently re-planned", label)
	s.require(s.appliedMarker(), "%s: missing applied marker", label)
	s.audit(r, "success")
	s.locksReleased()
}

func commandContains(commands []string, prefix, fragment string) bool {
	for _, command := range commands {
		if strings.HasPrefix(command, prefix) && strings.Contains(command, fragment) {
			return true
		}
	}
	return false
}
