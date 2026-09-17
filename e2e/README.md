# Local E2E

- Run the real Reeve CLI, OpenTofu, Terraform, and Pulumi against a loopback GitHub REST fixture.
- OpenTofu uses the built-in `terraform_data` resource and local engine state.
- Terraform runs a separate create, update, no-op, delete, and saved-plan lifecycle against the same cloud-free fixture shape.
- Pulumi uses component resources, a prebuilt Go program, and a disposable local backend.
- Reeve uses a separate filesystem bucket; both stores survive the scenario's commands and are deleted at completion.
- A separate local S3 contract runs the Reeve AWS SDK adapter against disposable MinIO.
- A separate local GCS contract runs the Reeve Google SDK adapter against `fake-gcs-server`.

## Run

From `reeve-test`, with mise installed:

```bash
mise run e2e
```

- This builds the sibling `../reeve` checkout with that repository's Go toolchain.
- The harness uses Go's standard library and runs with `go test -race -tags=e2e`; ordinary Go tests do not require the E2E binaries.
- The harness uses OpenTofu 1.12.6, Terraform 1.16.2, and Pulumi 3.262.0 without changing the existing demo state.
- Initial tool installation and building need network access; the built-in resource requires no provider download.

To test an already-built candidate:

```bash
mise run e2e:run -- --reeve /absolute/path/to/reeve
```

To select a report location:

```bash
mise run e2e:run -- --report-dir .local/e2e/my-run
```

To run the S3-compatible adapter contract without AWS credentials:

```bash
mise run blob:local
mise run blob:s3-local
mise run blob:gcs-local
```

- `blob:local` runs both contracts concurrently and is the CI entrypoint.

- The report directory must not already exist.
- Missing binaries, unexpected API calls, failed assertions, and timeouts produce a nonzero exit.
- The harness supports Linux and macOS process groups; CI runs on Ubuntu.

## Coverage

| Area | Assertions |
| --- | --- |
| Lifecycle | Create, update, replace, delete, and converged no-op through separate CLI processes. |
| Preview concurrency | Independent OpenTofu projects overlap while workspaces sharing one directory stay serial; one failed stack does not hide three successful stack results. |
| Apply concurrency | A process that outlives the lock TTL keeps its lease through heartbeats; a concurrent run of the same PR stays out of the engine. |
| Lock queue | Three PRs contend for one stack, preserve FIFO order, adopt promoted reservations, and leave no queue entries. |
| Lock recovery | An expired holder is evicted during acquisition; SIGTERM releases a live lock and persists a failed manifest and audit record. |
| Maintenance | A real sweep promotes expired queues, prunes only old artifacts, isolates malformed locks, and performs no writes on a second no-op run. |
| Historical isolation | Preview and apply ignore 2,000 expired malformed artifacts owned by another PR and leave them for explicit maintenance. |
| Saved plans | Preview artifact exists; successful apply executes it without re-planning. |
| Plan modes | Locking off re-plans; missing artifacts fall back; stale plans fail closed. |
| Refresh | Dry-run and writing refreshes plus apply-with-refresh use the intended engine modes. |
| Rerun identity | Two attempts keep distinct manifests and plans; apply selects the newer snapshot, and apply/refresh retain attempt 2 after authoritative head lookup. |
| Terraform | Separate create, update, forced replacement, no-op, delete, and saved-plan apply through the public CLI. |
| Pulumi | Local-backend create, no-op, refresh, delete, and saved-plan apply. |
| Approvals | CODEOWNERS, explicit lists, combined policies, and public unlisted-review opt-in exercise allowed and denied applies. Missing, self, stale, and changes-requested reviews also deny apply. |
| Break-glass | `internal_list`, CODEOWNERS, and `anyone` exercise authorization, denial, justification, intent/completion audits, comments, state, and lock cleanup. |
| Changed commit | Previous-head approval stops counting on a new head. |
| Other gates | Active freeze windows, failing checks, behind-base, draft, and fork classifications deny apply. |
| API failure | Review-read failure exits nonzero and never invokes engine apply. |
| Idempotency | A repeated apply of the same head does not invoke the engine or change state. |
| Failure reporting | Failed preview/apply persists failure artifacts and updates simulated PR comments. |
| Audit/locks | Apply outcomes have audit records; finished/blocked runs leave no lock holder or queue entry. |
| Local S3 contract | Nine storage cases use the real S3 adapter over MinIO, including metadata listing and an explicit unsupported conditional-delete verdict. |
| Local GCS contract | Eight positive cases use the real GCS adapter over `fake-gcs-server`; a ninth negative case proves the emulator ignores delete generation preconditions and is rejected by the strict contract. |

- Gate tests assert that no engine command ran and engine state remained unchanged.
- Blocked apply exits zero by Reeve's contract; the test inspects the failed gate and blocked manifest explicitly.
- The API fixture supplies public-repository and App-shaped identities; no real GitHub identity is impersonated.
- Child environments exclude inherited cloud, GitHub, and local user credentials.

## Reports

- `.local/e2e/<timestamp>/summary.json` records per-command elapsed time, exit codes, engine invocations, and API request deltas.
- Per-command logs, manifests, audit entries, and simulated comments aid failure diagnosis.
- Reports exclude engine state and opaque saved plans; fixtures contain only synthetic data.
- CI uploads `e2e-report/` for seven days, including on failure.

## CI

- [e2e-local.yml](../.github/workflows/e2e-local.yml) runs on PRs, master pushes, and manual dispatch.
- It builds the pinned Reeve commit; manual dispatch can select a candidate commit through `reeve-ref`.
- It grants only `contents: read` and requests no OIDC token or repository secret.
- CI installs only the pinned tools required by the local OpenTofu, Terraform, and Pulumi fixtures.
- CI starts pinned MinIO and `fake-gcs-server` only for their contract steps inside the existing lifecycle job.
- MinIO is an external AGPLv3 test tool; Reeve does not link or distribute it.
- `fake-gcs-server` is an external BSD-2-Clause test tool.
- Shared GitOps, drift, and maintenance callers replace the legacy direct-action demo workflows.

## Boundaries and next layers

- This suite covers the public Reeve CLI, real engines, the filesystem adapter, S3 and GCS adapters over local HTTP, and simulated GitHub REST responses.
- It does not execute the composite action, real webhook routing, actual GitHub reviews, or real cloud services.
- [Live GitHub identity tests](github-apps.md) have a separate manual workflow with two Apps and a filesystem bucket in one job.
- Action routing remains covered separately by workflow tests; this suite owns concurrent process, heartbeat, queue, and cancellation behavior.
- Add AWS/GCP/R2 blob contract lanes after those local checks; a blob lane is separate from the engine's workload provider.
- Separate GitHub events need storage shared across runners; an in-job filesystem cannot provide that.
- `reeve` branch `feat/live-integration-harness` currently adds only `checkout-pr-head: false`; this CLI harness does not need that option.
