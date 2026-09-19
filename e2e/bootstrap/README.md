# Cloud contract bootstrap

Create and remove the disposable AWS or GCP resources used by the [cloud bucket contract](../cloud-buckets.md).
These OpenTofu roots use GitHub OIDC federation and create no cloud access keys.

## Before you begin

Run commands from the `reeve-test` repository root with mise installed.
You need a cloud identity with provisioning permissions and an authenticated GitHub CLI identity allowed to set the destination repository's Actions variables.

Choose AWS, GCP, or both. The `cloud:apply:*` tasks create real resources, present OpenTofu's normal apply approval, and then write the corresponding GitHub variables.
Keep each root's local OpenTofu state until you have removed its resources.

## Validate

```bash
mise run cloud:validate
```

This initializes the providers with backend setup disabled, validates both roots, and checks the variable-configuration script's syntax. It does not apply resources.
Use `mise run cloud:fmt` when changing the OpenTofu files.

## AWS

Authenticate OpenTofu with an AWS identity allowed to manage S3, IAM roles and policies, and an IAM OIDC provider.
Choose a globally unique test bucket name:

```bash
export E2E_AWS_REGION=us-east-1
export E2E_S3_BUCKET=YOUR-GLOBALLY-UNIQUE-BUCKET
mise run cloud:apply:aws
```

The task initializes and applies the AWS root, then writes `E2E_AWS_ROLE_ARN`, `E2E_AWS_REGION`, and `E2E_S3_BUCKET` to GitHub.
The bucket blocks public access, has versioning enabled, and restricts the workflow role to the `reeve-test/` prefix.

### Reuse an existing GitHub OIDC provider

The root creates GitHub's IAM OIDC provider by default. If your account already has one, supply its ARN before applying:

```bash
export TF_VAR_create_github_oidc_provider=false
export TF_VAR_github_oidc_provider_arn=arn:aws:iam::YOUR-ACCOUNT:oidc-provider/token.actions.githubusercontent.com
mise run cloud:apply:aws
```

## GCP

Authenticate OpenTofu with Application Default Credentials that can manage project services, buckets, service accounts, and Workload Identity Federation:

```bash
export E2E_GCP_PROJECT_ID=YOUR-PROJECT
export E2E_GCS_BUCKET=YOUR-GLOBALLY-UNIQUE-BUCKET
mise run cloud:apply:gcp
```

The task initializes and applies the GCP root, then writes `E2E_GCP_WORKLOAD_IDENTITY_PROVIDER`, `E2E_GCP_SERVICE_ACCOUNT`, and `E2E_GCS_BUCKET` to GitHub.
The service account has object-admin access only on the disposable contract bucket.

## Repository and federation settings

The defaults match `reeveops/reeve-test`: immutable owner ID `310228695`, repository ID `1231457834`, and `refs/heads/master`.
Both roots limit the federated identity to this repository and branch. Objects under `reeve-test/` expire after seven days.

To reuse the bootstrap elsewhere, set `E2E_GITHUB_REPOSITORY=owner/repository` for the GitHub variable destination **and** override the OpenTofu owner/repository names and IDs together.
Changing the destination variable alone does not change federation trust.

| OpenTofu variable | Meaning |
| --- | --- |
| `github_owner`, `github_repository` | Names used by the target repository. |
| `github_owner_id`, `github_repository_id` | Immutable IDs for that owner and repository. |
| `github_ref` | Allowed ref, defaulting to `refs/heads/master`. |
| AWS: `use_immutable_github_subject` | Whether to use the immutable subject format; match the target repository's actual OIDC settings. |

Pass overrides through `TF_VAR_*` environment variables or a local `.tfvars` file.
[Full AWS variables](aws/variables.tf) and [full GCP variables](gcp/variables.tf) include optional role, service-account, pool, provider, and location settings.

## Run and remove

### Run the contract

After configuring both backends:

```bash
mise run cloud:run
```

For one backend, choose it explicitly:

```bash
E2E_CLOUD_BACKEND=aws mise run cloud:run
E2E_CLOUD_BACKEND=gcp mise run cloud:run
```

`E2E_REEVE_REF` selects the exact Reeve commit; the default matches the workflow's pinned candidate.
`E2E_GITHUB_REPOSITORY` selects the destination repository for dispatch as well as variable updates.

### Refresh GitHub variables

```bash
mise run cloud:configure
```

The default reads both roots' existing state and refreshes all six variables without applying resources.
If you provisioned only one backend, set `E2E_CLOUD_BACKEND=aws` or `gcp` for this task too.

### Remove a backend

Use the same input values and cloud identity that created the resources:

```bash
mise exec -- tofu -chdir=e2e/bootstrap/aws destroy \
  -var="region=$E2E_AWS_REGION" -var="bucket_name=$E2E_S3_BUCKET"

mise exec -- tofu -chdir=e2e/bootstrap/gcp destroy \
  -var="project_id=$E2E_GCP_PROJECT_ID" -var="bucket_name=$E2E_GCS_BUCKET"
```

Run only the command for the backend you intend to remove, carrying through any additional overrides used at creation.
Both buckets allow forced deletion of remaining objects, so inspect the destroy plan before approving it.
After removal, delete that backend's three Actions variables. Keep the state until the destroy succeeds.

## If a step fails

| Symptom | What to do |
| --- | --- |
| The GitHub IAM OIDC provider already exists | Use the existing-provider settings above. |
| Apply succeeds but setting variables fails | Check GitHub CLI access, then rerun `cloud:configure` for that backend; another apply is not needed. |
| `cloud:configure` cannot read one root's outputs | Select the backend you actually provisioned. |
| The cloud workflow cannot authenticate | Compare the target repository's real OIDC claims with the trust variables and [cloud setup requirements](../cloud-buckets.md). |
