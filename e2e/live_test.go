//go:build e2e

package e2e

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

var liveFlag = flag.Bool("live", false, "Enable real GitHub mutations using credentials supplied on stdin")

type liveInput struct {
	Repository      string `json:"repository"`
	AuthorToken     string `json:"author_token"`
	ReviewerToken   string `json:"reviewer_token"`
	ControllerToken string `json:"controller_token"`
	AuthorLogin     string `json:"author_login"`
	ReviewerLogin   string `json:"reviewer_login"`
	RunID           string `json:"run_id"`
}

type liveFixture struct {
	s                                 *suite
	author, reviewer, controller      *liveAPI
	branch, contentSHA, reviewerLogin string
	created                           bool
	pr                                int
	url                               string
	closed, deleted                   bool
}

const liveFixturePath = "tf/envs/lifecycle/main.tf"

func TestLiveGitHub(t *testing.T) {
	if !*liveFlag {
		t.Skip("Use the Live GitHub E2E workflow to provide scoped App tokens")
	}
	var input liveInput
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 65536)).Decode(&input); err != nil {
		t.Fatal("Expected live configuration and tokens as JSON on stdin")
	}
	if input.Repository != "reeveops/reeve-test" {
		t.Fatal("Live test is restricted to reeveops/reeve-test")
	}
	if input.AuthorToken == "" || input.ReviewerToken == "" || input.ControllerToken == "" {
		t.Fatal("Missing live tokens")
	}
	if input.AuthorToken == input.ReviewerToken || input.AuthorToken == input.ControllerToken || input.ReviewerToken == input.ControllerToken {
		t.Fatal("Live identities must use separate tokens")
	}
	loginPattern := regexp.MustCompile(`^[a-zA-Z0-9-]+\[bot\]$`)
	if !loginPattern.MatchString(input.AuthorLogin) || !loginPattern.MatchString(input.ReviewerLogin) || input.AuthorLogin == input.ReviewerLogin {
		t.Fatal("Expected different author/reviewer App logins")
	}
	if !regexp.MustCompile(`^[0-9]+-[0-9]+$`).MatchString(input.RunID) {
		t.Fatal("Expected workflow run ID and attempt")
	}
	s := newSuite(t)
	s.repo, s.actor, s.githubMode = input.Repository, input.AuthorLogin, "live-github-relay"
	f := &liveFixture{s: s, author: newLiveAPI(s.repo, input.AuthorToken), reviewer: newLiveAPI(s.repo, input.ReviewerToken), controller: newLiveAPI(s.repo, input.ControllerToken), reviewerLogin: input.ReviewerLogin}
	var random [6]byte
	_, err := rand.Read(random[:])
	s.check(err)
	f.branch = "reeve-e2e/" + input.RunID + "-" + hex.EncodeToString(random[:])
	t.Cleanup(f.cleanup)
	f.create()
	s.pr = f.pr
	relay := httptest.NewServer(&liveRelay{api: f.controller, suite: s, commentIDs: map[int64]bool{}})
	t.Cleanup(relay.Close)
	for i, value := range s.env {
		if strings.HasPrefix(value, "GITHUB_API_URL=") {
			s.env[i] = "GITHUB_API_URL=" + relay.URL + "/api/v3/"
		}
	}
	s.env = append(s.env, "GITHUB_ACTIONS=true")
	configPath := filepath.Join(s.root, ".reeve", "shared.yaml")
	config := strings.ReplaceAll(string(s.read(configPath)), reviewer, input.ReviewerLogin)
	config = strings.ReplaceAll(config, author, input.AuthorLogin)
	s.write(configPath, []byte(config), 0600)
	f.waitChecks()
	s.preview("live-create-preview", counts{Add: 1})
	s.blocked("live-missing-approval", "approvals")
	f.review("APPROVE", "APPROVED")
	s.apply("live-create-apply", false)
	s.require(len(s.resources()) == 1, "Live create did not persist one resource")
	s.preview("live-create-converged", counts{})

	source := filepath.Join(s.module, "main.tf")
	f.commit(strings.ReplaceAll(string(s.read(source)), `"initial"`, `"updated"`))
	f.waitChecks()
	s.preview("live-update-preview", counts{Change: 1})
	s.blocked("live-stale-approval", "approvals")
	f.review("REQUEST_CHANGES", "CHANGES_REQUESTED")
	s.blocked("live-changes-requested", "approvals")
	f.review("APPROVE", "APPROVED")
	s.apply("live-update-apply", false)
	var updated struct {
		Resources []struct {
			Instances []struct {
				Attributes struct{ Input struct{ Value string } }
			}
		}
	}
	s.readJSON(filepath.Join(s.module, "terraform.tfstate"), &updated)
	s.require(len(updated.Resources) == 1 && len(updated.Resources[0].Instances) == 1 && updated.Resources[0].Instances[0].Attributes.Input.Value == "updated", "Live update did not persist expected input")
	s.preview("live-update-converged", counts{})

	f.commit("terraform {}\n")
	f.waitChecks()
	s.preview("live-delete-preview", counts{Delete: 1})
	f.review("APPROVE", "APPROVED")
	s.apply("live-delete-apply", false)
	s.require(len(s.resources()) == 0, "Live deletion left resources")
	s.preview("live-delete-converged", counts{})
	s.require(len(s.results) == 12, "Expected 12 live CLI scenarios")
}

