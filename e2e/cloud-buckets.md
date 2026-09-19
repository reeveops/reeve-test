# Cloud bucket contract setup

Test Reeve's storage adapters against real AWS S3 or Google Cloud Storage using the manual **Cloud Blob Contract** workflow.
A storage contract is a set of assertions about reads, writes, listing, and conditional operations; it tests the Reeve bucket independently of any IaC workload provider.

For a run with no cloud account, use the [local S3/GCS emulator tests](README.md#test-local-storage-adapters).
Those report emulator limitations and do not replace acceptance against the actual cloud service.

## Before you begin

Choose a disposable bucket and an identity that can access only the required test data.
The workflow uses GitHub OIDC federation; it stores no cloud access key in GitHub.

Use the [OpenTofu bootstrap](bootstrap/README.md) to create resources and populate repository variables, or configure an existing disposable backend using the requirements below.
Run the cloud workflow from `master`, matching the federation trust conditions.

## AWS

Create a disposable S3 bucket and an IAM role trusted by this repository on `refs/heads/master`.
The role needs:

| Action | Scope |
| --- | --- |
| `s3:ListBucket` | The contract bucket, restricted to the test prefix. |
| `s3:GetObject`, `s3:PutObject`, `s3:DeleteObject` | Objects under `reeve-test/` in that bucket. |

Set these Actions repository variables:

| Variable | Value |
| --- | --- |
| `E2E_AWS_ROLE_ARN` | Federated role ARN. |
| `E2E_AWS_REGION` | Bucket region. |
| `E2E_S3_BUCKET` | Bucket name. |

This repository uses GitHub's immutable OIDC subject format. Restrict `token.actions.githubusercontent.com:sub` to:

```text
repo:reeveops@310228695/reeve-test@1231457834:ref:refs/heads/master
```

Keep the audience restricted to `sts.amazonaws.com`.
The [AWS bootstrap](bootstrap/README.md#aws) also handles an existing GitHub IAM OIDC provider.

## GCP

Create a disposable GCS bucket, Workload Identity Pool provider, and service account.
Grant that service account `roles/storage.objectAdmin` on the contract bucket only.

| Variable | Value |
| --- | --- |
| `E2E_GCP_WORKLOAD_IDENTITY_PROVIDER` | Full Workload Identity provider resource name. |
| `E2E_GCP_SERVICE_ACCOUNT` | Service account email. |
| `E2E_GCS_BUCKET` | Bucket name. |

Restrict federation using immutable GitHub owner ID `310228695`, repository ID `1231457834`, and `refs/heads/master`.
The [GCP bootstrap](bootstrap/gcp/main.tf) applies those conditions to the provider and binds the service account to the repository identity.

## Running the contract

1. Open **Actions → Cloud Blob Contract → Run workflow** from `master`.
2. Choose `aws`, `gcp`, or `all`. Every selected backend needs its three variables.
3. Choose the exact Reeve commit using `reeve-ref`; the input must be a full lowercase, 40-character SHA.
4. Check each selected provider job's assertions and final result.

The workflow fails preflight if selected configuration is missing or the commit input is malformed.
It runs the same provider-neutral contract against the selected real adapter and service.

The [bootstrap tasks](bootstrap/README.md#run-and-remove) offer command-line equivalents for variable configuration and dispatch.
Keep the run link, harness revision, and Reeve revision with acceptance results.

## Cleanup

Each assertion uses a random child prefix beneath `reeve-test/<run>-<attempt>/` and deletes its own objects during cleanup.
Keep bucket lifecycle expiration enabled to remove abandoned data from interrupted runs.

Tests do not delete the bucket or federation resources. When they are no longer needed, follow the [bootstrap removal steps](bootstrap/README.md#run-and-remove).

## Troubleshooting

| Symptom | What to check |
| --- | --- |
| Preflight reports missing variables | Configure all three variables for every selected backend, or select only the configured backend. |
| Federation is denied | Check the branch, repository/owner IDs, OIDC subject or attribute conditions, and AWS audience. |
| Object or list operations are denied | Check the bucket role and `reeve-test/` prefix scope. |
| Conditional operations fail | Read the specific assertion. A passing local emulator test can include an expected limitation; a real-service acceptance failure needs investigation. |

## R2

R2 remains a separate future test lane. Do not add an R2 parent API token as a repository secret.
Add the lane when a trusted broker can exchange GitHub identity for short-lived, bucket-scoped credentials.
