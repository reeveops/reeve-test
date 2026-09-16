//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type liveAPI struct {
	base, repo, token string
	client            *http.Client
}

func newLiveAPI(repo, token string) *liveAPI {
	return &liveAPI{base: "https://api.github.com", repo: repo, token: token,
		client: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
}

func (a *liveAPI) request(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return a.client.Do(req)
}

func (a *liveAPI) call(ctx context.Context, method, suffix string, body, result any) error {
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	resp, err := a.request(ctx, method, "/repos/"+a.repo+suffix, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("GitHub %s %s: transport failed", method, suffix)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GitHub %s %s: HTTP %d", method, suffix, resp.StatusCode)
	}
	if result == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(result)
}

// waitPRHead waits for GitHub's PR metadata to reflect the committed branch update.
func (a *liveAPI) waitPRHead(ctx context.Context, pr int, sha string, interval time.Duration) error {
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("wait for PR #%d head: %w", pr, err)
		}
		var result struct{ Head struct{ SHA string } }
		if err := a.call(ctx, http.MethodGet, fmt.Sprintf("/pulls/%d", pr), nil, &result); err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("wait for PR #%d head: %w", pr, ctx.Err())
			}
			return err
		}
		if result.Head.SHA == sha {
			return nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("wait for PR #%d head: %w", pr, ctx.Err())
		case <-timer.C:
		}
	}
}

type liveRelay struct {
	mu         sync.Mutex
	api        *liveAPI
	suite      *suite
	commentIDs map[int64]bool
}

// allowed exposes only reads needed by Reeve and comments belonging to this fixture PR.
func (p *liveRelay) allowed(method, path string) bool {
	s := p.suite
	prefix := "/repos/" + s.repo
	pr := strconv.Itoa(s.pr)
	sha := s.github.head()
	if method == http.MethodGet {
		switch path {
		case "/user", prefix + "/pulls/" + pr, prefix + "/pulls/" + pr + "/files", prefix + "/pulls/" + pr + "/reviews",
			prefix + "/issues/" + pr + "/comments", prefix + "/commits/" + sha + "/check-runs", prefix + "/commits/" + sha + "/status",
			prefix + "/compare/master..." + sha, prefix + "/contents/.github/CODEOWNERS", prefix + "/contents/CODEOWNERS", prefix + "/contents/docs/CODEOWNERS":
			return true
		}
	}
	if method == http.MethodPost && path == prefix+"/issues/"+pr+"/comments" {
		return true
	}
	if method == http.MethodPatch || method == http.MethodDelete {
		for id := range p.commentIDs {
			if path == fmt.Sprintf("%s/issues/comments/%d", prefix, id) {
				return true
			}
		}
	}
	return false
}

