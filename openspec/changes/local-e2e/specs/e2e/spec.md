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

#### Scenario: Approval authorization matrix

- GIVEN mutable CODEOWNERS and shared approval policy fixtures
- WHEN reviewers satisfy CODEOWNERS, explicit approver lists, both constraints, or the public unlisted-review opt-in
- THEN apply MUST run exactly once and persist the requested state.
- WHEN a reviewer satisfies none or only one of two required constraints
- THEN apply MUST be blocked without engine execution or state mutation.

#### Scenario: Break-glass authorization matrix

- GIVEN an unsatisfied normal approval policy and a non-empty justification
- WHEN `internal_list`, CODEOWNERS, or `anyone` authorizes the actor
- THEN apply MUST run and persist intent and completion audits naming the authorization source.
- WHEN `internal_list` or CODEOWNERS does not authorize the actor
- THEN Reeve MUST deny before engine execution and leave state unchanged.

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

### Requirement: Local object-store compatibility

The harness MUST run the provider contracts against pinned disposable S3 and GCS emulators.
It MUST distinguish an emulator's declared conditional-delete limitation from an adapter failure.

#### Scenario: Unsupported conditional delete

- GIVEN an emulator that reports conditional deletion as unsupported
- WHEN the provider contract probes that capability
- THEN the adapter MUST return the unsupported-capability sentinel and preserve the replacement object.

#### Scenario: Ignored conditional-delete precondition

- GIVEN an emulator that silently ignores a delete precondition
- WHEN the provider contract attempts a stale conditional delete
- THEN the harness MUST require the strict contract to reject the emulator with the ignored-precondition diagnostic.
