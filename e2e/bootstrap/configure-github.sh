#!/usr/bin/env bash
set -euo pipefail

backend=${1:-}
repository=${2:-reeveops/reeve-test}
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

set_variable() {
  local name=$1
  local root=$2
  local output=$3
  local value

  value=$(tofu -chdir="$root" output -raw "$output")
  gh variable set "$name" --repo "$repository" --body "$value"
}

case "$backend" in
  aws)
    root="$script_dir/aws"
    set_variable E2E_AWS_ROLE_ARN "$root" github_role_arn
    set_variable E2E_AWS_REGION "$root" region
    set_variable E2E_S3_BUCKET "$root" bucket_name
    ;;
  gcp)
    root="$script_dir/gcp"
    set_variable E2E_GCP_WORKLOAD_IDENTITY_PROVIDER "$root" workload_identity_provider
    set_variable E2E_GCP_SERVICE_ACCOUNT "$root" service_account
    set_variable E2E_GCS_BUCKET "$root" bucket_name
    ;;
  *)
    printf 'usage: %s aws|gcp [owner/repository]\n' "$0" >&2
    exit 2
    ;;
esac
