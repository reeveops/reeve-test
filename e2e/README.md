# Local E2E

- Run the real Reeve CLI and OpenTofu against a loopback GitHub REST fixture.
- OpenTofu uses the built-in `terraform_data` resource and local engine state.
- Reeve uses a separate filesystem bucket; both stores survive the scenario's commands and are deleted at completion.

## Run

From `reeve-test`, with mise installed:

```bash
mise run e2e
```

- This builds the sibling `../reeve` checkout with that repository's Go toolchain.
- The harness uses Go's standard library and runs with `go test -race -tags=e2e`; ordinary Go tests do not require the E2E binaries.
- The harness uses OpenTofu 1.12.6 and does not initialize or change the existing Pulumi/Tofu demo state.
- Initial tool installation and building need network access; the built-in resource requires no provider download.

To test an already-built candidate:

```bash
mise run e2e:run -- --reeve /absolute/path/to/reeve
```

To select a report location:

```bash
mise run e2e:run -- --report-dir .local/e2e/my-run
```

- The report directory must not already exist.
- Missing binaries, unexpected API calls, failed assertions, and timeouts produce a nonzero exit.
- The harness supports Linux and macOS process groups; CI runs on Ubuntu.

## Coverage

| Area | Assertions |
| --- | --- |
| Lifecycle | Create, update, delete, and converged no-op through separate CLI processes. |
| Preview concurrency | Independent OpenTofu projects overlap while workspaces sharing one directory stay serial. |
| Saved plans | Preview artifact exists; successful apply executes it without re-planning. |
| Approvals | Missing, self, unlisted, stale, and changes-requested reviews deny apply. |
| Changed commit | Previous-head approval stops counting on a new head. |
| Other gates | Failing checks, behind-base, draft, and fork classifications deny apply. |
| API failure | Review-read failure exits nonzero and never invokes engine apply. |
| Idempotency | A repeated apply of the same head does not invoke the engine or change state. |
| Failure reporting | Failed preview/apply persists failure artifacts and updates simulated PR comments. |
| Audit/locks | Apply outcomes have audit records; finished/blocked runs leave no lock holder or queue entry. |

- Gate tests assert that no engine command ran and engine state remained unchanged.
- Blocked apply exits zero by Reeve's contract; the test inspects the failed gate and blocked manifest explicitly.
- The API fixture supplies public-repository and App-shaped identities; no real GitHub identity is impersonated.
- Child environments exclude inherited cloud, GitHub, and local user credentials.

## Reports

- `.local/e2e/<timestamp>/summary.json` records scenarios, exit codes, engine invocations, and API counts.
- Per-command logs, manifests, audit entries, and simulated comments aid failure diagnosis.
- Reports exclude engine state and opaque saved plans; fixtures contain only synthetic data.
- CI uploads `e2e-report/` for seven days, including on failure.

## CI

- [e2e-local.yml](../.github/workflows/e2e-local.yml) runs on PRs, master pushes, and manual dispatch.
- It builds the pinned Reeve commit; manual dispatch can select a candidate commit through `reeve-ref`.
- It grants only `contents: read` and requests no OIDC token or repository secret.
- CI replaces the demo tool configuration on its disposable checkout with Go/OpenTofu only.
- Existing live demo workflows are opt-in through `REEVE_LIVE_DEMO=true`; keep them off until their shared state and credentials exist.

## Boundaries and next layers

- This suite covers the public Reeve CLI, real engine, real filesystem adapter, and simulated GitHub REST responses.
- It does not execute the composite action, real webhook routing, actual GitHub reviews, or cloud SDK storage calls.
- [Live GitHub identity tests](github-apps.md) have a separate manual workflow with two Apps and a filesystem bucket in one job.
- Follow with Pulumi local-backend coverage, action routing tests, and concurrent processes/heartbeat tests.
- Add AWS/GCP/R2 blob contract lanes after those local checks; a blob lane is separate from the engine's workload provider.
- Separate GitHub events need storage shared across runners; an in-job filesystem cannot provide that.
- `reeve` branch `feat/live-integration-harness` currently adds only `checkout-pr-head: false`; this CLI harness does not need that option.
