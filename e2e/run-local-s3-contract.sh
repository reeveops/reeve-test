#!/usr/bin/env bash
set -euo pipefail

reeve_source=${1:-../reeve}
reeve_source=$(cd "$reeve_source" && pwd -P)

for command in curl go minio; do
  if ! command -v "$command" >/dev/null 2>&1; then
    printf 'required command is missing: %s\n' "$command" >&2
    exit 1
  fi
done

port=${REEVE_MINIO_PORT:-}
if [ -z "$port" ]; then
  for candidate in $(seq 19000 19099); do
    if ! (exec 3<>"/dev/tcp/127.0.0.1/$candidate") 2>/dev/null; then
      port=$candidate
      break
    fi
  done
fi
if [ -z "$port" ]; then
  printf 'no free local MinIO port found\n' >&2
  exit 1
fi

work_dir=$(mktemp -d "${TMPDIR:-/tmp}/reeve-minio.XXXXXX")
data_dir=$work_dir/data
log_file=$work_dir/minio.log
bucket=reeve-e2e
mkdir -p "$data_dir/$bucket"

export MINIO_ROOT_USER=reevee2e
export MINIO_ROOT_PASSWORD=reeve-e2e-secret
minio server "$data_dir" --address "127.0.0.1:$port" >"$log_file" 2>&1 &
minio_pid=$!

cleanup() {
  status=$?
  trap - EXIT
  kill "$minio_pid" 2>/dev/null || true
  wait "$minio_pid" 2>/dev/null || true
  if [ "$status" -ne 0 ]; then
    cat "$log_file" >&2
  fi
  find "$work_dir" -depth -delete
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

ready=false
for _ in $(seq 1 100); do
  if curl -fsS "http://127.0.0.1:$port/minio/health/live" >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 0.1
done
if [ "$ready" != true ]; then
  printf 'MinIO did not become ready\n' >&2
  exit 1
fi

export AWS_ACCESS_KEY_ID=$MINIO_ROOT_USER
export AWS_SECRET_ACCESS_KEY=$MINIO_ROOT_PASSWORD
export AWS_REGION=us-east-1
export AWS_EC2_METADATA_DISABLED=true
export REEVE_S3_CONTRACT_BUCKET=$bucket
export REEVE_S3_CONTRACT_REGION=$AWS_REGION
export REEVE_S3_CONTRACT_ENDPOINT="http://127.0.0.1:$port"
export REEVE_S3_CONTRACT_PREFIX="local-${RANDOM}-${RANDOM}"

go -C "$reeve_source" test -race -count=1 -v ./internal/blob/s3 -run '^TestContract$'
