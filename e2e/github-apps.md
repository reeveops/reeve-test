# GitHub identities for the live test layer

## First layer

- `mise run e2e` needs no tokens and uses synthetic author/reviewer identities.
- [Live GitHub E2E](../.github/workflows/e2e-live.yml) is a separate manual workflow on `master`.

## Actions settings

| Name | Kind |
| --- | --- |
| `E2E_AUTHOR_APP_ID` | Variable |
| `E2E_REVIEWER_APP_ID` | Variable |
| `E2E_AUTHOR_PRIVATE_KEY` | Secret |
| `E2E_REVIEWER_PRIVATE_KEY` | Secret |

- Repository settings or organization settings granted to `reeve-test` work with the workflow.
- The configured Apps use organization settings shared with `reeve` and `reeve-test`; token minting remains scoped to `reeve-test`.
- Environment-scoped settings need that environment added to the live job before dispatch.
- Open Actions, select **Live GitHub E2E**, and run the workflow from `master`.
- The job builds its pinned source before minting tokens and discovers App logins from the token action.
- Tokens enter the Go harness through stdin; Reeve reaches GitHub through a scoped loopback relay and receives only the fixture credential.

## Two Apps

| Identity | Suggested App name | Repository permissions | Purpose |
| --- | --- | --- | --- |
| Author | `reeve-e2e-author` | Contents: write; Pull requests: write | Create fixture commits, branches, and PRs. |
| Reviewer | `reeve-e2e-reviewer` | Pull requests: write | Submit a review for the exact PR head SHA. |

- App names must be available; use the resulting exact `<app-slug>[bot]` login in test configuration.
- Install each App on `reeve-test` only and mint repository-scoped installation tokens when the live job starts.
- Keep webhook delivery disabled if the Apps only serve as API identities.
- Record each App ID and arrange signing-key access through the approved secret store; do not place signing keys or tokens in fixture files or engine environments.
- Keep reviewer signing authority separate from author/workload code.
- A different Git commit author string does not create a second authenticated GitHub reviewer.

## Review policy

- Configure `approvals.sources` with `pr_review` and explicitly allow the reviewer App login in the live fixture's `approvers` list.
- Keep `dismiss_on_new_commit: true` and submit the review with `commit_id` set to the resolved head SHA.
- GitHub's review endpoint accepts installation tokens with Pull requests: write. [GitHub review API](https://docs.github.com/en/rest/pulls/reviews#create-a-review-for-a-pull-request).
- Assert the returned review's author, state, and commit ID before expecting Reeve's gate to pass.
- Test GitHub branch-protection review eligibility separately; a Reeve approval result does not establish that GitHub will permit a merge.

## Command dispatch

- Reeve rejects bot-authored command comments and bot-authored `pr_comment` approvals.
- An App's `/reeve apply` comment must remain a rejected-command test.
- Drive the successful live apply through a trusted explicit workflow invocation that still evaluates normal Reeve gates.
- Give an eventual workflow dispatcher Actions: write only if it actually needs to dispatch; neither initial App requires it for commits or reviews.
- Use a real authorized human for the successful slash-command path until that path has its own live driver.

## Token lifecycle

- Use `actions/create-github-app-token` pinned to a reviewed commit when implementing live jobs.
- Limit each minted token to the fixture repository and required permissions, and revoke it at job completion. [GitHub App token action](https://github.com/actions/create-github-app-token).
- Installation tokens are short-lived; App signing keys remain bootstrap secrets and need separate storage/rotation controls.
- No token or key is needed to enable the current local E2E workflow.

## Live acceptance sequence

1. Author App creates a fixture PR at a recorded head SHA.
2. Preview persists artifacts to the job's temporary filesystem bucket.
3. Apply without approval is blocked.
4. Reviewer App approves that exact head, then the trusted command job applies.
5. Author App pushes another commit and the old approval is rejected.
6. Reviewer requests changes and the gate stays blocked.
7. Cleanup closes the test PR and removes only that scenario's resources/state.

- Author and reviewer tokens must not reach untrusted PR workflow code.
- This workflow keeps all commands in one job; storage shared across separate workflow runs remains a later test lane.
- Reports include `live.json` with the owned PR and branch identifiers; cleanup runs in Go and in an `always()` workflow step.
- A runner lost before cleanup can leave a `reeve-e2e/<run>-<attempt>-<suffix>` branch and PR for manual removal.
