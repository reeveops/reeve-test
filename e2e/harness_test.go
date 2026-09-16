//go:build e2e

package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

var (
	reeveFlag     = flag.String("reeve", "../reeve/bin/reeve", "Reeve binary, relative to repository root")
	engineFlag    = flag.String("engine", "tofu", "OpenTofu executable or path relative to repository root")
	terraformFlag = flag.String("terraform", "terraform", "Terraform executable or path relative to repository root")
	pulumiFlag    = flag.String("pulumi", "pulumi", "Pulumi executable or path relative to repository root")
	reportFlag    = flag.String("report-dir", "", "New report directory, relative to repository root")
)

var reportRoots = struct {
	sync.Mutex
	created map[string]bool
}{created: map[string]bool{}}

type counts struct{ Add, Change, Delete, Replace int }
type gate struct{ Gate, Outcome string }
type stackResult struct {
	Counts          counts
	Status, PlanKey string
	Gates           []gate
}
type manifest struct {
	Stacks []stackResult `json:"stacks"`
}
type record struct {
	Scenario       string         `json:"scenario"`
	ExitCode       int            `json:"exit_code"`
	Seconds        float64        `json:"seconds"`
	Log            string         `json:"log"`
	EngineCommands []string       `json:"engine_commands"`
	APIRequests    map[string]int `json:"api_requests"`
	Manifest       bool           `json:"manifest"`
	RunID          string         `json:"run_id"`
}
type suite struct {
	t                                          *testing.T
	root, report, reeve, engine, module, trace string
	env                                        []string
	github                                     *githubFixture
	results                                    []record
	failure                                    string
	repo, actor, githubMode                    string
	pr                                         int
}

func (s *suite) require(ok bool, format string, args ...any) {
	s.t.Helper()
	if !ok {
		s.failure = fmt.Sprintf(format, args...)
		s.t.Fatal(s.failure)
	}
}

func (s *suite) check(err error) {
	s.t.Helper()
	s.require(err == nil, "%v", err)
}

func (s *suite) write(path string, data []byte, mode fs.FileMode) {
	s.t.Helper()
	s.check(os.WriteFile(path, data, mode))
}

func (s *suite) read(path string) []byte {
	s.t.Helper()
	data, err := os.ReadFile(path)
	s.check(err)
	return data
}

func (s *suite) readJSON(path string, dest any) {
	s.t.Helper()
	s.check(json.Unmarshal(s.read(path), dest))
}

