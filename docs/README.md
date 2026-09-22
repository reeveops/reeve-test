# reeve-test documentation

Start with what you want to try. Setup and detailed test coverage are available in the linked guides.

## Try Reeve

- [Guided playground](playground.md): follow a temporary PR through preview, approval, apply, and recovery. Currently requires repository write access.
- [Local demos](local-demos.md): explore Pulumi, OpenTofu, or Terraform projects on your machine.
- [Use Reeve in your repository](https://github.com/reeveops/reeve/blob/master/docs/getting-started.md): move from the playground to a persistent deployment setup.

## Run tests

| Guide | Use it for |
| --- | --- |
| [Local E2E](../e2e/README.md) | Run automated scenarios, select a Reeve binary, read reports, and inspect coverage. |
| [Workflow guide](workflows.md) | Choose a GitHub Actions workflow and understand its inputs, state, and results. |
| [GitHub identities](../e2e/github-apps.md) | Configure author/reviewer Apps and run the live approval lifecycle. |
| [Cloud bucket contracts](../e2e/cloud-buckets.md) | Test real AWS S3 and GCS adapters using federation. |
| [Cloud bootstrap](../e2e/bootstrap/README.md) | Provision, configure, and remove the disposable cloud test resources. |

## Look something up

- [Scenario coverage](../e2e/README.md#coverage): assertions and engine coverage.
- [Reports and troubleshooting](../e2e/README.md#reports): find a failed command, gate, or engine invocation.
- [Local demo commands](local-demos.md#commands-and-maintenance): mise tasks, state, and reset behavior.
- [Version pins](workflows.md#versions-and-results): identify the exact Reeve and harness sources a result tested.

The [OpenSpec change records](../openspec/changes/) describe the original local and live test designs. The current workflows and scenario source define what runs today.
