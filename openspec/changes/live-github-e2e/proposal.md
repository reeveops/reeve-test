# Live GitHub E2E

- Exercise real App-authored PRs and reviews through Reeve's existing CLI gates.
- Keep engine and coordination state in one disposable job; cloud storage remains a later lane.

## Scope

- Add a manually dispatched workflow on trusted `master`, using separate author and reviewer Apps.
- Reuse the Go lifecycle assertions and real OpenTofu fixture.
- Record the PR URL, command outcomes, and cleanup identifiers without credentials.
- Leave Reeve product code and bot-command rejection unchanged.
