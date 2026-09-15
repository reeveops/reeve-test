# Design

- Implement the harness as Go tests using the standard library and the `e2e` build tag.
- Run the tagged tests with the race detector through mise and CI.

## Execution boundary

- Build Reeve from a sibling checkout or accept an explicit binary path.
- Start a GitHub REST fixture bound to `127.0.0.1` on an ephemeral port.
- Each Reeve command runs as a separate process using the same temporary scenario directory.
- Use OpenTofu's built-in `terraform_data` resource, which needs no provider download or cloud account.

## Isolation

- Construct child environments from an allowlist and give them a temporary home/config directory.
- Never forward developer tokens, cloud credentials, or GitHub Actions credentials into the scenario.
- Keep coordination state separate from engine state and delete both after the test.
- Keep only command output, simulated comments, operation counts, and assertion results in the report directory.

## Assertions

- Inspect Reeve manifests, gate traces, audit entries, applied markers, lock files, and real engine state.
- Trace real engine invocations to prove denied applies never reach `apply` and successful locked applies do not re-plan.
- Treat unexpected API routes, missing binaries, missing artifacts, and incorrect exit codes as failures.

## Live GitHub extension

- Author and reviewer are separate identities; changing commit attribution alone does not change the authenticated PR author.
- Use two GitHub Apps with scoped installation tokens and explicit reviewer allowlisting for live tests.
- Keep bot-command rejection enabled and drive mutating commands through a trusted explicit workflow path.
- Exercise separate-event state sharing only after a shared storage fixture is available.
