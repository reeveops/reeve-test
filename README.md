# reeve-test

End-to-end test fixture and public playground for
[reeve](https://github.com/reeveops/reeve). Pulumi, Terraform, and OpenTofu projects,
local-only providers, no cloud credentials, and local-filesystem state.

[Try Reeve now](docs/playground.md) through a disposable guided pull request using OpenTofu, Terraform, or Pulumi with local-only state.

## Automated E2E first

```bash
mise run e2e
```

- Builds sibling `../reeve` with its own toolchain and runs real Reeve/OpenTofu, Terraform, and Pulumi processes.
- Uses a simulated GitHub API and disposable state; no GitHub, AWS, GCP, or Pulumi Cloud token is needed.
- Verifies create, update, delete, no-op, approval gates, failure persistence, saved plans, and lock cleanup.
- [Local E2E](.github/workflows/e2e-local.yml) runs the same harness in CI and uploads diagnostic reports.
- Local E2E also runs the Reeve S3 and GCS adapter contracts against disposable local servers without cloud credentials.
- [Reeve Shared E2E](.github/workflows/reeve-shared.yml) exercises the pinned reusable GitOps workflow on disposable live PRs.
- [Reeve Shared Drift E2E](.github/workflows/reeve-shared-drift.yml) exercises the same workflow in manual drift mode with OpenTofu and filesystem state.
- [Reeve Shared Maintenance E2E](.github/workflows/reeve-shared-maintenance.yml) exercises the same workflow in manual maintenance mode without installing an IaC engine.
- [Guided playground](docs/playground.md) creates an App-owned PR and exercises approval, denial, locking, failure, break-glass, and cleanup with the selected local engine.
- [Cloud Blob Contract](.github/workflows/cloud-blob-contract.yml) runs the same storage contract against trusted AWS and GCP buckets with federated credentials.
- Read [E2E setup and limits](e2e/README.md) and [live GitHub identity setup](e2e/github-apps.md).
- Read [cloud bucket setup](e2e/cloud-buckets.md) before enabling AWS or GCP contract runs.

## Live GitHub acceptance

- Install the two scoped test Apps described in [e2e/github-apps.md](e2e/github-apps.md).
- Run **Live GitHub E2E** manually from `master`.
- The workflow creates a disposable PR, exercises author and reviewer separation, and removes its branch and PR.
- The Apps mint short-lived installation tokens only after the trusted Reeve source and harness are built.

The ordinary shared-workflow callers use local state to verify routing and setup in one run.
Use the live harness or configured cloud lanes for lifecycle behavior that crosses workflow runs.

## Layout

```
reeve-test/
├── .reeve/
│   ├── shared.yaml      # filesystem bucket, approvals, freeze, etc.
│   └── pulumi.yaml      # engine config: pulumi binary + stack patterns
├── projects/
│   ├── random-name/     # RandomPet generator, dev + prod stacks
│   ├── random-secret/   # RandomPassword generator, dev stack only
│   └── random-fail/     # previews clean, always fails on apply (dev only)
├── go.mod               # module (used by all projects)
└── README.md
```

## One-time setup

With [mise](https://mise.jdx.dev):

```bash
mise install
mise run setup
```

- `setup`: builds sibling reeve, local pulumi backend, inits all four stacks
- Daily: `lint` / `stacks` / `preview` / `locks` / `state`
- Go deps: `tidy` / `update`
- Pre-push: `check`. Fresh start: `clean`. Full list: `mise tasks`

Or by hand:

```bash
# 1. Build reeve from sibling repo (or `go install` into PATH).
( cd ../reeve && go build -o bin/reeve ./cmd/reeve )
export PATH="$PWD/../reeve/bin:$PATH"

# 2. Pulumi local-file backend, scoped to this repo.
pulumi login file://./pulumi-state

# 3. Pull Go deps (Pulumi `runtime: go` shells out to `go run`).
go mod tidy

# 4. Initialise stacks the projects expect.
( cd projects/random-name   && pulumi stack init dev  --secrets-provider=passphrase --non-interactive )
( cd projects/random-name   && pulumi stack init prod --secrets-provider=passphrase --non-interactive )
( cd projects/random-secret && pulumi stack init dev  --secrets-provider=passphrase --non-interactive )
( cd projects/random-fail   && pulumi stack init dev  --secrets-provider=passphrase --non-interactive )

# Set a passphrase env var so non-interactive runs succeed.
export PULUMI_CONFIG_PASSPHRASE="reeve-test"
```

## Exercise reeve

All commands run from the repo root and discover `.reeve/` automatically.

Read-only checks (no Pulumi invocation needed):

```bash
reeve lint                          # validates .reeve/ config
reeve stacks                        # enumerates discovered stacks
reeve rules explain random-name/prod
```

Local preview against all stacks (requires `pulumi` CLI on PATH and the
one-time setup above):

```bash
reeve run preview --local
```

Inspect what reeve wrote:

```bash
ls -R .reeve-state/
```

Locks introspection:

```bash
reeve locks list
reeve locks explain random-name/prod
```

Tip: pass `--log-level debug` to any command for the full slog trace
(check_run inspection, gate evaluation, etc.).

## Run via GitHub Actions

- [Reeve Shared E2E](.github/workflows/reeve-shared.yml) is the copy-ready GitOps caller.
- [Reeve Shared Drift E2E](.github/workflows/reeve-shared-drift.yml) is the scheduled/manual drift caller.
- [Reeve Shared Maintenance E2E](.github/workflows/reeve-shared-maintenance.yml) is the cleanup caller.
- Every caller pins one exact Reeve candidate commit and delegates setup to the maintained reusable workflow.
- Engine and state credentials come from Reeve auth providers; the callers do not accept long-lived engine-token secrets.
- AWS and GCP storage contracts use GitHub OIDC federation after the variables in [cloud bucket setup](e2e/cloud-buckets.md) exist.

## OpenTofu / Terraform scenarios (`tf/`)

reeve allows one engine config per root. The HCL scenarios are a second
consumer root under `tf/`:

```
tf/
├── .reeve/
│   ├── shared.yaml      # same bucket/approvals/break-glass wiring as the root
│   └── tofu.yaml        # engine: tofu; workspace-per-stack model
└── envs/
    ├── random-name/     # random_pet, dev + prod workspaces
    └── random-fail/     # terraform_data + local-exec exit 1; plan clean, apply fails
```

- Self-contained: hashicorp/random + builtin `terraform_data`, local state, no cloud creds
- Declared workspaces are authoritative; reeve creates them on first use
- Local loop: `mise run tf-lint` / `tf-stacks` / `tf-preview` (mise installs `opentofu`)
- CI: [Reeve Shared E2E](.github/workflows/reeve-shared.yml) selects `root: tf`
  and installs the pinned OpenTofu version through the reusable workflow
- A PR touching one root maps to zero stacks in the other. Both workflows coexist
- Terraform instead of OpenTofu: `engine.type: terraform`, `binary.path: terraform`.
  One adapter, two registrations

## Boundaries

- The random provider exercises Reeve without creating paid workload resources.
- Filesystem state is job-local and does not claim cross-run persistence.
- The manual live harness owns the complete GitHub approval lifecycle in one trusted job.
- Real S3 and GCS adapter acceptance remains opt-in until its federated resources are configured.
