# Try Reeve now

Follow a temporary pull request through previews, approvals, applies, and recovery with Pulumi, Terraform, or OpenTofu.
You do not need a local CLI, bucket, or cloud workload credentials.

## Before you begin

The guided playground currently accepts people with **write, maintain, or admin access** to `reeveops/reeve-test`.
Public admission is still disabled while Actions-minute quotas and abuse controls are developed.
If you do not have access, use the [local demos](local-demos.md) or [local automated tests](../e2e/README.md).

Only the person who requests a session can advance it. Allow time to stay with the tour: it stops after five minutes waiting for a command or twenty minutes of controller execution.

## 1. Choose an engine

| Engine | Start a session |
| --- | --- |
| Pulumi | [Request a Pulumi playground](https://github.com/reeveops/reeve-test/issues/new?template=playground-pulumi.yml) |
| Terraform | [Request a Terraform playground](https://github.com/reeveops/reeve-test/issues/new?template=playground-terraform.yml) |
| OpenTofu | [Request an OpenTofu playground](https://github.com/reeveops/reeve-test/issues/new?template=playground-opentofu.yml) |

Submit the prefilled request with its confirmation checked.
The launcher creates an App-owned PR, labels it, closes your request, and replies with links to the PR and controller run.

## 2. Follow the progress comment

Open the PR and wait for setup and the initial preview. The PR links to the Actions run so you can see setup or queue progress.
One progress comment stays updated with the next command to enter.

Copy that command into a new PR comment. You can use `/reeve explain` at the first step, or continue immediately with `/reeve apply` (also accepted as `/reeve up`).

## What you will see

1. A create preview, then a preview with additions, changes, deletions, and replacements.
2. An optional explanation of the selected stack's gates.
3. An apply denied because approval is missing.
4. A real GitHub App approval and a successful local apply.
5. A changes-requested denial.
6. A stack lock blocking apply.
7. An intentional engine failure and its saved diagnostics.
8. A break-glass apply with a required justification.
9. A converged no-op and the final audit summary.

## Commands

Follow the active instruction rather than submitting the whole list at once.

| Command | Use it for |
| --- | --- |
| `/reeve explain reeve-e2e-pulumi/dev` | Explain the Pulumi stack. |
| `/reeve explain playground/default` | Explain the Terraform or OpenTofu stack. |
| `/reeve apply` or `/reeve up` | Try the next apply step. |
| `/playground approve` | Ask the reviewer App to approve during the approval step. |
| `/playground request-changes` | Exercise the changes-requested step. |
| `/reeve breakglass "playground recovery" apply` | Exercise the justified recovery step. |
| `/playground help` | Repeat the current instruction. |
| `/playground finish` | End your session early. |

Commands from other users do not advance the session. If your command belongs to a different stage, the controller replies with the command it expects.

## If the tour does not advance

| What you see | What to do |
| --- | --- |
| No PR was created | Check your repository access and the request confirmation. Admission is limited to this repository. |
| The request says the queue is full | Open a new request after an active session finishes. |
| The PR says setup is running | Follow its controller-run link; engine setup and the first preview happen before the progress comment. |
| The run is queued | Wait for the active tour to finish. One controller runs at a time; admission is capped at 100 active or queued sessions. |
| Your command did not advance the tour | Use the requesting account and follow the latest progress comment; `/playground help` repeats it. |
| A check is red after an intentional failure | Read the progress comment for the next recovery step. If the controller itself failed, inspect the run's logs and diagnostic artifact. |

## State and safety

Terraform and OpenTofu use built-in `terraform_data` resources and local state. Pulumi uses a local `file://` backend and a playground-only passphrase.
No engine receives cloud credentials or creates external workload resources.

The controller runs trusted fixture code and never checks out or executes the temporary PR branch.
It reads your comments and runs Reeve within one job; the ordinary shared GitOps caller skips playground PRs.
This demonstrates the guided flow with real GitHub reviews, while standard event routing and state shared across jobs have separate tests.

## Cleanup

The controller closes its PR and deletes its App-owned branch when the tour finishes, expires, or you use `/playground finish`.
If the runner stops before cleanup, the nightly sweeper removes expired, stranded sessions. Active and queued runs are preserved.

The launcher records a six-hour expiry for stranded PRs; this is separate from the running tour's five-minute command wait and twenty-minute limit.

## Use Reeve in your repository

Continue with [Reeve's getting-started guide](https://github.com/reeveops/reeve/blob/master/docs/getting-started.md) and [shared workflow setup](https://github.com/reeveops/reeve/blob/master/docs/github-actions.md).
For repeatable automated scenarios, see [local E2E coverage](../e2e/README.md#coverage).
