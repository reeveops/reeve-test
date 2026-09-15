# reeve-test

End-to-end test fixture and public playground for
[reeve](https://github.com/reeveops/reeve). Pulumi + OpenTofu projects,
the `random` provider only, no cloud credentials, local-filesystem state.

## Automated E2E first

```bash
mise run e2e
```

- Builds sibling `../reeve` with its own toolchain and runs real Reeve/OpenTofu processes.
- Uses a simulated GitHub API and disposable state; no GitHub, AWS, GCP, or Pulumi Cloud token is needed.
- Verifies create, update, delete, no-op, approval gates, failure persistence, saved plans, and lock cleanup.
- [Local E2E](.github/workflows/e2e-local.yml) runs the same harness in CI and uploads diagnostic reports.
- Read [E2E setup and limits](e2e/README.md) and [live GitHub identity setup](e2e/github-apps.md).
- The older live demo workflows require the repository variable `REEVE_LIVE_DEMO=true` and their state/identity setup below.

## Live demo after state and identity setup

Open a PR against this repo and watch reeve work:

1. Fork, edit [`tf/envs/random-name/main.tf`](./tf/envs/random-name/main.tf)
   (bump `length`, rename the pet, anything)
2. Open the PR. reeve posts one comment: per-stack plan, gate trace
3. Push again. The same comment rewrites in place
4. Comment `/reeve preview` to re-run, `/reeve help` for the command list

What you'll hit, by design:

- `/reeve apply` is denied: you're not an approver, and fork PRs are
  dry-run-only (`allow_fork_prs: false`)
- Fork PRs run with zero secrets. The `tf/` root needs none; the Pulumi
  root needs `PULUMI_ACCESS_TOKEN`, so target `tf/` for the full loop
- First-time contributors may need a maintainer to approve the workflow run

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

[`.github/workflows/reeve.yml`](./.github/workflows/reeve.yml) wires the
`reeveops/reeve@master` action into the four events that drive the PR
lifecycle: `pull_request`, `pull_request_review`,
`issue_comment` (`/reeve preview|apply|ready|help`), and
`ready_for_review`. The action auto-detects the right subcommand from
the event payload.

**Two state stores need to be persistent for CI to work** — the local
defaults in this fixture are for local-loop iteration only:

| Store               | Local default                  | What CI needs                                   |
| ------------------- | ------------------------------ | ----------------------------------------------- |
| Pulumi stack state  | `pulumi login file://./pulumi-state` | Pulumi Cloud (`PULUMI_ACCESS_TOKEN`) or S3/GCS/Azure |
| Reeve bucket        | `bucket.type: filesystem`      | S3 / GCS / Azure / R2 — see swap below          |

Required repo secrets (Settings → Secrets and variables → Actions):

- `PULUMI_ACCESS_TOKEN` — get one at https://app.pulumi.com/account/tokens
- `PULUMI_CONFIG_PASSPHRASE` — any string; encrypts the `random-secret`
  output so `pulumi up` succeeds non-interactively

To swap reeve's filesystem bucket for S3, edit
[`.reeve/shared.yaml`](./.reeve/shared.yaml):

```yaml
bucket:
  type: s3
  name: <your-bucket>
  region: <region>
```

…and add an `auth.yaml` with a binding that exposes AWS creds via
`aws_oidc` (or whatever provider you use). See `../reeve/docs/auth.md`.

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
- CI: [`reeve-tf.yml`](./.github/workflows/reeve-tf.yml) adds `opentofu/setup-opentofu`
  and passes `root: tf`. The reeve action installs pulumi, not tofu
- A PR touching one root maps to zero stacks in the other. Both workflows coexist
- Terraform instead of OpenTofu: `engine.type: terraform`, `binary.path: terraform`.
  One adapter, two registrations
- Playbook below works here too: `envs/random-name` prod workspace for the blocked
  and break-glass shots, `envs/random-fail` for the failed apply

## Scenario playbook (two-user screenshot runs)

Setup:

- Replace every `YOUR-SECOND-USER` in both `shared.yaml` files
- GitHub refuses self-approval. Dev stacks need 1 approval: user B grants it
- `*/prod` needs 2: unreachable with two accounts. Prod stays blocked, on purpose

**1. Happy path**

- PR touches `projects/random-name/main.go`
- B approves. A comments `/reeve apply`
- Shots: preview comment, clear gate trace, applied timeline

**2. Blocked apply**

- PR touches `projects/random-name/Pulumi.prod.yaml`
- B approves (1 of 2). A comments `/reeve apply`
- Nothing runs
- Shots: `approvals 1 of 2` gate trace, blocked notice

**3. Break-glass**

- Same blocked PR
- A comments `/reeve breakglass "prod is down, demo run" apply`
- Shots: approvals-overridden warning in the trace, the apply, the audit entry in the bucket

**4. Failed apply**

- PR touches `projects/random-fail/main.go`
- B approves. A comments `/reeve apply`
- Plan is a clean +1 create. The create command exits 1. Apply fails
- Shots: failed timeline entry, failing-stack ref

**5. Drift**

- Needs applied state
- Mutate state outside git: `cd projects/random-name && pulumi destroy -s dev --yes`
- Actions tab, `drift`, Run workflow
- `drift_detected` opens a labeled issue
- Re-apply, re-run drift: `drift_resolved` closes it

## What's intentionally off

- **`require_checks_passing: false`.** Local runs have no CI run to
  check. Even in CI, reeve already skips its own check_run via
  `$GITHUB_WORKFLOW`/`$GITHUB_JOB`, so flipping this to `true` is safe
  — toggle when you're ready to enforce the gate.
- **No real cloud provider.** The `random` provider is enough to
  exercise reeve's preview / apply / lock / redact pipelines without
  spending money. Swap in a real provider later.
