//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

const alternateReviewer = "reeve-e2e-alternate-reviewer[bot]"

type approvalPolicy struct {
	required      int
	approvers     []string
	codeowners    bool
	allowUnlisted bool
	breakGlass    *breakGlassPolicy
}

type breakGlassPolicy struct {
	internalList []string
	codeowners   bool
	anyone       bool
}

func yamlStrings(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = strconv.Quote(value)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func (s *suite) configureAuthorization(policy approvalPolicy) {
	s.t.Helper()
	var config strings.Builder
	fmt.Fprintf(&config, `version: 1
config_type: shared
bucket:
  type: filesystem
  name: .reeve-state
locking:
  ttl: 1m
  queue: fifo
approvals:
  sources:
    - type: pr_review
      enabled: true
  allow_unlisted_approvals_on_public: %t
  default:
    required_approvals: %d
    approvers: %s
    codeowners: %t
    dismiss_on_new_commit: true
preconditions:
  require_up_to_date: true
  require_checks_passing: true
  preview_freshness: 1h
`, policy.allowUnlisted, policy.required, yamlStrings(policy.approvers), policy.codeowners)
	if policy.breakGlass != nil {
		fmt.Fprintf(&config, `break_glass:
  authorized:
    internal_list: %s
    codeowners: %t
    anyone: %t
`, yamlStrings(policy.breakGlass.internalList), policy.breakGlass.codeowners, policy.breakGlass.anyone)
	}
	config.WriteString(`apply:
  trigger: comment
  allow_fork_prs: false
`)
	s.write(filepath.Join(s.root, ".reeve", "shared.yaml"), []byte(config.String()), 0600)
}

func (s *suite) setCodeowners(content string) {
	s.t.Helper()
	s.github.edit(func(g *githubState) { g.codeowners = content })
}

func (s *suite) commentsContain(want string) bool {
	s.t.Helper()
	s.github.mu.Lock()
	comments := append([]object(nil), s.github.state.comments...)
	s.github.mu.Unlock()
	data, err := json.Marshal(comments)
	s.check(err)
	return strings.Contains(strings.ToLower(string(data)), strings.ToLower(want))
}

func TestApprovalAuthorizationMatrix(t *testing.T) {
	cases := []struct {
		name, reviewLogin, codeowners string
		policy                        approvalPolicy
		allowed                       bool
	}{
		{
			name: "codeowners-owner", reviewLogin: reviewer,
			codeowners: "/envs/lifecycle/ @" + reviewer + "\n",
			policy:     approvalPolicy{codeowners: true}, allowed: true,
		},
		{
			name: "codeowners-non-owner", reviewLogin: alternateReviewer,
			codeowners: "/envs/lifecycle/ @" + reviewer + "\n",
			policy:     approvalPolicy{codeowners: true},
		},
		{
			name: "explicit-list-listed", reviewLogin: reviewer,
			policy: approvalPolicy{required: 1, approvers: []string{reviewer}}, allowed: true,
		},
		{
			name: "explicit-list-unlisted", reviewLogin: alternateReviewer,
			policy: approvalPolicy{required: 1, approvers: []string{reviewer}},
		},
		{
			name: "combined-list-only", reviewLogin: reviewer,
			codeowners: "/envs/lifecycle/ @" + alternateReviewer + "\n",
			policy:     approvalPolicy{required: 1, approvers: []string{reviewer}, codeowners: true},
		},
		{
			name: "combined-codeowners-only", reviewLogin: reviewer,
			codeowners: "/envs/lifecycle/ @" + reviewer + "\n",
			policy:     approvalPolicy{required: 1, approvers: []string{alternateReviewer}, codeowners: true},
		},
		{
			name: "combined-both", reviewLogin: reviewer,
			codeowners: "/envs/lifecycle/ @" + reviewer + "\n",
			policy:     approvalPolicy{required: 1, approvers: []string{reviewer}, codeowners: true}, allowed: true,
		},
		{
			name: "public-unlisted-denied", reviewLogin: alternateReviewer,
			policy: approvalPolicy{required: 1},
		},
		{
			name: "public-unlisted-opt-in", reviewLogin: alternateReviewer,
			policy: approvalPolicy{required: 1, allowUnlisted: true}, allowed: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSuiteInReport(t, "authorization-"+tc.name)
			s.newHead("approval-matrix-" + tc.name)
			s.configureAuthorization(tc.policy)
			if tc.codeowners != "" {
				s.setCodeowners(tc.codeowners)
			}
			s.preview(tc.name+"-preview", counts{Add: 1})
			s.github.approve(tc.reviewLogin, "", "")
			if tc.allowed {
				s.apply(tc.name+"-apply", false)
				s.require(len(s.resources()) == 1, "%s: allowed apply did not change state", tc.name)
				s.require(s.commentsContain("applied"), "%s: PR comment did not report apply", tc.name)
				return
			}
			s.blocked(tc.name+"-apply", "approvals")
			s.require(s.commentsContain("blocked"), "%s: PR comment did not report denial", tc.name)
		})
	}
}

