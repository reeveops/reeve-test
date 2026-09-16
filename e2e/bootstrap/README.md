# Cloud contract bootstrap

These OpenTofu roots create the disposable AWS and GCP resources used by the manual `Cloud Blob Contract` workflow. They use GitHub OIDC federation and create no cloud access keys.

## Validate

```bash
mise run cloud:validate
```

## AWS

Authenticate OpenTofu with an AWS identity allowed to manage S3, IAM roles, policies, and an IAM OIDC provider.

```bash
E2E_AWS_REGION=us-east-1 \
E2E_S3_BUCKET=YOUR-GLOBALLY-UNIQUE-BUCKET \
mise run cloud:apply:aws
```

The task initializes OpenTofu, presents its normal apply approval, and writes the three repository variables after a successful apply.

The root creates GitHub's IAM OIDC provider by default. If the account already has one, set `create_github_oidc_provider=false` and pass its ARN as `github_oidc_provider_arn`.

## GCP

Authenticate OpenTofu with Application Default Credentials that can manage project services, buckets, service accounts, and Workload Identity Federation.

```bash
E2E_GCP_PROJECT_ID=YOUR-PROJECT \
E2E_GCS_BUCKET=YOUR-GLOBALLY-UNIQUE-BUCKET \
mise run cloud:apply:gcp
```

Set `E2E_GITHUB_REPOSITORY` to reuse either task with another repository.

Both roots restrict federation to immutable owner ID `310228695`, repository ID `1231457834`, and `refs/heads/master`. Bucket access is limited to the disposable contract bucket, and objects under `reeve-test/` expire after seven days.

The defaults match this repository's current GitHub OIDC configuration. Override the owner and repository names and IDs together when reusing the roots elsewhere.

## Run and remove

Run the contract after configuring one or both backends:

```bash
mise run cloud:run
```

Set `E2E_CLOUD_BACKEND=aws` or `gcp` to run one backend. `mise run cloud:configure` refreshes all six GitHub variables from existing state without changing cloud resources.

Keep each root's local OpenTofu state until acceptance is complete. Run `tofu destroy` in that root and delete its three repository variables when the backend is no longer needed.
