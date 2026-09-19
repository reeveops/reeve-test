# Local demos

Explore Reeve's checked-in projects with Pulumi, OpenTofu, or Terraform.
These demos use local state and need no cloud workload credentials. The random and command providers may need an initial download.

For a repeatable test run that creates and cleans its own fixtures, use [local E2E](../e2e/README.md).
For a guided approval/apply flow in GitHub, use the [playground](playground.md).

## Before you begin

Install [mise](https://mise.jdx.dev/) and use the [sibling checkout layout](../README.md#automated-e2e-first).
Run the commands below from `reeve-test`, unless a step changes directories.

The root `.reeve/` configures Pulumi. The `tf/.reeve/` directory configures OpenTofu, with instructions below for switching that demo to Terraform.
Each engine keeps its own workload state; Reeve's `.reeve-state/` directories hold locks, plans, run records, and audit entries.

## Pulumi

### Projects and expected behavior

| Project | Stacks | What it demonstrates |
| --- | --- | --- |
| [`random-name`](../projects/random-name/) | `dev`, `prod` | A RandomPet name and multiple stacks in one project. |
| [`random-secret`](../projects/random-secret/) | `dev` | A RandomPassword and secret-output handling. |
| [`random-fail`](../projects/random-fail/) | `dev` | A clean create preview followed by an intentional apply failure. |

The project programs use the repository's Go module. `random-fail` is supposed to fail on apply so you can inspect and capture failure reporting.

### Configure local state

Use a disposable checkout. Prepare an isolated Pulumi home and an absolute backend URL so every project directory uses the same local state:

```bash
mise install
export PULUMI_HOME="$PWD/.local/pulumi-home"
export PULUMI_BACKEND_URL="file://$PWD/pulumi-state"
mkdir -p "$PULUMI_HOME" "$PWD/pulumi-state"
pwd
```

In [`.reeve/pulumi.yaml`](../.reeve/pulumi.yaml), add this `state` block inside `engine`, alongside `binary` and `stacks`. Replace `/ABSOLUTE/PATH/reeve-test` with the directory printed above, keeping `file://` and `/pulumi-state`:

```yaml
  state:
    backend: file
    url: file:///ABSOLUTE/PATH/reeve-test/pulumi-state
    secrets_provider:
      type: passphrase
      passphrase: reeve-test
```

This is a local demo override; keep the other engine settings. The passphrase is public fixture data.
Reeve builds its engine environment explicitly, so shell exports alone do not supply its backend or passphrase. The YAML settings and shell URL must identify the same directory.
Keep the exported paths when opening a new shell, and update the YAML path if you move the checkout.

### Initialize the stacks

After configuring local state, run these steps against a fresh demo backend. An already initialized stack does not need `stack init` again.

```bash
mise run reeve-build
export PATH="$PWD/../reeve/bin:$PATH"
export PULUMI_CONFIG_PASSPHRASE="reeve-test"
mise exec -- pulumi login "$PULUMI_BACKEND_URL"
mise exec -- go mod download

(cd projects/random-name && mise exec -- pulumi stack init dev --secrets-provider=passphrase --non-interactive)
(cd projects/random-name && mise exec -- pulumi stack init prod --secrets-provider=passphrase --non-interactive)
(cd projects/random-secret && mise exec -- pulumi stack init dev --secrets-provider=passphrase --non-interactive)
(cd projects/random-fail && mise exec -- pulumi stack init dev --secrets-provider=passphrase --non-interactive)
```

The existing `mise run setup` task is a shortcut for the build, dependency, login, and initialization steps after you configure local state. It uses a relative login URL, so Pulumi may warn that it differs from the exported absolute URL. It also suppresses stack-initialization errors; use the explicit steps above when diagnosing setup problems.
The repository's mise configuration supplies the demo passphrase and adds the sibling binary to `PATH`.

### Inspect and preview

These checks read configuration without invoking Pulumi:

```bash
mise run lint
mise run stacks
mise exec -- reeve rules explain random-name/prod
```

Expect four discovered stacks, as listed above. Preview them with:

```bash
mise run preview
```

The task runs `reeve run preview --local`. It executes the real engine and writes local Reeve artifacts without posting to GitHub.
Preview results include the name generator, secret generator, and intentional-failure project; the latter fails only when applied.

Inspect state and locks:

```bash
mise run state
mise run locks
mise exec -- reeve locks explain random-name/prod
```

The equivalent direct commands are `reeve lint`, `reeve stacks`, `reeve run preview --local`, and `reeve locks list` when Reeve and Pulumi are on `PATH`.
`ls -R .reeve-state/` shows the artifact directory. Add `--log-level debug` to a Reeve command for more detail about checks and gate evaluation.

## OpenTofu and Terraform

### Projects and workspaces

| Project in `tf/envs/` | Workspaces | What it demonstrates |
| --- | --- | --- |
| [`random-name`](../tf/envs/random-name/) | `dev`, `prod` | `random_pet` with local state. |
| [`random-fail`](../tf/envs/random-fail/) | `default` | Built-in `terraform_data` with a provisioner that intentionally fails on apply. |
| `lifecycle` | `default` | Declared in the config; the live harness creates this project on its temporary PR branch. It is absent from a clean default-branch checkout. |

Declared workspaces are authoritative: Reeve creates missing declared workspaces when it uses an existing project.
In a clean checkout, `tf-stacks` lists the three stacks from `random-name` and `random-fail`.

### Preview with OpenTofu

After building the sibling Reeve binary:

```bash
mise run tf-lint
mise run tf-stacks
mise run tf-preview
```

These tasks run from `tf/`. Its [`tofu.yaml`](../tf/.reeve/tofu.yaml) selects OpenTofu, and its Reeve filesystem bucket is `tf/.reeve-state/`.
The random provider may be downloaded during initialization; no cloud credentials are needed.

### Use Terraform instead

In a disposable copy of the demo, rename `tf/.reeve/tofu.yaml` to `tf/.reeve/terraform.yaml` and set:

```yaml
engine:
  type: terraform
  binary:
    path: terraform
```

Keep the remaining configuration in that file. Then run the same `tf-lint`, `tf-stacks`, and `tf-preview` tasks; they invoke Reeve, which selects the configured engine.
The shared Terraform/OpenTofu adapter has separate engine registrations, and the automated suite runs each binary independently.
If adapting the Actions caller as well, replace `opentofu_version` with `terraform_version` and select its version.

## Approval and failure demonstrations

Both roots retain the two-user approval and break-glass setup in their `shared.yaml` files.
Replace `YOUR-SECOND-USER` with the other authorized demo user when testing reviews in your own setup.
Development stacks require one approval. Production stacks require two; with two people and no self-approval, production remains blocked deliberately so you can inspect the gate and demonstrate an authorized override.

The configuration includes an explicit break-glass list and requires a justification. Overrides do not bypass locks, checks, or plan freshness.
The [guided tour](playground.md) supplies its own trusted fixtures and App identities for an interactive version of these scenarios.

## Commands and maintenance

| Task | Purpose |
| --- | --- |
| `mise run setup` | Build and initialize the Pulumi demos after configuring local state above. |
| `mise run reeve-build` | Rebuild the sibling Reeve binary. |
| `mise run lint`, `stacks`, `preview`, `locks`, `state` | Inspect or run the Pulumi root. Use the full `mise run` prefix for each task. |
| `mise run tf-lint`, `tf-stacks`, `tf-preview` | Inspect or run the HCL root. |
| `mise run fmt` or `mise run format` | Format Go source. |
| `mise run vet` | Check Go source. |
| `mise run tidy` | Update the module's dependency metadata. |
| `mise run update` | Upgrade the Go toolchain and dependencies. |
| `mise run e2e:scripts` | Check shell syntax and playground lifecycle tests. |
| `mise run check` | Run the existing pre-push checks, including both roots' configuration and discovery. |
| `mise tasks` | Show the complete task list, including E2E and cloud tasks. |

### Cleanup

`mise run clean` deletes the root `.reeve-state/` and `pulumi-state/` directories. It resets those local demos; it does not destroy provisioned cloud resources.
It does not remove `tf/` state, `.local/` reports, or the isolated Pulumi home.

Use a disposable checkout if you want to discard all HCL state and provider caches together. Keep state you need for further inspection before resetting it.
The [cloud bootstrap guide](../e2e/bootstrap/README.md#run-and-remove) has separate cleanup instructions for actual cloud test resources.

### If setup or preview fails

| Symptom | Next step |
| --- | --- |
| Reeve cannot be found | Build the sibling checkout and run through mise, or add `../reeve/bin` to `PATH`. |
| Pulumi uses the wrong backend or cannot find a stack | Check `engine.state.url` as well as the exported URL and Pulumi home. Inspect `mise exec -- pulumi stack ls` in that project and rerun the required initialization. |
| Pulumi asks for a passphrase | Set both the initialization passphrase and the explicit `engine.state.secrets_provider` settings above. |
| `random-fail` fails during apply | This is the intended failure fixture; inspect its Reeve result and diagnostics. |
| The HCL root does not list `lifecycle/default` | That project is created by the live harness on its fixture PR; it is not present in the default checkout. |
