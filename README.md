# reeve-test

**Try Reeve and test its behavior with Pulumi, Terraform, and OpenTofu.**

This repository is [Reeve's](https://github.com/reeveops/reeve) evolving playground and end-to-end (E2E) test harness.
The local demos and guided tour use local state and create no cloud workload resources. Separate, optional tests exercise real GitHub identities and cloud storage.

## Start here

| I want to… | Start here | What I need |
| --- | --- | --- |
| Try Reeve in a guided pull request | [Guided playground](docs/playground.md) | Write, maintain, or admin access to this repository; no local installation. |
| Run automated tests on my machine | [Local E2E](e2e/README.md) | Git, mise, and a Reeve checkout or binary; no GitHub or cloud credentials. |
| Explore the checked-in projects by hand | [Local demos](docs/local-demos.md) | Reeve and the selected engine. |
| Test real GitHub approvals | [Live GitHub setup](e2e/github-apps.md) | Repository access and two scoped GitHub Apps. |
| Test real S3 or GCS storage | [Cloud bucket setup](e2e/cloud-buckets.md) | A disposable bucket and federated identity. |

To add Reeve to your own infrastructure repository, use [Reeve's getting-started guide](https://github.com/reeveops/reeve/blob/master/docs/getting-started.md).

## Automated E2E first

For a first local run, install [mise](https://mise.jdx.dev/) and clone the repositories side by side:

```bash
git clone https://github.com/reeveops/reeve.git
git clone https://github.com/reeveops/reeve-test.git
cd reeve
mise trust
cd ../reeve-test
mise trust
mise run e2e
```

The task builds the sibling Reeve checkout, runs real engine processes against simulated GitHub responses, and writes diagnostic reports.
It uses disposable state and leaves the checked-in demos' state alone. Initial tool and dependency downloads need network access.

[Choose a candidate, read reports, or find a scenario](e2e/README.md).
Local S3/GCS emulator tests are separate commands, also included in the Local E2E workflow.

## Live GitHub acceptance

The manual **Live GitHub E2E** workflow creates a disposable PR and tests separate author and reviewer identities, approval changes, and cleanup.
[Set up the two Apps and run the workflow](e2e/github-apps.md).

The [guided playground](docs/playground.md) uses the same Apps for a human-driven tour of all three engines.
Access requirements and the first command are in the tour guide.

## Layout

```text
reeve-test/
├── .reeve/            # Pulumi demo configuration
├── projects/          # Random name, secret, and intentional-failure demos
├── tf/                # Separate OpenTofu root; can be adapted to Terraform
├── playground/        # Engine selectors for temporary guided PRs
├── e2e/               # Automated scenarios, fixtures, and cloud bootstrap
├── docs/              # Guided tour, local demos, and workflow guide
└── .github/workflows/ # Local, live, guided, shared, and cloud test workflows
```

## One-time setup

The automated suite prepares its own fixtures. To work with the checked-in Pulumi projects instead, follow [local demo setup](docs/local-demos.md#pulumi).
That guide retains both the mise setup task and the manual setup steps.

## Exercise reeve

[Local demos](docs/local-demos.md#inspect-and-preview) covers configuration checks, stack discovery, previews, locks, state inspection, debugging, and cleanup.

## OpenTofu / Terraform scenarios (`tf/`)

Reeve loads one engine per root. The repository root demonstrates Pulumi; `tf/` is the OpenTofu demo root.
[HCL demo instructions](docs/local-demos.md#opentofu-and-terraform) explain the projects, workspaces, and how to select Terraform. The automated suite tests Terraform and OpenTofu separately.

## Run via GitHub Actions

[Workflow guide](docs/workflows.md) explains which workflow to run, what it exercises, its source pin, and where to find results.
It includes the shared GitOps, drift, and maintenance callers as well as the local, guided, live, and cloud tests.

## Boundaries

- Local fixtures use built-in, component, or random resources and need no cloud workload credentials.
- Filesystem state lasts for one local scenario or job. Separate Actions runs need shared storage to prove persistence between runs.
- Live GitHub tests and the tour add real App reviews while keeping engine state in one job.
- Real S3/GCS tests require the configured cloud resources. Emulator results include explicit conditional-delete limitations.

The harness is still expanding. Use the [coverage table](e2e/README.md#coverage) and recorded results for the scenario and Reeve version you need.

[Browse the documentation](docs/README.md).
