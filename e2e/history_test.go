//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUnrelatedRunHistoryDoesNotAffectLifecycle(t *testing.T) {
	t.Parallel()
	s := newSuiteInReport(t, "unrelated-history")

	historyDir := filepath.Join(s.root, ".reeve-state", "runs", "pr-999", "history")
	s.check(os.MkdirAll(historyDir, 0o700))
	old := time.Now().Add(-60 * 24 * time.Hour)
	for i := range 2000 {
		path := filepath.Join(historyDir, fmt.Sprintf("artifact-%04d.json", i))
		s.write(path, []byte("{unrelated-invalid-history\n"), 0o600)
		s.check(os.Chtimes(path, old, old))
	}

	first := filepath.Join(historyDir, "artifact-0000.json")
	last := filepath.Join(historyDir, "artifact-1999.json")
	s.newHead("unrelated-history")
	s.preview("history-preview", counts{Add: 1})
	s.github.approve("", "", "")
	s.apply("history-apply", false)

	s.require(exists(first) && exists(last), "ordinary preview/apply pruned unrelated run history")
	s.require(len(s.resources()) == 1, "unrelated run history changed the applied lifecycle")
}
