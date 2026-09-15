# Design

- Compile the Go harness and pinned Reeve source before minting App tokens.
- Pass short-lived tokens to the harness through stdin; signing keys remain action inputs.
- A loopback relay authenticates Reeve's GitHub requests with the workflow token, keeping real tokens out of engine environments and command arguments.
- Forward only the fixture repository's required API routes; preserve GitHub response status and payload.
- Create and update a fixed OpenTofu fixture using the author App's Contents API identity.
- Submit reviews using the reviewer App and assert the returned login, state, and commit SHA.
- Keep approvals, freshness, check, and base-update gates enabled; wait for real checks before expecting an apply.
- Execute trusted fixture content from the workflow checkout without checking out PR code.
- Close the owned PR and delete its branch on completion or failure; retain cleanup identifiers for interrupted jobs.
- Suppress the local E2E job for generated `reeve-e2e/` branches to avoid redundant harness runs.