func TestBreakGlassAuthorizationMatrix(t *testing.T) {
	cases := []struct {
		name, codeowners, authorizedVia string
		policy                          breakGlassPolicy
		allowed                         bool
	}{
		{
			name: "internal-list-allowed", authorizedVia: "internal_list",
			policy: breakGlassPolicy{internalList: []string{author}}, allowed: true,
		},
		{
			name:   "internal-list-denied",
			policy: breakGlassPolicy{internalList: []string{reviewer}},
		},
		{
			name: "codeowners-allowed", authorizedVia: "codeowners",
			codeowners: "/envs/lifecycle/ @" + author + "\n",
			policy:     breakGlassPolicy{codeowners: true}, allowed: true,
		},
		{
			name:       "codeowners-denied",
			codeowners: "/envs/lifecycle/ @" + reviewer + "\n",
			policy:     breakGlassPolicy{codeowners: true},
		},
		{
			name: "anyone-allowed", authorizedVia: "anyone",
			policy: breakGlassPolicy{anyone: true}, allowed: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSuiteInReport(t, "break-glass-"+tc.name)
			s.newHead("break-glass-matrix-" + tc.name)
			s.configureAuthorization(approvalPolicy{
				required: 1, approvers: []string{reviewer}, breakGlass: &tc.policy,
			})
			if tc.codeowners != "" {
				s.setCodeowners(tc.codeowners)
			}
			s.preview(tc.name+"-preview", counts{Add: 1})
			before := s.state()
			justification := "authorization matrix " + tc.name
			if !tc.allowed {
				_, r := s.runArgs(tc.name+"-apply", "apply", 1,
					"--break-glass", "--justification", justification)
				s.require(len(r.EngineCommands) == 0, "%s: denied break-glass invoked the engine", tc.name)
				s.require(reflect.DeepEqual(s.state(), before), "%s: denied break-glass changed state", tc.name)
				s.require(!s.appliedMarker(), "%s: denied break-glass wrote an applied marker", tc.name)
				log := strings.ToLower(string(s.read(filepath.Join(s.report, r.Log))))
				s.require(strings.Contains(log, "break-glass") && strings.Contains(log, "not authorized"),
					"%s: denial log did not explain break-glass authorization", tc.name)
				return
			}

			_, r := s.applyArgs(tc.name+"-apply", false,
				"--break-glass", "--justification", justification)
			s.require(len(s.resources()) == 1, "%s: break-glass apply did not change state", tc.name)
			s.assertBreakGlassAudit(r, tc.authorizedVia, justification)
			s.require(s.commentsContain("reeve:break-glass:v1"), "%s: comment lacks break-glass marker", tc.name)
			s.require(s.commentsContain(justification), "%s: comment lacks justification", tc.name)
		})
	}
}

func (s *suite) assertBreakGlassAudit(r record, authorizedVia, justification string) {
	s.t.Helper()
	auditRoot := filepath.Join(s.root, ".reeve-state", "audit")
	completion := s.files(auditRoot, r.RunID+".json")
	intent := s.files(auditRoot, r.RunID+"-intent.json")
	s.require(len(completion) == 1, "%s: expected one completion audit", r.Scenario)
	s.require(len(intent) == 1, "%s: expected one intent audit", r.Scenario)
	var entry struct {
		BreakGlass *struct {
			Justification string `json:"justification"`
			AuthorizedVia string `json:"authorized_via"`
		} `json:"break_glass"`
	}
	s.readJSON(completion[0], &entry)
	s.require(entry.BreakGlass != nil, "%s: completion audit lacks break-glass detail", r.Scenario)
	s.require(entry.BreakGlass.Justification == justification,
		"%s: audit justification mismatch", r.Scenario)
	s.require(entry.BreakGlass.AuthorizedVia == authorizedVia,
		"%s: audit source %q, want %q", r.Scenario, entry.BreakGlass.AuthorizedVia, authorizedVia)
}
