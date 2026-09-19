# Live GitHub identity tests

Use two GitHub Apps to verify that Reeve distinguishes a PR author from its reviewer and evaluates reviews against the correct commit.
The manual **Live GitHub E2E** workflow creates a disposable PR, runs the lifecycle, and cleans it up.

## First layer

Start with [local E2E](README.md) if you want to test without credentials. It uses simulated author/reviewer identities and needs no GitHub Apps.
The live workflow adds real GitHub API interactions while keeping engine and Reeve state in a temporary filesystem inside one job.
For a human-driven tour, use the [guided playground](../docs/playground.md), which uses the same configured Apps.

## Before you begin

You need access to configure repository or organization Actions settings and install two Apps on `reeve-test`.
The current live driver is restricted to `reeveops/reeve-test` and its tracked App identities; reusing it elsewhere requires aligning the workflow guards, harness identities, and CODEOWNERS.

## 1. Configure two Apps

| Identity | App name used by this repository | Repository permissions | Purpose |
| --- | --- | --- | --- |
| Author | `reeve-e2e-author` | Contents: write; Pull requests: write | Create fixture commits, branches, and PRs. |
| Reviewer | `reeve-e2e-reviewer` | Pull requests: write | Review the exact PR head SHA. |

App names must be available when creating a new installation. Use each resulting exact `<app-slug>[bot]` login consistently in the test configuration and CODEOWNERS.
A different Git commit author string does not create a second authenticated reviewer.

Install each App on the fixture repository only and keep webhook delivery disabled if it serves only as an API identity.
Store signing keys outside fixture files and engine environments, and keep reviewer signing authority separate from author/workload code.
App signing keys need their own rotation; the job's installation tokens are short-lived.

## 2. Add Actions settings

| Name | Kind | Value |
| --- | --- | --- |
| `E2E_AUTHOR_APP_ID` | Variable | Author App ID. |
| `E2E_REVIEWER_APP_ID` | Variable | Reviewer App ID. |
| `E2E_AUTHOR_PRIVATE_KEY` | Secret | Author App signing key. |
| `E2E_REVIEWER_PRIVATE_KEY` | Secret | Reviewer App signing key. |

Repository settings or organization settings granted to `reeve-test` work with the workflow.
The current organization settings are shared with `reeve` and `reeve-test`; token minting remains scoped to `reeve-test`.
If you use environment-scoped settings, the live job must select that environment before dispatch.

## 3. Run the workflow

1. Open **Actions → Live GitHub E2E**.
2. Choose **Run workflow** from `master`.
3. Open the run and follow the owned fixture PR link in its output.
4. Check the final job result and download the diagnostic artifact if a scenario fails.

The [workflow](../.github/workflows/e2e-live.yml) builds the pinned Reeve source and trusted harness before minting tokens.
It discovers the App logins from the token action and supplies tokens to the Go harness through stdin.
Reeve reaches GitHub through a scoped loopback relay and receives only the fixture credential; author/reviewer tokens stay out of engine and untrusted PR code.

## Live acceptance sequence

1. The author App creates a fixture PR at a recorded head SHA.
2. The shared GitOps workflow previews its OpenTofu project; the harness waits for that check.
3. The harness runs its own preview and persists artifacts in the job's temporary bucket.
4. Apply without approval is blocked after fetching default-branch CODEOWNERS.
5. The reviewer App approves that exact head; Reeve resolves the approval and the harness applies.
6. Further commits test cancellation of a superseded shared-workflow preview and rejection of the old approval.
7. A changes-requested review keeps apply blocked; a new approval permits the update.
8. A final approved delete converges to no resources, then cleanup closes the PR and deletes its branch.

The successful lifecycle applies run through the trusted harness while evaluating normal Reeve gates.
The shared-workflow checks and the harness's local artifacts are separate: this result does not prove artifact sharing or slash-command apply across different runners.

## Review policy

The default-branch [CODEOWNERS](../.github/CODEOWNERS) assigns the lifecycle fixture to both App bot logins.
The baseline local and live fixtures enable `codeowners: true` without an explicit approver list, so a successful baseline apply proves Reeve resolved the reviewer through CODEOWNERS. The separate local authorization matrix also tests other policy combinations.

Keep `dismiss_on_new_commit: true` and submit reviews with `commit_id` set to the resolved head SHA.
The harness checks each returned review's author, state, and commit ID before expecting the gate to pass.
GitHub's [create-review API](https://docs.github.com/en/rest/pulls/reviews#create-a-review-for-a-pull-request) accepts installation tokens with Pull requests: write.

GitHub native code owners must be people or teams with write access. The App entries exercise Reeve's raw CODEOWNERS gate and do not establish GitHub branch-protection review eligibility.
Test merge eligibility separately from Reeve's apply decision.

## Command dispatch

Reeve rejects bot-authored command comments and bot-authored `pr_comment` approvals. An App's `/reeve apply` comment remains a rejection case.
Use a real authorized human for the normal successful slash-command path; the [guided tour](../docs/playground.md) reads human comments through its own controller.

Neither initial App needs Actions: write just to create commits or reviews. Give a future workflow dispatcher that permission only if it needs to dispatch workflows.

## Token lifecycle

Use the pinned [GitHub App token action](https://github.com/actions/create-github-app-token) to mint repository-scoped tokens with only the required permissions and revoke them when the job ends.
The existing workflow does this after the build. No token or signing key is needed for the local E2E workflow.

## Reports and cleanup

The report artifact includes `live.json` with the owned repository, PR, and branch identifiers, alongside the command reports.
Cleanup runs both in Go and in an `always()` workflow step, with ownership checks before closing or deleting anything.

A runner lost before cleanup can leave a `reeve-e2e/<run>-<attempt>-<suffix>` branch and PR for manual removal.
Use `live.json` and the PR's head branch to identify that fixture. The guided playground has separate nightly cleanup; it does not clean up live-test branches.

## Troubleshooting

| Symptom | What to check |
| --- | --- |
| App token creation fails | App IDs, signing-key settings, installation access to `reeve-test`, and the requested permissions. |
| The job is skipped | Dispatch the live workflow from `master` in `reeveops/reeve-test`. |
| The harness rejects App identities | Match the App bot logins to tracked CODEOWNERS and the harness identity constants. |
| The shared-workflow preview never succeeds | Open the fixture PR's checks; the live test waits for that separate workflow. |
| Approval exists but apply is blocked | Verify reviewer identity, review state, current commit ID, and the reported gate. |