func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func absolute(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func executable(t *testing.T, root, value string) string {
	t.Helper()
	if strings.ContainsRune(value, filepath.Separator) {
		value = absolute(root, value)
	}
	path, err := exec.LookPath(value)
	if err != nil {
		t.Fatalf("Executable unavailable: %v", err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func newSuite(t *testing.T) *suite {
	return newSuiteInReport(t, "")
}

func newSuiteInReport(t *testing.T, reportSubdir string) *suite {
	return newHCLSuiteInReport(t, reportSubdir, "tofu", "OpenTofu", *engineFlag)
}

func newHCLSuiteInReport(t *testing.T, reportSubdir, engineType, engineName, engineValue string) *suite {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Dir(cwd)
	s := &suite{t: t, github: newGitHub(), results: []record{}, repo: repository, actor: author, pr: 1, githubMode: "simulated-loopback"}
	s.reeve = executable(t, repoRoot, *reeveFlag)
	s.engine = executable(t, repoRoot, engineValue)
	report := *reportFlag
	if report == "" {
		report = filepath.Join(".local", "e2e", time.Now().UTC().Format("20060102T150405.000000000Z"))
	}
	reportRoot := absolute(repoRoot, report)
	s.check(os.MkdirAll(filepath.Dir(reportRoot), 0700))
	s.check(createReportRoot(reportRoot))
	s.report = reportRoot
	if reportSubdir != "" {
		s.report = filepath.Join(reportRoot, reportSubdir)
		s.check(os.Mkdir(s.report, 0700))
	}
	s.root = t.TempDir()
	server := httptest.NewServer(s.github)
	t.Cleanup(s.saveReport)
	t.Cleanup(server.Close)
	s.module = filepath.Join(s.root, "envs", "lifecycle")
	s.check(os.MkdirAll(s.module, 0700))
	config := filepath.Join(s.root, ".reeve")
	s.check(os.Mkdir(config, 0700))
	s.write(filepath.Join(config, "shared.yaml"), s.read(filepath.Join(cwd, "fixtures", "shared.yaml")), 0600)
	s.write(filepath.Join(s.module, "main.tf"), s.read(filepath.Join(cwd, "fixtures", "main.tf")), 0600)
	s.trace = filepath.Join(s.root, "engine-invocations.log")
	wrapper := filepath.Join(s.root, engineType+"-traced")
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
	s.write(wrapper, []byte("#!/bin/sh\nset -eu\nprintf '%s\\n' \"$*\" >> "+quote(s.trace)+"\nexec "+quote(s.engine)+" \"$@\"\n"), 0700)
	wrapperJSON, err := json.Marshal(wrapper)
	s.check(err)
	s.write(filepath.Join(config, engineType+".yaml"), []byte(fmt.Sprintf(`version: 1
config_type: engine
engine:
  type: %s
  binary:
    path: %s
  plan_locking: true
  stacks:
    - project: lifecycle
      path: envs/lifecycle
      stacks: [default]
  execution:
    preview_timeout: 1m
    apply_timeout: 1m
`, engineType, wrapperJSON)), 0600)
	testHome := filepath.Join(s.root, "home")
	s.check(os.Mkdir(testHome, 0700))
	s.env = []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + testHome,
		"XDG_CONFIG_HOME=" + filepath.Join(testHome, ".config"),
		"XDG_CACHE_HOME=" + filepath.Join(testHome, ".cache"),
		"TMPDIR=" + s.root, "LANG=C.UTF-8", "NO_COLOR=1", "CI=true",
		"GITHUB_API_URL=" + server.URL + "/api/v3/", "GITHUB_TOKEN=" + dummyToken,
	}
	t.Logf("Reeve: %s\n%s: %s\nReports: %s", s.reeve, engineName, s.engine, s.report)
	return s
}

func createReportRoot(path string) error {
	reportRoots.Lock()
	defer reportRoots.Unlock()
	if reportRoots.created[path] {
		return nil
	}
	if err := os.Mkdir(path, 0700); err != nil {
		return err
	}
	reportRoots.created[path] = true
	return nil
}

func (s *suite) saveReport() {
	status := "passed"
	if s.t.Failed() {
		status = "failed"
	}
	s.github.mu.Lock()
	defer s.github.mu.Unlock()
	for name, value := range map[string]any{
		"summary.json": object{
			"status": status, "error": s.failure, "github": s.githubMode,
			"reeve": s.reeve, "engine": s.engine, "scenarios": s.results,
			"api_requests": s.github.state.requests, "unexpected_requests": s.github.state.unexpected,
		},
		"comments.json": s.github.state.comments,
	} {
		data, err := json.MarshalIndent(value, "", "  ")
		if err == nil {
			err = os.WriteFile(filepath.Join(s.report, name), append(data, '\n'), 0600)
		}
		if err != nil {
			s.t.Errorf("Save %s: %v", name, err)
		}
	}
}

// tailBuffer bounds captured process output while keeping the final failure diagnostics.
type tailBuffer struct{ data []byte }

func (b *tailBuffer) Write(p []byte) (int, error) {
	const limit = 262144
	n := len(p)
	if n >= limit {
		b.data = append(b.data[:0], p[n-limit:]...)
		return n, nil
	}
	if overflow := len(b.data) + n - limit; overflow > 0 {
		copy(b.data, b.data[overflow:])
		b.data = b.data[:len(b.data)-overflow]
	}
	b.data = append(b.data, p...)
	return n, nil
}

func (s *suite) commands() []string {
	if !exists(s.trace) {
		return []string{}
	}
	return strings.Split(strings.TrimSuffix(string(s.read(s.trace)), "\n"), "\n")
}

func (s *suite) state() object {
	state := object{}
	path := filepath.Join(s.module, "terraform.tfstate")
	if exists(path) {
		s.readJSON(path, &state)
	}
	return state
}

func (s *suite) newHead(label string) {
	s.github.edit(func(g *githubState) { g.sha = fmt.Sprintf("%x", sha256.Sum256([]byte(label)))[:40] })
}

func (s *suite) run(label, command string, expected int) (*manifest, record) {
	return s.runArgs(label, command, expected)
}

func (s *suite) runArgs(label, command string, expected int, extra ...string) (*manifest, record) {
	s.t.Helper()
	requestsBefore := s.github.requestSnapshot()
	prRequestKey := fmt.Sprintf("GET /repos/%s/pulls/%d", s.repo, s.pr)
	prReadsBefore := 0
	if command == "apply" {
		s.github.edit(func(g *githubState) { prReadsBefore = g.requests[prRequestKey] })
	}
	sequence := len(s.results) + 1
	runAttempt := 1
	sha := s.github.head()
	prefix := command
	if command == "preview" {
		prefix = "run"
	}
	r := record{Scenario: label, RunID: fmt.Sprintf("%s-%d-%d-%s", prefix, sequence, runAttempt, sha[:7]), Log: fmt.Sprintf("%02d-%s.log", sequence, label)}
	args := []string{"run", command, "--root", s.root, "--repo", s.repo, "--pr", strconv.Itoa(s.pr), "--sha", sha, "--run-number", strconv.Itoa(sequence), "--run-attempt", strconv.Itoa(runAttempt)}
	if command == "apply" || command == "refresh" {
		args = append(args, "--actor", s.actor)
	}
	if command == "apply" {
		args = append(args, "--trigger-source", "comment")
	}
	args = append(args, extra...)
	before := len(s.commands())
	ctx, cancel := context.WithTimeout(s.t.Context(), 120*time.Second)
	defer cancel()
	// #nosec G204 -- Explicit test binary and fixed CLI arguments execute only in the disposable fixture.
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
	s.write(filepath.Join(s.report, r.Log), output.data, 0600)
	path := filepath.Join(s.root, ".reeve-state", "runs", fmt.Sprintf("pr-%d", s.pr), r.RunID, "manifest.json")
	var result *manifest
	if exists(path) {
		r.Manifest = true
		s.write(filepath.Join(s.report, fmt.Sprintf("%02d-%s.manifest.json", sequence, label)), s.read(path), 0600)
	}
	s.results = append(s.results, r)
	s.require(ctx.Err() == nil, "%s: command timed out or was canceled; see %s", label, r.Log)
	var exitErr *exec.ExitError
	s.require(err == nil || errors.As(err, &exitErr), "%s: process failed: %v", label, err)
	s.require(r.ExitCode == expected, "%s: exit %d, expected %d; see %s", label, r.ExitCode, expected, r.Log)
	var unexpected []string
	prReadsAfter := 0
	s.github.edit(func(g *githubState) {
		unexpected = append(unexpected, g.unexpected...)
		prReadsAfter = g.requests[prRequestKey]
	})
	s.require(len(unexpected) == 0, "Unexpected API calls: %v", unexpected)
	if command == "apply" {
		s.require(prReadsAfter-prReadsBefore == 1,
			"%s: apply fetched PR metadata %d times, expected one coherent snapshot", label, prReadsAfter-prReadsBefore)
	}
	if r.Manifest {
		result = &manifest{}
		s.readJSON(path, result)
	}
	s.t.Logf("%s: exit %d (%.3fs)", label, r.ExitCode, r.Seconds)
	return result, r
}

func (s *suite) stack(m *manifest) stackResult {
	s.t.Helper()
	s.require(m != nil, "Missing run manifest")
	s.require(len(m.Stacks) == 1, "Expected exactly one stack result")
	return m.Stacks[0]
}

func (s *suite) preview(label string, expected counts) {
	s.t.Helper()
	m, _ := s.run(label, "preview", 0)
	stack := s.stack(m)
	s.require(stack.Counts == expected, "%s: counts %+v != %+v", label, stack.Counts, expected)
	status := "noop"
	if expected != (counts{}) {
		status = "planned"
	}
	s.require(stack.Status == status, "%s: unexpected preview status %s", label, stack.Status)
	s.require(stack.PlanKey != "" && exists(filepath.Join(s.root, ".reeve-state", stack.PlanKey)), "%s: saved plan missing", label)
}

func (s *suite) files(dir, name string) []string {
	var paths []string
	s.check(filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && ((name == "" && filepath.Ext(path) == ".json") || entry.Name() == name) {
			paths = append(paths, path)
		}
		return nil
	}))
	return paths
}

func (s *suite) locksReleased() {
	paths := s.files(filepath.Join(s.root, ".reeve-state", "locks"), "")
	s.require(len(paths) > 0, "Expected a persisted lock")
	for _, path := range paths {
		var lock struct {
			Holder json.RawMessage   `json:"holder"`
			Queue  []json.RawMessage `json:"queue"`
		}
		s.readJSON(path, &lock)
		s.require((len(lock.Holder) == 0 || string(lock.Holder) == "null") && len(lock.Queue) == 0, "Apply left a holder or queue entry")
	}
}

func (s *suite) audit(r record, outcome string) {
	paths := s.files(filepath.Join(s.root, ".reeve-state", "audit"), r.RunID+".json")
	s.require(len(paths) == 1, "Expected one durable audit entry")
	var entry struct {
		Outcome string `json:"outcome"`
		SHA     string `json:"commit_sha"`
	}
	s.readJSON(paths[0], &entry)
	s.require(entry.Outcome == outcome, "Audit outcome %s != %s", entry.Outcome, outcome)
	s.require(entry.SHA == s.github.head(), "Audit SHA does not match the PR")
	s.write(filepath.Join(s.report, r.Scenario+".audit.json"), s.read(paths[0]), 0600)
}

func (s *suite) appliedMarker() bool {
	return exists(filepath.Join(s.root, ".reeve-state", "runs", fmt.Sprintf("pr-%d", s.pr), "applied", s.github.head()+".json"))
}

func (s *suite) blocked(label, gateName string) (*manifest, record) {
	s.t.Helper()
	before := s.state()
	m, r := s.run(label, "apply", 0)
	stack := s.stack(m)
	s.require(stack.Status == "blocked", "%s: expected blocked result", label)
	denied := false
	for _, gate := range stack.Gates {
		if gate.Gate == gateName && gate.Outcome == "fail" {
			denied = true
		}
	}
	s.require(denied, "%s: %s did not deny", label, gateName)
	s.require(len(r.EngineCommands) == 0, "%s: engine executed despite denial", label)
	s.require(reflect.DeepEqual(s.state(), before), "%s: state changed despite denial", label)
	s.require(!s.appliedMarker(), "%s: false applied marker", label)
	s.audit(r, "blocked")
	s.locksReleased()
	return m, r
}

func (s *suite) apply(label string, failed bool) {
	s.t.Helper()
	exit, status, outcome := 0, "planned", "success"
	if failed {
		exit, status, outcome = 1, "error", "failed"
	}
	requestKey := fmt.Sprintf("GET /repos/%s/issues/%d/comments", s.repo, s.pr)
	commentReadsBefore := 0
	s.github.edit(func(g *githubState) { commentReadsBefore = g.requests[requestKey] })
	m, r := s.run(label, "apply", exit)
	commentReadsAfter := 0
	s.github.edit(func(g *githubState) { commentReadsAfter = g.requests[requestKey] })
	s.require(commentReadsAfter-commentReadsBefore == 1,
		"%s: apply fetched the PR comment history %d times, expected one coherent snapshot",
		label, commentReadsAfter-commentReadsBefore)
	stack := s.stack(m)
	s.require(stack.Status == status, "%s: unexpected apply status %s", label, stack.Status)
	applied := false
	for _, command := range r.EngineCommands {
		if strings.HasPrefix(command, "apply ") {
			applied = true
		}
		s.require(!strings.HasPrefix(command, "plan "), "%s: saved-plan apply silently re-planned", label)
	}
	s.require(applied, "%s: engine apply did not execute", label)
	s.require(s.appliedMarker() != failed, "%s: applied marker mismatch", label)
	s.audit(r, outcome)
	s.locksReleased()
}
