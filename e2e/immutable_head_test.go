//go:build e2e

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestImmutableHeadMismatchFailsClosed(t *testing.T) {
	t.Parallel()

	s := newSuiteInReport(t, "immutable-head")
	s.newHead("immutable-head")
	expected := s.github.head()
	s.env = append(s.env, "REEVE_EXPECTED_HEAD_SHA="+expected)

	_, preview := s.run("immutable-head-preview", "preview", 0)
	s.require(preview.Manifest, "matching immutable head did not publish a preview manifest")

	s.newHead("immutable-head-moved")
	_, apply := s.run("immutable-head-apply", "apply", 1)
	s.require(len(apply.EngineCommands) == 0, "moved PR head invoked the engine: %v", apply.EngineCommands)
	log := string(s.read(filepath.Join(s.report, apply.Log)))
	s.require(strings.Contains(log, "checked-out revision does not match the current PR head"),
		"moved-head failure did not explain the identity mismatch: %s", log)
}
