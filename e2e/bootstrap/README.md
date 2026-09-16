# Cloud contract bootstrap

These OpenTofu roots create the disposable AWS and GCP resources used by the manual `Cloud Blob Contract` workflow. They use GitHub OIDC federation and create no cloud access keys.

## Validate

```bash
mise run cloud:validate
```

## AWS

Authenticate OpenTofu with an AWS identity allowed to manage S3, IAM roles, policies, and an IAM OIDC provider.

```bash
tofu -chdir=e2e/bootstrap/aws apply \
  -var='region=us-east-1' \
  -var='bucket_name=YOUR-GLOBALLY-UNIQUE-BUCKET'

e2e/bootstrap/configure-github.sh aws
```

The root creates GitHub's IAM OIDC provider by default. If the account already has one, set `create_github_oidc_provider=false` and pass its ARN as `github_oidc_provider_arn`.

## GCP

Authenticate OpenTofu with Application Default Credentials that can manage project services, buckets, service accounts, and Workload Identity Federation.

```bash
tofu -chdir=e2e/bootstrap/gcp apply \
  -var='project_id=YOUR-PROJECT' \
  -var='bucket_name=YOUR-GLOBALLY-UNIQUE-BUCKET'

e2e/bootstrap/configure-github.sh gcp
```

Both roots restrict federation to immutable owner ID `310228695`, repository ID `1231457834`, and `refs/heads/master`. Bucket access is limited to the disposable contract bucket, and objects under `reeve-test/` expire after seven days.

The defaults match this repository's current GitHub OIDC configuration. Override the owner and repository names and IDs together when reusing the roots elsewhere.

## Run and remove

Run the contract after configuring one or both backends:

```bash
gh workflow run cloud-blob-contract.yml \
  --repo reeveops/reeve-test \
  -f backend=all \
  -f reeve-ref=9cf21fa3de74c5c877ac72630e76e76fa12479ac
```

Keep each root's local OpenTofu state until acceptance is complete. Run `tofu destroy` in that root and delete its three repository variables when the backend is no longer needed.
