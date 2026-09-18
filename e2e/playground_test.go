//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

var playgroundFlag = flag.Bool("playground", false, "Enable the guided live playground controller")

type playgroundInput struct {
	Repository      string `json:"repository"`
	PR              int    `json:"pr"`
	Engine          string `json:"engine"`
	HeadSHA         string `json:"head_sha"`
	SessionOwner    string `json:"session_owner"`
	ControllerToken string `json:"controller_token"`
	AuthorToken     string `json:"author_token"`
	ReviewerToken   string `json:"reviewer_token"`
	ReviewerLogin   string `json:"reviewer_login"`
}

type playgroundComment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
	User struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"user"`
}

type playgroundController struct {
	s                    *suite
	controller, author   *liveAPI
	reviewer             *liveAPI
	input                playgroundInput
	progressID, lastRead int64
	stack                string
}

func TestPlaygroundCommandParser(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		body, want string
	}{
		{"  /reeve apply\r\n", "/reeve apply"},
		{"/reeve breakglass \"playground recovery\" apply\n", "/reeve breakglass \"playground recovery\" apply"},
		{"/playground help", "/playground help"},
		{"/reeve apply\nanything", ""},
		{"hello /reeve apply", ""},
		{"", ""},
	} {
		if got := parsePlaygroundCommand(tc.body); got != tc.want {
			t.Errorf("parsePlaygroundCommand(%q) = %q, want %q", tc.body, got, tc.want)
		}
	}
}

func TestNormalizePlaygroundCommand(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		command, want string
	}{
		{"/reeve up", "/reeve apply"},
		{"/reeve plan", "/reeve preview"},
		{"/reeve apply", "/reeve apply"},
		{"/reeve explain playground/default", "/reeve explain playground/default"},
		{"/playground approve", "/playground approve"},
	} {
		if got := normalizePlaygroundCommand(tc.command); got != tc.want {
			t.Errorf("normalizePlaygroundCommand(%q) = %q, want %q", tc.command, got, tc.want)
		}
	}
}

func TestPlaygroundCommandAccepted(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		command      string
		allowed      []string
		wantCommand  string
		wantAccepted bool
	}{
		{"apply alias", "/reeve up", []string{"/reeve apply"}, "/reeve apply", true},
		{"wrong stage", "/reeve up", []string{"/reeve explain playground/default"}, "/reeve apply", false},
		{"exact command", "/playground approve", []string{"/playground approve"}, "/playground approve", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command, accepted := playgroundCommandAccepted(tc.command, tc.allowed...)
			if command != tc.wantCommand || accepted != tc.wantAccepted {
				t.Errorf("playgroundCommandAccepted(%q) = (%q, %t), want (%q, %t)",
					tc.command, command, accepted, tc.wantCommand, tc.wantAccepted)
			}
		})
	}
}

func parsePlaygroundCommand(body string) string {
	command := strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n"))
	if command == "" || strings.Contains(command, "\n") {
		return ""
	}
	if !strings.HasPrefix(command, "/reeve ") && !strings.HasPrefix(command, "/playground ") {
		return ""
	}
	return command
}

func normalizePlaygroundCommand(command string) string {
	switch command {
	case "/reeve up":
		return "/reeve apply"
	case "/reeve plan":
		return "/reeve preview"
	default:
		return command
	}
}

func playgroundCommandAccepted(command string, allowed ...string) (string, bool) {
	command = normalizePlaygroundCommand(command)
	for _, candidate := range allowed {
		if command == normalizePlaygroundCommand(candidate) {
			return command, true
		}
	}
	return command, false
}

func TestPlaygroundGitHub(t *testing.T) {
	if !*playgroundFlag {
		t.Skip("Use the Guided Reeve Playground workflow")
	}
	var input playgroundInput
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 65536)).Decode(&input); err != nil {
		t.Fatalf("Expected playground configuration on stdin: %v", err)
	}
	validatePlaygroundInput(t, input)

	var s *suite
	switch input.Engine {
	case "opentofu":
		s = newHCLSuiteInReport(t, "playground-opentofu", "tofu", "OpenTofu", *engineFlag)
	case "terraform":
		s = newHCLSuiteInReport(t, "playground-terraform", "terraform", "Terraform", *terraformFlag)
	case "pulumi":
		s = newPulumiSuiteInReport(t, "playground-pulumi")
	default:
		t.Fatalf("Unsupported playground engine %q", input.Engine)
	}

	s.repo, s.pr, s.actor, s.githubMode = input.Repository, input.PR, input.SessionOwner, "guided-playground"
	s.github.edit(func(g *githubState) { g.sha = input.HeadSHA })
	configurePlaygroundFixture(s, input.Engine)

	p := &playgroundController{
		s: s, input: input, stack: "playground/default",
		controller: newLiveAPI(input.Repository, input.ControllerToken),
		author:     newLiveAPI(input.Repository, input.AuthorToken),
		reviewer:   newLiveAPI(input.Repository, input.ReviewerToken),
	}
	if input.Engine == "pulumi" {
		p.stack = "reeve-e2e-pulumi/dev"
	}
	p.validateOwnedPR()
	p.attachRelay()
	p.captureExistingComments()
	p.runTour()
}

func TestPlaygroundEngineShowcase(t *testing.T) {
	for _, tc := range []struct {
		name, engine string
		newSuite     func(*testing.T) *suite
	}{
		{"opentofu", "opentofu", func(t *testing.T) *suite {
			return newHCLSuiteInReport(t, "playground-showcase-opentofu", "tofu", "OpenTofu", *engineFlag)
		}},
		{"terraform", "terraform", func(t *testing.T) *suite {
			return newHCLSuiteInReport(t, "playground-showcase-terraform", "terraform", "Terraform", *terraformFlag)
		}},
		{"pulumi", "pulumi", func(t *testing.T) *suite {
			return newPulumiSuiteInReport(t, "playground-showcase-pulumi")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.newSuite(t)
			configurePlaygroundFixture(s, tc.engine)
			stack := "playground/default"
			if tc.engine == "pulumi" {
				stack = "reeve-e2e-pulumi/dev"
			}
			s.newHead(tc.name + "-playground-initial")
			m, _ := s.run(tc.name+"-playground-initial-preview", "preview", 0)
			initial := s.stack(m)
			s.require(initial.Counts.Add > 0, "Initial playground preview has no additions: %+v", initial.Counts)
			s.runArgs(tc.name+"-playground-explain", "explain", 0, "--stack", stack)
			s.github.approve("", "", "")
			if tc.engine == "pulumi" {
				s.pulumiApply(tc.name + "-playground-initial-apply")
			} else {
				s.apply(tc.name+"-playground-initial-apply", false)
			}

			s.newHead(tc.name + "-playground-varied")
			if tc.engine == "pulumi" {
				writePulumiShowcase(s, "updated")
			} else {
				writeHCLShowcase(s, "updated")
			}
			m, _ = s.run(tc.name+"-playground-varied-preview", "preview", 0)
			varied := s.stack(m).Counts
			s.require(varied.Add > 0 && varied.Change > 0 && varied.Delete > 0 && varied.Replace > 0,
				"Varied playground preview did not show every operation: %+v", varied)
			s.github.approve("", "", "")
			controller := &playgroundController{s: s, stack: stack}
			controller.seedLock()
			controller.blockedByExistingLock()
			controller.clearLocks()
		})
	}
}

func validatePlaygroundInput(t *testing.T, input playgroundInput) {
	t.Helper()
	if input.Repository != "reeveops/reeve-test" || input.PR <= 0 {
		t.Fatal("Playground is restricted to an owned reeve-test PR")
	}
	if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(input.HeadSHA) ||
		!regexp.MustCompile(`^[A-Za-z0-9-]+$`).MatchString(input.SessionOwner) {
		t.Fatal("Invalid playground head or session owner")
	}
	if input.ControllerToken == "" || input.AuthorToken == "" || input.ReviewerToken == "" ||
		input.ControllerToken == input.AuthorToken || input.AuthorToken == input.ReviewerToken {
		t.Fatal("Playground requires distinct scoped credentials")
	}
	if input.ReviewerLogin != reviewer {
		t.Fatalf("Reviewer App login %q does not match CODEOWNERS %q", input.ReviewerLogin, reviewer)
	}
}

func configurePlaygroundFixture(s *suite, engine string) {
	s.t.Helper()
	originalModule := s.module
	playgroundModule := filepath.Join(s.root, "playground")
	s.check(os.Rename(originalModule, playgroundModule))
	s.module = playgroundModule
	if engine == "pulumi" {
		project := filepath.Join(s.module, "Pulumi.yaml")
		contents := strings.Replace(string(s.read(project)), originalModule, s.module, 1)
		s.write(project, []byte(contents), 0600)
	}

	engineConfig := filepath.Join(s.root, ".reeve", engine+".yaml")
	if engine == "opentofu" {
		engineConfig = filepath.Join(s.root, ".reeve", "tofu.yaml")
	}
	config := string(s.read(engineConfig))
	config = strings.Replace(config, "path: envs/lifecycle", "path: playground", 1)
	config = strings.Replace(config, "path: envs/pulumi", "path: playground", 1)
	s.require(strings.Contains(config, "path: playground"), "Playground engine path was not configured")
	s.write(engineConfig, []byte(config), 0600)

	if engine == "pulumi" {
		writePulumiShowcase(s, "initial")
	} else {
		writeHCLShowcase(s, "initial")
	}
	s.github.edit(func(g *githubState) { g.changed = "playground/" + engine + ".yaml" })

	path := filepath.Join(s.root, ".reeve", "shared.yaml")
	shared := strings.Replace(string(s.read(path)), "require_checks_passing: true", "require_checks_passing: false", 1) + `