func (p *liveRelay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/api/v3")
	allowed := p.allowed(r.Method, path)
	g := p.suite.github
	g.mu.Lock()
	defer g.mu.Unlock()
	g.state.requests[r.Method+" "+path]++
	if r.Header.Get("Authorization") != "Bearer "+dummyToken || !allowed {
		g.state.unexpected = append(g.state.unexpected, r.Method+" "+path)
		http.Error(w, "Unexpected live API request", http.StatusForbidden)
		return
	}
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "Invalid request body", 400)
		return
	}
	resp, err := p.api.request(r.Context(), r.Method, path, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "GitHub request failed", 502)
		return
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil || len(data) > 8<<20 {
		http.Error(w, "GitHub response unreadable or too large", 502)
		return
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && r.Method == http.MethodPost {
		var comment struct {
			ID int64 `json:"id"`
		}
		if json.Unmarshal(data, &comment) == nil && comment.ID > 0 {
			p.commentIDs[comment.ID] = true
		}
	}
	if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/comments") && resp.StatusCode == 200 {
		var comments []object
		if json.Unmarshal(data, &comments) == nil {
			g.state.comments = comments
		}
	}
	for _, header := range []string{"Content-Type", "Link", "Retry-After", "X-RateLimit-Remaining", "X-RateLimit-Reset"} {
		if value := resp.Header.Get(header); value != "" {
			w.Header().Set(header, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := w.Write(data); err != nil {
		g.state.unexpected = append(g.state.unexpected, "Relay response write failed")
	}
}

func TestLiveRelay(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, token string
		status                    int
		forwarded                 bool
	}{
		{"read", "GET", "/api/v3/repos/reeve-e2e/fixture/pulls/1", dummyToken, 503, true},
		{"wrong-repo", "GET", "/api/v3/repos/reeve-e2e/other/pulls/1", dummyToken, 403, false},
		{"wrong-pr", "POST", "/api/v3/repos/reeve-e2e/fixture/issues/2/comments", dummyToken, 403, false},
		{"unknown-comment", "PATCH", "/api/v3/repos/reeve-e2e/fixture/issues/comments/999", dummyToken, 403, false},
		{"write-source", "PUT", "/api/v3/repos/reeve-e2e/fixture/contents/main.tf", dummyToken, 403, false},
		{"wrong-token", "GET", "/api/v3/repos/reeve-e2e/fixture/pulls/1", "invalid", 403, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var forwarded atomic.Bool
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				forwarded.Store(true)
				if r.Header.Get("Authorization") != "Bearer controller-fixture-token" {
					t.Error("Relay did not substitute credential")
				}
				w.WriteHeader(503)
				_, _ = io.WriteString(w, `{"message":"fixture outage"}`)
			}))
			defer upstream.Close()
			a := newLiveAPI(repository, "controller-fixture-token")
			a.base = upstream.URL
			s := &suite{repo: repository, pr: 1, github: newGitHub()}
			p := &liveRelay{api: a, suite: s, commentIDs: map[int64]bool{}}
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Header.Set("Authorization", "Bearer "+tc.token)
			w := httptest.NewRecorder()
			p.ServeHTTP(w, r)
			if w.Code != tc.status || forwarded.Load() != tc.forwarded {
				t.Fatalf("status=%d forwarded=%v", w.Code, forwarded.Load())
			}
			if tc.forwarded && w.Body.String() != `{"message":"fixture outage"}` {
				t.Fatal("Relay changed upstream error")
			}
		})
	}
}

func TestWaitPRHead(t *testing.T) {
	for _, tc := range []struct {
		name         string
		staleReads   int32
		status       int
		cancelOnRead bool
		wantReads    int32
		wantError    bool
	}{
		{name: "already-current", status: 200, wantReads: 1},
		{name: "stale-until-third-read", staleReads: 2, status: 200, wantReads: 3},
		{name: "api-denial", status: 403, wantReads: 1, wantError: true},
		{name: "cancel-stale-head", staleReads: 100, status: 200, cancelOnRead: true, wantReads: 1, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			var reads atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/repos/"+repository+"/pulls/12" {
					t.Errorf("Unexpected PR lookup: %s %s", r.Method, r.URL.Path)
				}
				sha := "new-head"
				if reads.Add(1) <= tc.staleReads {
					sha = "old-head"
				}
				w.WriteHeader(tc.status)
				if err := json.NewEncoder(w).Encode(object{"head": object{"sha": sha}}); err != nil {
					t.Error(err)
				}
				if tc.cancelOnRead {
					cancel()
				}
			}))
			defer server.Close()
			a := newLiveAPI(repository, dummyToken)
			a.base = server.URL
			err := a.waitPRHead(ctx, 12, "new-head", time.Millisecond)
			if (err != nil) != tc.wantError {
				t.Fatalf("Unexpected wait result: %v", err)
			}
			if tc.cancelOnRead && !errors.Is(err, context.Canceled) {
				t.Fatalf("Expected cancellation, got %v", err)
			}
			if reads.Load() != tc.wantReads {
				t.Fatalf("reads=%d, want %d", reads.Load(), tc.wantReads)
			}
		})
	}
}
