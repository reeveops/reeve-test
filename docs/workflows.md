# GitHub Actions workflows

Choose a workflow based on what you need to verify. Local tests, real GitHub identities, and real cloud storage have different setup requirements.

## Choose a workflow

| Workflow | Runs when | What it exercises | Setup |
| --- | --- | --- | --- |
| [Local E2E](../.github/workflows/e2e-local.yml) | Ordinary PRs, pushes to `master`, or manual dispatch. | Real Pulumi, Terraform, and OpenTofu processes with simulated GitHub, plus local S3/GCS adapter contracts. | No GitHub App or cloud credentials. |
| [Guided Reeve Playground](../.github/workflows/playground.yml) | An admitted playground request dispatches it from `master`. | A human-driven PR tour with the selected engine and real App reviews. | [Tour access](playground.md#before-you-begin) and the configured test Apps. |
| [Live GitHub E2E](../.github/workflows/e2e-live.yml) | Manual dispatch from `master`. | A disposable PR, separate App identities, shared-workflow preview checks, and trusted harness applies. | [Two GitHub Apps](../e2e/github-apps.md). |
| [Reeve Shared E2E](../.github/workflows/reeve-shared.yml) | PR events, merge-group checks, and issue comments. | Reeve's reusable GitOps workflow with the `tf/` OpenTofu root. | The tracked fixture configuration; approval placeholders must be adapted for manual use. |
| [Reeve Shared Drift E2E](../.github/workflows/reeve-shared-drift.yml) | Manual dispatch. | The reusable workflow's drift mode for `tf/`. | OpenTofu and job-local filesystem state. |
| [Reeve Shared Maintenance E2E](../.github/workflows/reeve-shared-maintenance.yml) | Manual dispatch. | The reusable workflow's maintenance mode. | No IaC engine installation. |
| [Cloud Blob Contract](../.github/workflows/cloud-blob-contract.yml) | Manual dispatch. | Storage assertions against real AWS S3 and/or GCS. | [Buckets and federation](../e2e/cloud-buckets.md). |

The shared GitOps caller skips guided playground PRs. Local E2E also skips the App-owned live and guided PR branches.
The drift caller is currently manual; add a schedule when adapting it if you want periodic runs.

## Start a manual run

1. Open the repository's **Actions** tab and select the workflow.
2. Select **Run workflow** and choose the intended branch; live and guided jobs require `master`.
3. Supply the inputs described below, then open the run to follow the jobs and download reports.

Use the [playground request links](playground.md#1-choose-an-engine) to start a guided session. The launcher supplies its PR, engine, head SHA, and session-owner inputs after validating the request.

| Workflow input | Meaning |
| --- | --- |
| Local E2E: `reeve-ref` | Reeve source revision to build; defaults to the workflow's pinned candidate. Use a full commit SHA for reproducible results. |
| Cloud Blob Contract: `backend` | `aws`, `gcp`, or `all`; every selected backend must be configured. |
| Cloud Blob Contract: `reeve-ref` | Exact lowercase, 40-character Reeve commit SHA. |

The live and guided workflows build their pinned source before minting App tokens.
The [Start Reeve Playground](../.github/workflows/playground-start.yml) launcher and [nightly cleanup](../.github/workflows/playground-cleanup.yml) handle admission, queueing, and stranded sessions; those are separate from the tour controller.

## What a passing run establishes

The local suite proves the [recorded assertions](../e2e/README.md#coverage) against real engines and simulated GitHub responses.
The live and guided runs add real GitHub review identities. The live harness observes shared-workflow previews and superseded-preview cancellation, then performs its lifecycle applies inside its own job.

The shared callers exercise event routing and engine/setup selection. Their Reeve filesystem bucket and engine state are local to each job.
A successful caller run does not establish persistence across separate runners or a production drift baseline. Use shared persistent storage when adapting these callers to a real deployment.

Cloud contracts exercise storage adapters separately from engine workload providers.
Local emulator tests deliberately detect conditional-delete limitations; real S3/GCS acceptance requires a result from the configured cloud workflow.

## Versions and results

The workflows currently pin Reeve candidate `d31c814640689c2f2e1b0d02d2bc11a80a94faab`.
They use reviewed commit pins for actions. The local and guided engine versions are Pulumi 3.262.0, Terraform 1.16.2, and OpenTofu 1.12.6; the live workflow uses OpenTofu.

The top-level mise tools for manual demos track `latest`, while the E2E tasks select their engine versions explicitly.
A local `mise run e2e` builds your sibling Reeve checkout, which may differ from CI's candidate.

When sharing a screenshot, bug report, or acceptance result, include the harness commit, Reeve commit or binary version, selected engine, and workflow/run link.
A result for one engine or revision does not establish coverage for every later revision.

## Reports and cleanup

Local, live, and guided workflows upload diagnostic artifacts for seven days, including on failure. The cloud contract's assertion output is in its job logs.
[Local report contents](../e2e/README.md#reports) and [live cleanup](../e2e/github-apps.md#reports-and-cleanup) explain what to inspect.

The guided tour closes its owned PR and removes its branch; its nightly sweeper handles expired, stranded sessions while preserving active and queued runs.
The live workflow has its own ownership-checked cleanup. Cloud contracts remove their test objects; provisioning and removing buckets are separate [bootstrap tasks](../e2e/bootstrap/README.md).

## Adapt the shared callers

Start with [Reeve's workflow guide](https://github.com/reeveops/reeve/blob/master/docs/github-actions.md) and use this repository's callers as concrete examples of the GitOps, drift, and maintenance modes.
Select the correct root and engine version, replace the filesystem bucket with persistent storage, and configure the controller/backend/workload identities your repository needs.

Engine and state credentials come from Reeve's auth providers; the callers do not accept long-lived engine-token secrets.
The real AWS/GCP storage contracts use GitHub OIDC after their repository variables are configured.