break_glass:
  authorized:
    internal_list: []
    codeowners: false
    anyone: true
  override_freeze: true
`
	s.write(path, []byte(shared), 0600)
}

func (p *playgroundController) validateOwnedPR() {
	p.s.t.Helper()
	var pr struct {
		State string `json:"state"`
		Body  string `json:"body"`
		User  struct {
			Login string `json:"login"`
		} `json:"user"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
		Head struct {
			SHA  string `json:"sha"`
			Ref  string `json:"ref"`
			Repo struct {
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"head"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	p.s.check(p.controller.call(p.s.t.Context(), http.MethodGet, fmt.Sprintf("/pulls/%d", p.input.PR), nil, &pr))
	labelled := false
	for _, label := range pr.Labels {
		labelled = labelled || label.Name == "reeve-playground"
	}
	p.s.require(pr.State == "open" && pr.Base.Ref == "master" && pr.Head.SHA == p.input.HeadSHA,
		"Playground PR state, base, or head changed")
	p.s.require(pr.User.Login == author, "Playground PR was not created by the author App")
	p.s.require(pr.Head.Repo.FullName == p.input.Repository && strings.HasPrefix(pr.Head.Ref, "reeve-playground/"),
		"Playground PR branch is not owned")
	p.s.require(labelled && strings.Contains(pr.Body, "<!-- reeve:playground-session:v1 -->"),
		"Playground ownership markers are missing")
}

func (p *playgroundController) attachRelay() {
	p.s.t.Helper()
	relay := httptestNewServer(p)
	p.s.t.Cleanup(relay.Close)
	for i, value := range p.s.env {
		if strings.HasPrefix(value, "GITHUB_API_URL=") {
			p.s.env[i] = "GITHUB_API_URL=" + relay.URL + "/api/v3/"
		}
	}
	p.s.env = append(p.s.env, "GITHUB_ACTIONS=true")
}

// httptestNewServer is kept small so the live relay remains the only API
// surface available to the Reeve child process.
func httptestNewServer(p *playgroundController) *httptest.Server {
	return httptest.NewServer(&liveRelay{api: p.controller, suite: p.s, commentIDs: map[int64]bool{}})
}

func (p *playgroundController) captureExistingComments() {
	for _, comment := range p.comments() {
		if comment.ID > p.lastRead {
			p.lastRead = comment.ID
		}
	}
}

func (p *playgroundController) comments() []playgroundComment {
	p.s.t.Helper()
	var comments []playgroundComment
	p.s.check(p.controller.call(p.s.t.Context(), http.MethodGet,
		fmt.Sprintf("/issues/%d/comments?per_page=100", p.input.PR), nil, &comments))
	return comments
}

func (p *playgroundController) progress(stage, instruction string) {
	p.s.t.Helper()
	body := fmt.Sprintf("<!-- reeve:playground-progress:v1 -->\n## Reeve playground · %s\n\nEngine: **%s**\n\n%s\n\nOnly commands from @%s advance this session. Use `/playground help` to repeat this instruction or `/playground finish` to stop.\n",
		stage, p.input.Engine, instruction, p.input.SessionOwner)
	if p.progressID == 0 {
		var created struct {
			ID int64 `json:"id"`
		}
		p.s.check(p.controller.call(p.s.t.Context(), http.MethodPost,
			fmt.Sprintf("/issues/%d/comments", p.input.PR), object{"body": body}, &created))
		p.s.require(created.ID > 0, "Progress comment ID missing")
		p.progressID = created.ID
		return
	}
	p.s.check(p.controller.call(p.s.t.Context(), http.MethodPatch,
		fmt.Sprintf("/issues/comments/%d", p.progressID), object{"body": body}, nil))
}

func (p *playgroundController) wait(stage, instruction string, allowed ...string) string {
	p.s.t.Helper()
	p.progress(stage, instruction)
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		for _, comment := range p.comments() {
			if comment.ID <= p.lastRead {
				continue
			}
			p.lastRead = comment.ID
			if comment.User.Login != p.input.SessionOwner || comment.User.Type == "Bot" {
				continue
			}
			command, accepted := playgroundCommandAccepted(parsePlaygroundCommand(comment.Body), allowed...)
			switch command {
			case "/playground help":
				p.progress(stage, instruction)
			case "/playground finish":
				p.progress("Finished", "The session was stopped. Cleanup is closing this PR.")
				return command
			default:
				if accepted {
					return command
				}
				if command != "" {
					p.progress(stage, "That command does not advance this stage. "+instruction)
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	p.s.require(false, "Playground was idle for five minutes")
	return ""
}

func (p *playgroundController) runTour() {
	p.s.t.Helper()
	initial, _ := p.s.run("playground-preview", "preview", 0)
	p.s.require(p.s.stack(initial).Counts.Add > 0, "Playground preview did not contain additions")
	command := p.wait("Preview", fmt.Sprintf("Inspect Reeve's preview comment. Run `/reeve explain %s` for the decision trace, or `/reeve apply` to continue.", p.stack),
		"/reeve explain "+p.stack, "/reeve apply")
	if command == "/playground finish" {
		return
	}
	if command != "/reeve apply" {
		p.s.runArgs("playground-explain", "explain", 0, "--stack", p.stack)
		if p.wait("Denied apply", "Run `/reeve apply`. Approval policy should block it before the engine runs.", "/reeve apply") == "/playground finish" {
			return
		}
	}
	p.s.blocked("playground-denied", "approvals")

	if p.wait("Approval", "Run `/playground approve` to ask the reviewer App for an approval on this exact head.", "/playground approve") == "/playground finish" {
		return
	}
	p.review("APPROVE", "APPROVED")
	if p.wait("Approved apply", "The review is recorded. Run `/reeve apply` again.", "/reeve apply") == "/playground finish" {
		return
	}
	p.apply("playground-approved", false)

	if p.wait("Idempotency", "Run `/reeve apply` once more. Reeve should recognize that this exact commit was already applied.", "/reeve apply") == "/playground finish" {
		return
	}
	_, repeated := p.s.run("playground-already-applied", "apply", 0)
	p.s.require(len(repeated.EngineCommands) == 0, "Already-applied playground run executed the engine")

	p.advance("changes-requested")
	p.writeUpdate()
	updated, _ := p.s.run("playground-update-preview", "preview", 0)
	counts := p.s.stack(updated).Counts
	p.s.require(counts.Add > 0 && counts.Change > 0 && counts.Delete > 0 && counts.Replace > 0,
		"Playground update did not contain add, change, delete, and replace: %+v", counts)
	if p.wait("Changes requested", "Run `/playground request-changes`, then try the apply when prompted.", "/playground request-changes") == "/playground finish" {
		return
	}
	p.review("REQUEST_CHANGES", "CHANGES_REQUESTED")
	if p.wait("Changes requested", "Run `/reeve apply`. The latest review should deny it.", "/reeve apply") == "/playground finish" {
		return
	}
	p.s.blocked("playground-changes-requested", "approvals")
	p.review("APPROVE", "APPROVED")

	p.seedLock()
	if p.wait("Stack lock", "The controller seeded a synthetic holder on the selected stack. Run `/reeve apply` to see lock blocking.", "/reeve apply") == "/playground finish" {
		return
	}
	p.blockedByExistingLock()
	p.clearLocks()

	p.advance("engine-error")
	p.writeFailure()
	p.s.run("playground-failure-preview", "preview", 0)
	p.review("APPROVE", "APPROVED")
	if p.wait("Engine failure", "Run `/reeve apply`. The trusted fixture fails locally so you can inspect Reeve's diagnostics.", "/reeve apply") == "/playground finish" {
		return
	}
	p.apply("playground-engine-failure", true)

	p.advance("break-glass")
	p.writeRecovery()
	p.s.run("playground-recovery-preview", "preview", 0)
	if p.wait("Break-glass", "Normal approval is absent on the new head. Run `/reeve breakglass \"playground recovery\" apply`.",
		"/reeve breakglass \"playground recovery\" apply") == "/playground finish" {
		return
	}
	p.applyBreakGlass()
	p.s.run("playground-converged", "preview", 0)
	p.progress("Complete", "You exercised preview, explain, denial, approval, idempotency, changes requested, lock blocking, engine failure, break-glass, and convergence. Cleanup is closing this PR.")
}

func (p *playgroundController) review(event, expected string) {
	var review struct {
		ID       int64  `json:"id"`
		State    string `json:"state"`
		CommitID string `json:"commit_id"`
		User     struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	p.s.check(p.reviewer.call(p.s.t.Context(), http.MethodPost, fmt.Sprintf("/pulls/%d/reviews", p.input.PR), object{
		"event": event, "commit_id": p.s.github.head(), "body": "Guided Reeve playground review for this exact head.",
	}, &review))
	p.s.require(review.ID > 0 && review.State == expected && review.CommitID == p.s.github.head() && review.User.Login == p.input.ReviewerLogin,
		"Playground review identity, state, or head mismatch")
}

func (p *playgroundController) advance(label string) {
	var pr struct {
		Head struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
	}
	p.s.check(p.author.call(p.s.t.Context(), http.MethodGet, fmt.Sprintf("/pulls/%d", p.input.PR), nil, &pr))
	var commit struct {
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
	}
	p.s.check(p.author.call(p.s.t.Context(), http.MethodGet, "/git/commits/"+pr.Head.SHA, nil, &commit))
	var created struct {
		SHA string `json:"sha"`
	}
	p.s.check(p.author.call(p.s.t.Context(), http.MethodPost, "/git/commits", object{
		"message": "test: advance playground to " + label, "tree": commit.Tree.SHA, "parents": []string{pr.Head.SHA},
	}, &created))
	p.s.require(regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(created.SHA), "Playground commit response missing SHA")
	p.s.check(p.author.call(p.s.t.Context(), http.MethodPatch, "/git/refs/heads/"+pr.Head.Ref, object{"sha": created.SHA, "force": false}, nil))
	ctx, cancel := context.WithTimeout(p.s.t.Context(), 30*time.Second)
	defer cancel()
	p.s.check(p.controller.waitPRHead(ctx, p.input.PR, created.SHA, time.Second))
	p.input.HeadSHA = created.SHA
	p.s.github.edit(func(g *githubState) { g.sha = created.SHA })
}

func (p *playgroundController) writeUpdate() {
	if p.input.Engine == "pulumi" {
		writePulumiShowcase(p.s, "updated")
		return
	}
	writeHCLShowcase(p.s, "updated")
}

func (p *playgroundController) writeFailure() {
	if p.input.Engine == "pulumi" {
		p.writePulumiFailure()
		return
	}
	p.s.write(filepath.Join(p.s.module, "main.tf"), []byte(`terraform {}
resource "terraform_data" "fails" {
  provisioner "local-exec" {
    command = "echo simulated-playground-failure >&2; exit 1"
  }
}
`), 0600)
}

func (p *playgroundController) writeRecovery() {
	if p.input.Engine == "pulumi" {
		writePulumiShowcase(p.s, "recovered")
		return
	}
	p.s.write(filepath.Join(p.s.module, "main.tf"), []byte(`terraform {}
resource "terraform_data" "item" {
  input = "recovered"
}
`), 0600)
}

func writeHCLShowcase(s *suite, stage string) {
	s.t.Helper()
	source := `terraform {}

resource "terraform_data" "change" {
  input = "before"
}

resource "terraform_data" "remove" {
  input = "retired"
}

resource "terraform_data" "replace" {
  input            = "rotated"
  triggers_replace = "before"
}
`
	if stage == "updated" {
		source = `terraform {}

resource "terraform_data" "add" {
  input = "new"
}

resource "terraform_data" "change" {
  input = "after"
}

resource "terraform_data" "replace" {
  input            = "rotated"
  triggers_replace = "after"
}
`
	}
	s.write(filepath.Join(s.module, "main.tf"), []byte(source), 0600)
}

func writePulumiShowcase(s *suite, stage string) {
	s.t.Helper()
	resources := `
		_, err := local.NewCommand(ctx, "change", &local.CommandArgs{
			Create: pulumi.String("printf created"),
			Update: pulumi.String("printf updated"),
			Delete: pulumi.String("printf deleted"),
			Environment: pulumi.StringMap{"VERSION": pulumi.String("before")},
		})
		if err != nil { return err }
		_, err = local.NewCommand(ctx, "remove", &local.CommandArgs{
			Create: pulumi.String("printf created"),
			Delete: pulumi.String("printf deleted"),
		})
		if err != nil { return err }
		_, err = local.NewCommand(ctx, "replace", &local.CommandArgs{
			Create: pulumi.String("printf created"),
			Delete: pulumi.String("printf deleted"),
			Triggers: pulumi.Array{pulumi.String("before")},
		})
		return err
`
	if stage == "updated" {
		resources = `
		_, err := local.NewCommand(ctx, "add", &local.CommandArgs{
			Create: pulumi.String("printf created"),
			Delete: pulumi.String("printf deleted"),
		})
		if err != nil { return err }
		_, err = local.NewCommand(ctx, "change", &local.CommandArgs{
			Create: pulumi.String("printf created"),
			Update: pulumi.String("printf updated"),
			Delete: pulumi.String("printf deleted"),
			Environment: pulumi.StringMap{"VERSION": pulumi.String("after")},
		})
		if err != nil { return err }
		_, err = local.NewCommand(ctx, "replace", &local.CommandArgs{
			Create: pulumi.String("printf created"),
			Delete: pulumi.String("printf deleted"),
			Triggers: pulumi.Array{pulumi.String("after")},
		})
		return err
`
	}
	if stage == "recovered" {
		resources = `
		_, err := local.NewCommand(ctx, "recovered", &local.CommandArgs{
			Create: pulumi.String("printf recovered"),
			Delete: pulumi.String("printf deleted"),
		})
		return err
`
	}
	program := fmt.Sprintf(`package main

import (
	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {%s
	})
}
`, resources)
	s.write(filepath.Join(s.module, "main.go"), []byte(program), 0600)
	compilePulumiPlayground(s)
}

func compilePulumiPlayground(s *suite) {
	s.t.Helper()
	goBinary, err := exec.LookPath("go")
	s.check(err)
	cmd := exec.CommandContext(s.t.Context(), goBinary, "build", "-buildvcs=false", "-o", filepath.Join(s.module, "reeve-e2e-pulumi"), ".")
	cmd.Dir, cmd.Env = s.module, s.env
	output, err := cmd.CombinedOutput()
	s.require(err == nil, "Compile Pulumi playground fixture: %v\n%s", err, output)
}

func (p *playgroundController) writePulumiFailure() {
	program := `package main

import (
	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		_, err := local.NewCommand(ctx, "fails", &local.CommandArgs{
			Create: pulumi.String("echo simulated-playground-failure >&2; exit 1"),
		})
		return err
	})
}
`
	p.s.write(filepath.Join(p.s.module, "main.go"), []byte(program), 0600)
	compilePulumiPlayground(p.s)
}

func (p *playgroundController) seedLock() {
	path := filepath.Join(p.s.root, ".reeve-state", "locks", strings.Split(p.stack, "/")[0], strings.Split(p.stack, "/")[1]+".json")
	p.s.check(os.MkdirAll(filepath.Dir(path), 0700))
	now := time.Now().UTC()
	lock := object{
		"project": strings.Split(p.stack, "/")[0], "stack": strings.Split(p.stack, "/")[1],
		"holder": object{"pr": 999, "commit_sha": strings.Repeat("9", 40), "run_id": "playground-holder", "actor": "playground-lock",
			"acquired_at": now.Format(time.RFC3339), "expires_at": now.Add(10 * time.Minute).Format(time.RFC3339)},
		"queue": []any{}, "updated_at": now.Format(time.RFC3339),
	}
	data, err := json.Marshal(lock)
	p.s.check(err)
	p.s.write(path, data, 0600)
}

func (p *playgroundController) clearLocks() {
	p.s.check(os.RemoveAll(filepath.Join(p.s.root, ".reeve-state", "locks")))
}

func (p *playgroundController) blockedByExistingLock() {
	p.s.t.Helper()
	before := p.s.state()
	m, r := p.s.run("playground-lock-blocked", "apply", 0)
	stack := p.s.stack(m)
	denied := false
	for _, gate := range stack.Gates {
		denied = denied || (gate.Gate == "lock_acquirable" && gate.Outcome == "fail")
	}
	p.s.require(stack.Status == "blocked" && denied, "Existing playground lock did not deny apply")
	p.s.require(len(r.EngineCommands) == 0, "Engine executed despite the existing playground lock")
	p.s.require(reflect.DeepEqual(p.s.state(), before), "Engine state changed despite the existing playground lock")
	p.s.audit(r, "blocked")

	path := filepath.Join(p.s.root, ".reeve-state", "locks", strings.Split(p.stack, "/")[0], strings.Split(p.stack, "/")[1]+".json")
	var lock struct {
		Holder struct {
			RunID string `json:"run_id"`
		} `json:"holder"`
	}
	p.s.readJSON(path, &lock)
	p.s.require(lock.Holder.RunID == "playground-holder", "Blocked apply changed the existing playground lock")
}

func (p *playgroundController) apply(label string, failed bool) {
	expected, status, outcome := 0, "planned", "success"
	if failed {
		expected, status, outcome = 1, "error", "failed"
	}
	m, r := p.s.run(label, "apply", expected)
	stack := p.s.stack(m)
	p.s.require(stack.Status == status, "%s: status %s, want %s", label, stack.Status, status)
	p.s.require(len(r.EngineCommands) > 0, "%s: engine did not run", label)
	p.s.audit(r, outcome)
	p.s.locksReleased()
}

func (p *playgroundController) applyBreakGlass() {
	m, r := p.s.runArgs("playground-break-glass", "apply", 0,
		"--break-glass", "--justification", "playground recovery")
	p.s.require(p.s.stack(m).Status == "planned", "Break-glass did not apply")
	p.s.require(len(r.EngineCommands) > 0, "Break-glass did not run the engine")
	p.s.audit(r, "success")
	p.s.locksReleased()
}
