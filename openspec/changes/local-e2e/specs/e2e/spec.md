## ADDED Requirements

### Requirement: Disposable real-engine lifecycle

The harness MUST execute separate Reeve processes with a real OpenTofu binary and shared per-scenario filesystem state.
It MUST need no cloud credentials or real GitHub token.

#### Scenario: Resource lifecycle

- GIVEN an empty scenario directory
- WHEN preview and apply create, update, and remove a resource
- THEN manifests and engine state MUST reflect each change and a converged preview MUST report no changes.

### Requirement: Security decisions through the CLI

The harness MUST use Reeve's existing gates without adding a bypass.
Simulated GitHub responses MUST be identified as test fixtures.

#### Scenario: Apply denied

- GIVEN absent, stale, self, or unlisted approvals, failing checks, or a denied PR classification
- WHEN apply runs
- THEN the relevant gate MUST deny and the engine MUST perform no apply.

#### Scenario: Approval API unavailable

- GIVEN the review endpoint returns an error
- WHEN apply evaluates approvals
- THEN Reeve MUST exit nonzero without invoking engine apply.

### Requirement: Failure and artifact assertions

The harness MUST inspect artifacts and process outcomes instead of treating command completion as success.

#### Scenario: Engine failure

- GIVEN a valid preview and approval for a failing provisioner
- WHEN apply executes
- THEN Reeve MUST exit nonzero, persist a failed result and audit entry, release its lock, and omit the applied marker.

### Requirement: Isolated test execution

The harness MUST isolate inherited credentials and clean up disposable state on normal completion and failure.
Unexpected API requests MUST fail the scenario.

#### Scenario: Local and CI invocation

- GIVEN installed binaries and the same fixture source
- WHEN the harness runs locally or in CI
- THEN it MUST use loopback API traffic and local state with the same assertions.
