# Cloud bucket contract setup

The manual `Cloud Blob Contract` workflow runs the provider-neutral storage contract against real AWS S3 and Google Cloud Storage buckets. It uses GitHub OIDC federation and stores no cloud credential in GitHub.

Use the checked-in [OpenTofu bootstrap](bootstrap/README.md) to create either backend and populate its repository variables. The details below describe the access boundary enforced by those roots.

## AWS

Create a disposable S3 bucket and an IAM role trusted only by `reeveops/reeve-test` on `refs/heads/master`. Grant the role these actions on the bucket and its objects:

- `s3:ListBucket`
- `s3:GetObject`
- `s3:PutObject`
- `s3:DeleteObject`

Set these repository variables:

- `E2E_AWS_ROLE_ARN`
- `E2E_AWS_REGION`
- `E2E_S3_BUCKET`

The repository uses GitHub's immutable OIDC subject format. Restrict `token.actions.githubusercontent.com:sub` to `repo:reeveops@310228695/reeve-test@1231457834:ref:refs/heads/master` and keep the audience restricted to `sts.amazonaws.com`.

## GCP

Create a disposable GCS bucket, Workload Identity Pool provider, and service account restricted to `reeveops/reeve-test`. Grant the service account `roles/storage.objectAdmin` on that bucket only.

Set these repository variables:

- `E2E_GCP_WORKLOAD_IDENTITY_PROVIDER`
- `E2E_GCP_SERVICE_ACCOUNT`
- `E2E_GCS_BUCKET`

Restrict the provider attribute condition and service-account binding to GitHub owner ID `310228695`, repository ID `1231457834`, and `refs/heads/master`.

## Running the contract

Open Actions, select `Cloud Blob Contract`, and choose `aws`, `gcp`, or `all`. The workflow fails during preflight when a selected backend is missing configuration.

Every assertion uses a random child prefix beneath `reeve-test/<run>-<attempt>/` and deletes its objects during test cleanup. Bucket lifecycle expiration should still remove abandoned prefixes from interrupted runs.

## R2

Do not add an R2 parent API token as a repository secret. Add the R2 lane only after a trusted broker can exchange GitHub identity for short-lived, bucket-scoped credentials.
