//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type planStart struct {
	path string
	at   time.Time
}

func TestParallelPreview(t *testing.T) {
	s := newSuite(t)
	modules := []struct {
		project string
		path    string
		stacks  []string
	}{
		{project: "api", path: "envs/api", stacks: []string{"dev", "prod"}},
		{project: "worker", path: "envs/worker", stacks: []string{"prod"}},
		{project: "web", path: "envs/web", stacks: []string{"prod"}},
	}
	for _, module := range modules {
		dir := filepath.Join(s.root, module.path)
		s.check(os.MkdirAll(dir, 0o700))
		s.write(filepath.Join(dir, "main.tf"), s.read(filepath.Join(filepath.Dir(s.module), "lifecycle", "main.tf")), 0o600)
	}

	wrapper := filepath.Join(s.root, "tofu-traced")
	s.write(wrapper, []byte(parallelWrapper(s.trace, s.engine)), 0o700)
	wrapperJSON, err := json.Marshal(wrapper)
	s.check(err)
	var declarations strings.Builder
	for _, module := range modules {
		fmt.Fprintf(&declarations, "    - project: %s\n      path: %s\n      stacks: [%s]\n",
			module.project, module.path, strings.Join(module.stacks, ", "))
	}
	s.write(filepath.Join(s.root, ".reeve", "tofu.yaml"), []byte(fmt.Sprintf(`version: 1
config_type: engine
engine:
  type: tofu
  binary:
    path: %s
  plan_locking: true
  stacks:
%s  execution:
    max_parallel_stacks: 2
    preview_timeout: 1m
    apply_timeout: 1m
`, wrapperJSON, declarations.String())), 0o600)

	s.github.edit(func(g *githubState) { g.changed = "shared/provider.tf" })
	s.newHead("parallel-preview")
	manifest, _ := s.run("parallel-preview", "preview", 0)
	s.require(manifest != nil, "Parallel preview did not persist a manifest")
	s.require(len(manifest.Stacks) == 4, "Parallel preview returned %d stacks, want 4", len(manifest.Stacks))

	starts := planStarts(s, s.commands())
	s.require(len(starts) == 4, "Recorded %d plan starts, want 4: %v", len(starts), s.commands())
	firstByPath := map[string]time.Time{}
	var apiGap time.Duration
	overlapped := false
	for _, start := range starts {
		if first, ok := firstByPath[start.path]; ok {
			if strings.HasSuffix(filepath.ToSlash(start.path), "/envs/api") {
				apiGap = start.at.Sub(first)
			}
			continue
		}
		for path, at := range firstByPath {
			if path != start.path && absDuration(start.at.Sub(at)) < 500*time.Millisecond {
				overlapped = true
			}
		}
		firstByPath[start.path] = start.at
	}
	s.require(overlapped, "Independent project plans did not overlap: %v", starts)
	s.require(apiGap >= 900*time.Millisecond, "Same-directory API plans overlapped; start gap was %s", apiGap)
}

func parallelWrapper(trace, engine string) string {
	return "#!/bin/sh\nset -eu\n" +
		"printf '%s\\n' \"$*\" >> " + shellQuote(trace) + "\n" +
		"if [ \"${1:-}\" = plan ]; then\n" +
		"  printf 'PLAN_START|%s|%s\\n' \"$PWD\" \"$(date +%s%N)\" >> " + shellQuote(trace) + "\n" +
		"  sleep 1\n" +
		"fi\n" +
		"exec " + shellQuote(engine) + " \"$@\"\n"
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func planStarts(s *suite, lines []string) []planStart {
	s.t.Helper()
	var starts []planStart
	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) != 3 || parts[0] != "PLAN_START" {
			continue
		}
		nanos, err := strconv.ParseInt(parts[2], 10, 64)
		s.check(err)
		starts = append(starts, planStart{path: parts[1], at: time.Unix(0, nanos)})
	}
	return starts
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}
