# Local E2E without cloud credentials

## Why

- Existing workflows start with empty filesystem state on each runner.
- The fixture has no automated assertion of the complete preview, approval, apply, and cleanup lifecycle.

## What

- Run the real Reeve CLI and OpenTofu against disposable local state.
- Supply GitHub responses through a loopback HTTP fixture and assert security decisions through the public CLI.
- Add a pinned CI workflow and document the later live GitHub author/reviewer setup.

## Scope

- This change adds a test harness to `reeve-test`; it changes no Reeve runtime behavior.
- Live GitHub events, Pulumi coverage, and cloud storage contracts are follow-up layers.
