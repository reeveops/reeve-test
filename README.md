# reeve-test

End-to-end test fixture for [reeve](../reeve). Two Pulumi Go projects, the
`random` provider only (so no cloud credentials are needed), and a
`.reeve/` config that points at a local-filesystem state bucket.

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

With [mise](https://mise.jdx.dev) (installs go + pulumi, builds reeve,
inits the backend and all four stacks):

```bash
mise install
mise run setup
```

Day-to-day shortcuts: `mise run lint | stacks | preview | locks | state`,
`mise run tidy | update` for go.mod maintenance, `mise run check` before
pushing, `mise run clean` to fresh-start the fixture. `mise tasks` lists
everything.

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

reeve allows one engine config per root, so the HCL scenarios live under
`tf/` as their own self-contained consumer root with its own `.reeve/`:

```
tf/
├── .reeve/
│   ├── shared.yaml      # same bucket/approvals/break-glass wiring as the root
│   └── tofu.yaml        # engine: tofu; workspace-per-stack model
└── envs/
    ├── random-name/     # random_pet, dev + prod workspaces
    └── random-fail/     # terraform_data + local-exec exit 1; plan clean, apply fails
```

Fully self-contained: the hashicorp/random provider plus the builtin
`terraform_data`, default local state, no cloud credentials. Declared
workspaces are authoritative, so reeve creates them on first use with no
manual `tofu workspace new`.

Local loop (mise installs `opentofu`):

```bash
mise run tf-lint
mise run tf-stacks
mise run tf-preview
```

In CI, [`reeve-tf.yml`](./.github/workflows/reeve-tf.yml) runs the same
action with `root: tf` after an `opentofu/setup-opentofu` step (the reeve
action installs pulumi itself, not tofu). A PR touching only `tf/` maps to
no stacks in the pulumi root and vice versa, so the two workflows coexist.
`engine.type: terraform` with `binary.path: terraform` gives the identical
flow under Terraform instead of OpenTofu (one adapter, two registrations).

Every playbook scenario below also works in the tf root: same approvals
and break-glass config, with `envs/random-name` (prod workspace) for the
blocked and break-glass shots and `envs/random-fail` for the failed apply.

## Scenario playbook (two-user screenshot runs)

First: replace every `YOUR-SECOND-USER` in
[`.reeve/shared.yaml`](./.reeve/shared.yaml) with the second GitHub
account. GitHub refuses self-approval, which this playbook exploits: dev
stacks need 1 approval (the other user can grant it); `*/prod` needs 2
(unreachable with two accounts), keeping prod permanently blocked for the
blocked-gate and break-glass shots.

**1. Happy path.** User A opens a PR touching
`projects/random-name/main.go` (or its dev config). User B approves.
User A comments `/reeve apply`. Screenshot the preview comment, the clear
gate trace, and the applied timeline.

**2. Blocked apply.** PR touching a prod stack config
(`projects/random-name/Pulumi.prod.yaml`). User B approves (1 of 2).
User A comments `/reeve apply`. Screenshot the gate trace with
`approvals 1 of 2` and the blocked notice. Nothing runs.

**3. Break-glass.** Same blocked PR. User A comments:
`/reeve breakglass "prod is down, demo run" apply`
Screenshot the gate trace showing approvals overridden as a warning, the
apply, and the audit entry under `.reeve-state/` (or the bucket).

**4. Failed apply.** PR touching `projects/random-fail/main.go`. User B
approves, User A comments `/reeve apply`. Preview is a clean +1 create;
the create command exits 1, so the apply fails. Screenshot the failed
timeline entry and the failing-stack ref.

**5. Drift.** After an applied state exists, mutate state outside git,
e.g. locally: `cd projects/random-name && pulumi destroy -s dev --yes`
(same backend CI uses). Run the drift workflow (Actions tab, `drift`,
Run workflow). `drift_detected` opens a labeled GitHub issue. Re-apply,
re-run drift, and the issue closes via `drift_resolved`.

## What's intentionally off

- **`require_checks_passing: false`.** Local runs have no CI run to
  check. Even in CI, reeve already skips its own check_run via
  `$GITHUB_WORKFLOW`/`$GITHUB_JOB`, so flipping this to `true` is safe
  — toggle when you're ready to enforce the gate.
- **No real cloud provider.** The `random` provider is enough to
  exercise reeve's preview / apply / lock / redact pipelines without
  spending money. Swap in a real provider later.
