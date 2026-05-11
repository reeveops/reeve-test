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
│   └── random-secret/   # RandomPassword generator, dev stack only
├── go.mod               # module (used by both projects)
└── README.md
```

## One-time setup

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
`FynxLabs/reeve@master` action into the four events that drive the PR
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

## What's intentionally off

- **`require_checks_passing: false`.** Local runs have no CI run to
  check. Even in CI, reeve already skips its own check_run via
  `$GITHUB_WORKFLOW`/`$GITHUB_JOB`, so flipping this to `true` is safe
  — toggle when you're ready to enforce the gate.
- **No real cloud provider.** The `random` provider is enough to
  exercise reeve's preview / apply / lock / redact pipelines without
  spending money. Swap in a real provider later.
