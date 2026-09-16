//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	repository = "reeve-e2e/fixture"
	author     = "reeve-e2e-author[bot]"
	reviewer   = "reeve-e2e-reviewer[bot]"
	controller = "reeve-e2e-controller[bot]"
	dummyToken = "reeve-e2e-not-a-real-token"
)

type object = map[string]any

type githubState struct {
	sha                      string
	changed                  string
	reviews                  []object
	comments                 []object
	check                    string
	behind                   int
	draft, fork, reviewError bool
	requests                 map[string]int
	unexpected               []string
}

type githubFixture struct {
	mu    sync.Mutex
	state githubState
}

func newGitHub() *githubFixture {
	return &githubFixture{state: githubState{
		check: "success", changed: "envs/lifecycle/main.tf", reviews: []object{}, comments: []object{},
		requests: map[string]int{}, unexpected: []string{},
	}}
}

func (g *githubFixture) edit(fn func(*githubState)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	fn(&g.state)
}

func (g *githubFixture) head() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.state.sha
}

func (g *githubFixture) requestSnapshot() map[string]int {
	g.mu.Lock()
	defer g.mu.Unlock()
	result := make(map[string]int, len(g.state.requests))
	for key, count := range g.state.requests {
		result[key] = count
	}
	return result
}

func requestDelta(before, after map[string]int) map[string]int {
	result := map[string]int{}
	for key, count := range after {
		if delta := count - before[key]; delta > 0 {
			result[key] = delta
		}
	}
	return result
}

func (g *githubFixture) approve(login, sha, state string) {
	g.edit(func(s *githubState) {
		if login == "" {
			login = reviewer
		}
		if sha == "" {
			sha = s.sha
		}
		if state == "" {
			state = "APPROVED"
		}
		s.reviews = []object{{
			"id": 1, "user": object{"login": login, "type": "Bot"},
			"state": state, "commit_id": sha, "submitted_at": time.Now().UTC().Format(time.RFC3339),
		}}
	})
}

func (g *githubFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.Header.Get("Authorization") != "Bearer "+dummyToken {
		g.state.unexpected = append(g.state.unexpected, "Unexpected authorization header")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	body := object{}
	if r.Method == http.MethodPost || r.Method == http.MethodPatch {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
			g.state.unexpected = append(g.state.unexpected, "Invalid request body")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	code, result := g.route(r.Method, strings.TrimPrefix(r.URL.Path, "/api/v3"), body)
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		g.state.unexpected = append(g.state.unexpected, "Response write failed: "+err.Error())
	}
}

// route runs under the fixture mutex so test mutations and HTTP handlers stay synchronized.
func (g *githubFixture) route(method, path string, body object) (int, any) {
	s := &g.state
	s.requests[method+" "+path]++
	prefix := "/repos/" + repository
	if method == http.MethodGet {
		for pr := 1; pr <= 3; pr++ {
			switch path {
			case fmt.Sprintf("%s/pulls/%d", prefix, pr):
				headRepo := repository
				if s.fork {
					headRepo = "outside/fork"
				}
				return 200, object{
					"number": pr, "title": "Local E2E fixture", "state": "open",
					"user": object{"login": author, "type": "Bot"}, "draft": s.draft,
					"created_at": time.Now().UTC().Format(time.RFC3339), "html_url": fmt.Sprintf("http://127.0.0.1/pr/%d", pr),
					"head": object{"sha": s.sha, "ref": "e2e", "repo": object{"full_name": headRepo}},
					"base": object{"sha": strings.Repeat("b", 40), "ref": "master", "repo": object{"full_name": repository, "private": false}},
				}
			case fmt.Sprintf("%s/pulls/%d/files", prefix, pr):
				return 200, []object{{"filename": s.changed, "status": "modified"}}
			case fmt.Sprintf("%s/pulls/%d/reviews", prefix, pr):
				if s.reviewError {
					return 503, object{"message": "simulated review outage"}
				}
				return 200, s.reviews
			case fmt.Sprintf("%s/issues/%d/comments", prefix, pr):
				comments := make([]object, 0, len(s.comments))
				for _, comment := range s.comments {
					issue, ok := comment["issue_number"].(int)
					if !ok {
						issue = 1
					}
					if issue == pr {
						comments = append(comments, comment)
					}
				}
				return 200, comments
			}
		}
		switch path {
		case prefix + "/commits/" + s.sha + "/check-runs":
			return 200, object{"total_count": 1, "check_runs": []object{{
				"id": 1, "name": "fixture-required-check", "status": "completed", "conclusion": s.check,
			}}}
		case prefix + "/commits/" + s.sha + "/status":
			return 200, object{"state": "success", "statuses": []object{}, "total_count": 0}
		case prefix + "/compare/master..." + s.sha:
			return 200, object{"behind_by": s.behind}
		case prefix + "/contents/.github/CODEOWNERS", prefix + "/contents/CODEOWNERS", prefix + "/contents/docs/CODEOWNERS":
			return 404, object{"message": "Not Found"}
		case "/user":
			return 200, object{"login": controller}
		}
	}
	if method == http.MethodPost {
		for pr := 1; pr <= 3; pr++ {
			if path == fmt.Sprintf("%s/issues/%d/comments", prefix, pr) {
				comment := object{"id": len(s.comments) + 1, "issue_number": pr, "body": body["body"], "user": object{"login": controller}}
				s.comments = append(s.comments, comment)
				return 201, comment
			}
		}
	}
	if method == http.MethodPatch {
		for _, comment := range s.comments {
			if path == fmt.Sprintf("%s/issues/comments/%v", prefix, comment["id"]) {
				comment["body"] = body["body"]
				return 200, comment
			}
		}
	}
	s.unexpected = append(s.unexpected, method+" "+path)
	return 500, object{"message": "Unexpected test API route"}
}