func (f *liveFixture) snapshot() error {
	data, err := json.MarshalIndent(object{
		"repository": f.s.repo, "branch": f.branch, "branch_created": f.created,
		"pr": f.pr, "url": f.url, "closed": f.closed, "deleted": f.deleted,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(f.s.report, "live.json"), append(data, '\n'), 0600)
}

func (f *liveFixture) create() {
	s := f.s
	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	s.check(f.author.call(s.t.Context(), "GET", "/git/ref/heads/master", nil, &ref))
	s.require(len(ref.Object.SHA) == 40, "Missing master SHA")
	s.check(f.author.call(s.t.Context(), "POST", "/git/refs", object{"ref": "refs/heads/" + f.branch, "sha": ref.Object.SHA}, nil))
	f.created = true
	s.check(f.snapshot())
	f.commit(string(s.read(filepath.Join(s.module, "main.tf"))))
	var pr struct {
		Number  int
		HTMLURL string `json:"html_url"`
		User    struct{ Login string }
		Head    struct{ SHA string }
	}
	s.check(f.author.call(s.t.Context(), "POST", "/pulls", object{
		"title": "Reeve E2E: disposable approval lifecycle", "head": f.branch, "base": "master",
		"body": "Automated Reeve fixture PR. The live E2E workflow closes this PR and deletes its branch after testing.",
	}, &pr))
	f.pr, f.url = pr.Number, pr.HTMLURL
	s.check(f.snapshot())
	s.require(pr.Number > 0 && pr.User.Login == s.actor && pr.Head.SHA == s.github.head(), "Fixture PR identity or head mismatch")
	s.t.Logf("Fixture PR: %s", pr.HTMLURL)
}

func (f *liveFixture) commit(source string) {
	s := f.s
	body := object{"message": "test: advance disposable Reeve fixture", "branch": f.branch, "content": base64.StdEncoding.EncodeToString([]byte(source))}
	if f.contentSHA != "" {
		body["sha"] = f.contentSHA
	}
	var result struct {
		Commit  struct{ SHA string }
		Content struct{ SHA string }
	}
	s.check(f.author.call(s.t.Context(), "PUT", "/contents/"+liveFixturePath, body, &result))
	s.require(len(result.Commit.SHA) == 40 && result.Content.SHA != "", "Commit response missing SHA")
	f.contentSHA = result.Content.SHA
	s.github.edit(func(g *githubState) { g.sha = result.Commit.SHA })
	s.write(filepath.Join(s.module, "main.tf"), []byte(source), 0600)
	if f.pr > 0 {
		ctx, cancel := context.WithTimeout(s.t.Context(), 30*time.Second)
		defer cancel()
		s.check(f.controller.waitPRHead(ctx, f.pr, result.Commit.SHA, time.Second))
	}
}

func (f *liveFixture) review(event, expected string) {
	s := f.s
	var review struct {
		ID       int64
		State    string
		CommitID string `json:"commit_id"`
		User     struct{ Login string }
	}
	s.check(f.reviewer.call(s.t.Context(), "POST", fmt.Sprintf("/pulls/%d/reviews", f.pr), object{
		"event": event, "commit_id": s.github.head(), "body": "Automated Reeve E2E review for this exact head commit.",
	}, &review))
	s.require(review.ID > 0 && review.State == expected && review.CommitID == s.github.head() && review.User.Login == f.reviewerLogin, "Review identity, state, or head mismatch")
}

func (f *liveFixture) waitChecks() {
	s := f.s
	ctx, cancel := context.WithTimeout(s.t.Context(), 2*time.Minute)
	defer cancel()
	for {
		ready, sharedGitOps := true, false
		for page := 1; ; page++ {
			var checks struct {
				Total int                                         `json:"total_count"`
				Runs  []struct{ Name, Status, Conclusion string } `json:"check_runs"`
			}
			s.check(f.controller.call(ctx, "GET", "/commits/"+s.github.head()+"/check-runs?per_page=100&page="+strconv.Itoa(page), nil, &checks))
			for _, check := range checks.Runs {
				if check.Name == "gitops" || strings.HasSuffix(check.Name, " / gitops") {
					sharedGitOps = true
				}
				if check.Status != "completed" {
					ready = false
					continue
				}
				s.require(check.Conclusion == "success" || check.Conclusion == "skipped" || check.Conclusion == "neutral", "Fixture check %s concluded %s", check.Name, check.Conclusion)
			}
			if page*100 >= checks.Total {
				break
			}
		}
		if ready && sharedGitOps {
			return
		}
		select {
		case <-ctx.Done():
			s.require(false, "Timed out waiting for fixture checks")
		case <-time.After(2 * time.Second):
		}
	}
}

func (f *liveFixture) cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if f.pr > 0 {
		if err := f.author.call(ctx, "PATCH", fmt.Sprintf("/pulls/%d", f.pr), object{"state": "closed"}, nil); err != nil {
			f.s.t.Errorf("Close fixture PR: %v", err)
		} else {
			f.closed = true
		}
	}
	if f.created {
		if err := f.author.call(ctx, "DELETE", "/git/refs/heads/"+f.branch, nil, nil); err != nil {
			f.s.t.Errorf("Delete fixture branch: %v", err)
		} else {
			f.deleted = true
		}
	}
	if err := f.snapshot(); err != nil {
		f.s.t.Errorf("Save cleanup identifiers: %v", err)
	}
}
