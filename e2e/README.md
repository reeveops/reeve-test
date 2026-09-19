# Local E2E

Run the real Reeve CLI with Pulumi, Terraform, and OpenTofu against a simulated GitHub API.
The suite creates disposable projects and state, checks the results, and saves reports for inspection. It does not need GitHub, cloud, or Pulumi Cloud credentials.

For projects you can edit and preview manually, use the separate [local demos](../docs/local-demos.md).

## Before you begin

You need Git, [mise](https://mise.jdx.dev/), and Linux or macOS. The harness uses process groups; CI runs on Ubuntu.
Tool installation and the first Go builds need network access.

The default layout is two sibling checkouts:

```text
workspace/
├── reeve/       # source used to build bin/reeve
└── reeve-test/  # run the commands below here
```

Follow the [first-run setup](../README.md#automated-e2e-first) if you have not cloned and trusted both repositories.
You do not need to initialize the checked-in Pulumi stacks for this suite.

## Run

From `reeve-test`:

```bash
mise run e2e
```

This builds `../reeve` with Reeve's own toolchain, then runs the harness with the race detector.
The task selects Pulumi 3.262.0, Terraform 1.16.2, and OpenTofu 1.12.6. The fixture programs use built-in or component resources, so they need no cloud provider download.

Expect the selected scenarios to pass and report their output directories. The live and guided GitHub drivers are skipped unless explicitly enabled by their workflows.
Missing executables, unexpected API calls, failed assertions, or timeouts make the local run fail.

### Test an existing binary

Skip the build when testing an already-built candidate:

```bash
mise run e2e:run -- --reeve /absolute/path/to/reeve
```

To choose a report directory, give a new path for each run:

```bash
mise run e2e:run -- --reeve /absolute/path/to/reeve --report-dir .local/e2e/my-run
```

`--reeve` and `--report-dir` paths are relative to the repository root unless absolute. The report directory must not already exist.
The optional `--engine`, `--terraform`, and `--pulumi` flags select executable names or paths for OpenTofu, Terraform, and Pulumi respectively.

For a targeted test while iterating:

```bash
mise exec -- go test -race -tags=e2e -count=1 -v ./e2e \
  -run '^TestTerraformLifecycle$' -args --reeve /absolute/path/to/reeve
```

The harness uses Go's standard library and the `e2e` build tag. Ordinary Go tests do not launch these E2E binaries.

### Test local storage adapters

These are separate from `mise run e2e`; CI runs both the storage contracts and the CLI suite:

```bash
mise run blob:local
```

Or select one adapter:

```bash
mise run blob:s3-local
mise run blob:gcs-local
```

The tasks run Reeve's real S3 adapter against disposable MinIO and its GCS adapter against `fake-gcs-server`.
They use the sibling `../reeve` source by default. `blob:local` runs both concurrently and is the CI entry point.

The servers and their data directories are removed when the scripts finish. The scripts use ports 19000–19099 for S3 and 19100–19199 for GCS; set `REEVE_MINIO_PORT` or `REEVE_FAKE_GCS_PORT` if needed.
MinIO is an external AGPLv3 test tool; Reeve does not link or distribute it. `fake-gcs-server` is an external BSD-2-Clause test tool.

A successful emulator run includes expected conditional-delete limitations. It does not certify those servers as suitable shared lock backends; see the coverage details below and the [real cloud contract](cloud-buckets.md).

## How the fixtures work

| Engine | Workload and state |
| --- | --- |
| Pulumi | Component resources, a prebuilt Go program, and a disposable local backend. |
| Terraform | A separate CLI lifecycle using built-in `terraform_data` resources and local engine state. |
| OpenTofu | Built-in `terraform_data` resources, local engine state, and the broader concurrency/gate scenarios. |

Reeve uses its own filesystem bucket alongside the engine's state. Both survive the commands within a scenario and are deleted when the scenario finishes.
The suite leaves the checked-in demos' state alone. Engine coverage varies by scenario; the table below records the assertions rather than implying every test runs on every engine.

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
| Playground engine output | OpenTofu, Terraform, and Pulumi each produce add, change, delete, and replace operations from trusted local-only fixtures. |
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

Each report directory contains `summary.json` with per-command elapsed time, exit codes, engine invocations, and API request changes.
With the default settings, reports appear under `.local/e2e/<timestamp>/`; related scenarios can have their own subdirectories.

| File or output | Use it to inspect |
| --- | --- |
| `summary.json` | Which command failed, its exit code, and whether the engine ran. |
| Per-command logs | Reeve and engine diagnostics. |
| Saved manifests and audit entries | Stack outcomes, gate results, and recorded actions. |
| Simulated PR comments | The output a user would see in GitHub. |

Reports contain synthetic fixture data and exclude engine state and opaque saved plans.
The Local E2E workflow uploads `e2e-report/` and the local blob-contract log for seven days, including on failure.

### Troubleshooting

| Symptom | Next step |
| --- | --- |
| The sibling Reeve checkout is missing | Follow the first-run setup or use `e2e:run` with `--reeve`. |
| An executable is unavailable | Check the selected task's tools and any executable-path overrides. |
| The report directory already exists | Choose a new path; preserve the old directory if you need its diagnostics. |
| Apply exited zero but made no change | Inspect the manifest and gates. A blocked apply can correctly exit zero. |
| A conditional-delete test is red in emulator logs | Read the final script result; the GCS script requires that specific limitation to be detected. An unexpected failure still fails the task. |
| A local server cannot start | Check the script's log and choose a free port with the override above. |

You can remove report directories after inspection; no cloud cleanup is needed for the local suite.

## CI

[Local E2E](../.github/workflows/e2e-local.yml) runs on ordinary PRs, pushes to `master`, and manual dispatch. Owned live/playground PRs are excluded from its PR trigger.
It builds a pinned Reeve commit; manual dispatch can choose a candidate using `reeve-ref`.

That workflow has only `contents: read`, requests no OIDC token or repository secret, and installs the pinned engine and emulator tools it needs.
See the [workflow guide](../docs/workflows.md) for the guided playground, nightly cleanup, shared callers, and live/cloud workflows; their permissions and triggers differ.

## Boundaries and next layers

The local suite covers the public Reeve CLI, real engines, filesystem storage, S3/GCS adapters over local HTTP, and simulated GitHub REST responses.
It does not execute real cloud services.

[Live GitHub identity tests](github-apps.md) and the [guided playground](../docs/playground.md) add real App reviews while keeping engine and Reeve state inside one job.
The live test also observes the shared workflow's preview and cancellation checks. Its own successful applies use the trusted harness, so that result does not establish the ordinary slash-command apply path across separate runners.

The shared GitOps, drift, and maintenance callers replace the older direct-action demo workflows. Action routing and the local suite's process, heartbeat, queue, and cancellation assertions remain distinct coverage.
The original `feat/live-integration-harness` discussion concerned a `checkout-pr-head: false` action option. This CLI harness does not need that proposal; consult the current action's inputs when adapting workflow behavior.

[Real AWS/GCP storage contracts](cloud-buckets.md) are separate from engine workload providers and require configured federation and buckets.
R2 and other providers need their own acceptance work. Separate GitHub events need shared storage; an in-job filesystem cannot provide that.

## Contribute a scenario

Keep a scenario's prerequisites, command, expected result, reports, and cleanup easy to find.
Update the coverage table when assertions change, and record both the Reeve revision and harness revision with shared results.
[Repository checks and maintenance tasks](../docs/local-demos.md#commands-and-maintenance) lists the existing contributor commands.
