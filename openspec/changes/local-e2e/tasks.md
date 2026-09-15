# Tasks

- [x] Define the isolated process and filesystem test boundary.
- [x] Add real CLI/engine lifecycle assertions and GitHub API fixtures.
- [x] Cover approval denial, self/unlisted/stale reviews, failed checks, fork/draft gates, and API failures.
- [x] Assert create, update, delete, no-op, saved-plan execution, failure persistence, and lock cleanup.
- [x] Add a pinned local/CI entrypoint and bounded diagnostics.
- [x] Document test limitations and live GitHub App setup.
- [x] Run the complete harness and validate workflow syntax and spec structure.

## Validation

- `mise run e2e`: all 24 CLI scenarios passed against sibling Reeve commit `1847a83f6ec8cf4b6d9baa4054393f6666a51ad9` and OpenTofu 1.12.6.
- Go formatting, vet, and all 24 scenarios with the race detector passed; Actionlint 1.7.12 and strict OpenSpec validation passed.
- Live GitHub workflow execution remains untested; these changes have not been pushed.
