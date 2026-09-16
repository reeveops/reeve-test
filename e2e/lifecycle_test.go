//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLifecycle(t *testing.T) {
	s := newSuite(t)
	s.noOpPreviewSkipsStateAuth()
	s.newHead("create")
	s.preview("create-preview", counts{Add: 1})
	s.blocked("missing-approval", "approvals")
	for _, review := range []struct{ label, login, sha, state string }{
		{label: "self-approval", login: author},
		{label: "unlisted-approval", login: "unlisted-reviewer[bot]"},
		{label: "stale-approval", sha: strings.Repeat("c", 40)},
		{label: "changes-requested", state: "CHANGES_REQUESTED"},
	} {
		s.github.approve(review.login, review.sha, review.state)
		s.blocked(review.label, "approvals")
	}
	s.github.approve("", "", "")
	for _, denied := range []struct {
		label, gate     string
		mutate, restore func(*githubState)
	}{
		{"failed-check", "checks_green", func(g *githubState) { g.check = "failure" }, func(g *githubState) { g.check = "success" }},
		{"behind-base", "up_to_date", func(g *githubState) { g.behind = 1 }, func(g *githubState) { g.behind = 0 }},
		{"draft-pr", "not_draft_pr", func(g *githubState) { g.draft = true }, func(g *githubState) { g.draft = false }},
		{"fork-pr", "fork_pr_policy", func(g *githubState) { g.fork = true }, func(g *githubState) { g.fork = false }},
	} {
		s.github.edit(denied.mutate)
		s.blocked(denied.label, denied.gate)
		s.github.edit(denied.restore)
	}
	s.github.edit(func(g *githubState) { g.reviewError = true })
	before := s.state()
	_, r := s.run("approval-api-outage", "apply", 1)
	s.require(len(r.EngineCommands) == 0 && reflect.DeepEqual(s.state(), before), "Approval outage did not fail closed")
	s.require(!s.appliedMarker(), "Approval outage wrote an applied marker")
	s.github.edit(func(g *githubState) { g.reviewError = false })
	s.apply("create-apply", false)
	s.require(len(s.resources()) == 1, "Create did not persist a resource")

	before = s.state()
	_, r = s.run("already-applied", "apply", 0)
	s.require(len(r.EngineCommands) == 0 && reflect.DeepEqual(s.state(), before), "Already-applied guard executed the engine")
	s.preview("converged-preview", counts{})

	s.newHead("update")
	source := filepath.Join(s.module, "main.tf")
	s.write(source, []byte(strings.ReplaceAll(string(s.read(source)), `"initial"`, `"updated"`)), 0600)
	s.preview("update-preview", counts{Change: 1})
	s.blocked("approval-invalidated-by-commit", "approvals")
	s.github.approve("", "", "")
	s.apply("update-apply", false)
	var state struct {
		Resources []struct {
			Instances []struct {
				Attributes struct {
					Input struct{ Value string }
				}
			}
		}
	}
	s.readJSON(filepath.Join(s.module, "terraform.tfstate"), &state)
	s.require(len(state.Resources) == 1 && len(state.Resources[0].Instances) == 1, "Update did not persist one resource instance")
	s.require(state.Resources[0].Instances[0].Attributes.Input.Value == "updated", "Resource input was not updated")
	s.preview("update-converged", counts{})

	s.newHead("delete")
	s.write(source, []byte("terraform {}\n"), 0600)
	s.preview("delete-preview", counts{Delete: 1})
	s.github.approve("", "", "")
	s.apply("delete-apply", false)
	s.require(len(s.resources()) == 0, "Delete left managed resources")
	s.preview("delete-converged", counts{})

	s.newHead("engine-failure")
	s.write(source, []byte(`terraform {}
resource "terraform_data" "fails" {
  provisioner "local-exec" {
    command = "echo simulated-e2e-failure >&2; exit 1"
  }
}
`), 0600)
	s.preview("failure-preview", counts{Add: 1})
	s.github.approve("", "", "")
	s.apply("failed-apply", true)
	reported := false
	s.github.edit(func(g *githubState) {
		for _, comment := range g.comments {
			body, _ := comment["body"].(string)
			if strings.Contains(body, "simulated-e2e-failure") {
				reported = true
			}
		}
	})
	s.require(reported, "Failed apply was not reported on the simulated PR")

	s.newHead("invalid-source")
	s.write(source, []byte("terraform { invalid syntax !!!\n"), 0600)
	m, _ := s.run("failed-preview", "preview", 1)
	s.require(s.stack(m).Status == "error", "Failed preview was not persisted")
	s.require(!s.appliedMarker(), "Failed preview wrote an applied marker")
	s.require(len(s.results) == 25, "Expected 25 command scenarios, got %d", len(s.results))
}

func (s *suite) noOpPreviewSkipsStateAuth() {
	s.t.Helper()
	enginePath := filepath.Join(s.root, ".reeve", "tofu.yaml")
	authPath := filepath.Join(s.root, ".reeve", "auth.yaml")
	sharedPath := filepath.Join(s.root, ".reeve", "shared.yaml")
	originalEngine := s.read(enginePath)
	originalShared := s.read(sharedPath)
	configuredEngine := strings.Replace(string(originalEngine), "engine:\n", "engine:\n  state:\n    auth_provider: missing-state\n", 1)
	configuredShared := strings.Replace(string(originalShared),
		"bucket:\n  type: filesystem\n  name: .reeve-state",
		"bucket:\n  type: gcs\n  name: reeve-e2e-must-not-open", 1)
	s.write(enginePath, []byte(configuredEngine), 0600)
	s.write(sharedPath, []byte(configuredShared), 0600)
	s.write(authPath, []byte(`version: 1
config_type: auth
providers:
  missing-state:
    type: github_secret
    env_var: REEVE_E2E_MISSING_STATE_TOKEN
`), 0600)
	s.t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", filepath.Join(s.root, "missing-gcp-credentials.json"))
	s.github.edit(func(g *githubState) { g.changed = "README.md" })
	defer func() {
		s.write(enginePath, originalEngine, 0600)
		s.write(sharedPath, originalShared, 0600)
		s.check(os.Remove(authPath))
		s.github.edit(func(g *githubState) { g.changed = "envs/lifecycle/main.tf" })
	}()

	s.newHead("docs-only-no-auth")
	requestKey := "GET /repos/" + repository + "/pulls/1"
	prReadsBefore := 0
	s.github.edit(func(g *githubState) { prReadsBefore = g.requests[requestKey] })
	m, r := s.run("docs-only-no-auth", "preview", 0)
	s.require(m == nil, "Docs-only preview persisted a manifest after skipping blob storage")
	s.require(len(r.EngineCommands) == 0, "Docs-only preview invoked the engine: %v", r.EngineCommands)
	reported := false
	s.github.edit(func(g *githubState) {
		for _, comment := range g.comments {
			body, _ := comment["body"].(string)
			if strings.Contains(body, "Documentation/asset-only changes") {
				reported = true
			}
		}
	})
	s.require(reported, "Docs-only preview did not report its result on the PR")
	prReadsAfter := 0
	s.github.edit(func(g *githubState) { prReadsAfter = g.requests[requestKey] })
	s.require(prReadsAfter-prReadsBefore == 1, "Preview fetched PR metadata %d times, expected one coherent snapshot", prReadsAfter-prReadsBefore)
}

func (s *suite) resources() []any {
	resources, ok := s.state()["resources"].([]any)
	s.require(ok, "Engine state has no resources array")
	return resources
}
