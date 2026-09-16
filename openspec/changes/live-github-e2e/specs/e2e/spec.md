## ADDED Requirements

### Requirement: Live GitHub approval lifecycle

The harness MUST use different installed GitHub Apps to author PRs and submit reviews.
It MUST use real Reeve gates and real OpenTofu with disposable local state.

#### Scenario: Approval changes with the head commit

- GIVEN an App-authored fixture PR
- WHEN apply runs before approval or after the head changes
- THEN the approval gate MUST deny without invoking the engine.
- WHEN the reviewer App approves the current head
- THEN apply MUST use the saved plan and persist the expected state.

#### Scenario: Reviewer requests changes

- GIVEN a preview for the current head
- WHEN the reviewer App requests changes
- THEN apply MUST remain blocked without engine execution.

### Requirement: Trusted credential boundary

The live workflow MUST run only on manually selected trusted master code.
Tokens MUST stay out of engine environments, command arguments, and diagnostic artifacts.

#### Scenario: Live API access

- GIVEN tokens delivered to the harness through stdin
- WHEN Reeve accesses GitHub through the loopback relay
- THEN the relay MUST restrict requests to the fixture's allowed routes and attach the workflow credential only to GitHub.

### Requirement: Owned fixture cleanup

The harness MUST close its PR and delete only the branch it created on completion or failure.

#### Scenario: Interrupted run

- GIVEN a created fixture PR and branch
- WHEN execution fails
- THEN cleanup MUST run and diagnostic artifacts MUST retain the fixture identifiers.
