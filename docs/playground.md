# Try Reeve now

The guided playground creates a temporary pull request and lets you use Reeve without installing a CLI, configuring a bucket, or supplying cloud credentials.

Choose an engine:

- [Try Reeve with OpenTofu](https://github.com/reeveops/reeve-test/issues/new?template=playground-opentofu.yml)
- [Try Reeve with Terraform](https://github.com/reeveops/reeve-test/issues/new?template=playground-terraform.yml)
- [Try Reeve with Pulumi](https://github.com/reeveops/reeve-test/issues/new?template=playground-pulumi.yml)

Submit the prefilled request. Reeve Test creates and labels the playground PR, closes the request, and replies with the PR and controller-run links.

The PR explains that setup is running and links to the exact Actions run. Reeve posts the guided progress comment after setup and the initial preview finish.

## What you will see

The PR guides you through:

1. An initial create preview followed by a preview containing additions, changes, deletions, and replacements.
2. `/reeve explain` output for the selected stack.
3. An apply denied by approval policy.
4. A real GitHub App approval followed by a successful local-only apply.
5. A changes-requested denial.
6. An apply blocked by a stack lock.
7. A deterministic engine failure and its persisted diagnostics.
8. A justification-gated break-glass apply.
9. A converged no-op and final audit summary.

The controller keeps one progress comment updated with the next command. Commands from other users and commands outside the active stage are ignored.

## State and safety

- OpenTofu and Terraform use local state with built-in `terraform_data` resources.
- Pulumi uses a local `file://` backend and a playground-only passphrase.
- No engine receives cloud credentials or creates external resources.
- The workflow never checks out or executes the playground PR branch.
- A session stops after five idle minutes or twenty total minutes.
- The PR closes when the session finishes or expires.
- A nightly cleanup workflow closes stranded PRs carrying the `reeve-playground` label.

The first version accepts repository collaborators. Public admission will remain disabled until Actions-minute quotas and abuse controls are defined.

## Commands

Follow the progress comment. The tour uses these commands:

```text
/reeve explain playground/default       # OpenTofu and Terraform
/reeve explain reeve-e2e-pulumi/dev     # Pulumi
/reeve apply                            # `/reeve up` also works
/playground approve
/playground request-changes
/reeve breakglass "playground recovery" apply
/playground finish
```

The explain step is optional. Use `/reeve apply` or `/reeve up` after the initial preview to continue immediately.

Use `/playground help` at any stage to repeat the current instruction.
Commands from another stage get an immediate reply with the command currently expected.

## Cleanup

The session closes its PR when complete. `/playground finish` closes it immediately.

If a runner stops before cleanup, the nightly sweeper closes the labelled PR and deletes its App-owned branch.

After the tour, use Reeve's reusable workflow guide to add the same flow to a real repository.
