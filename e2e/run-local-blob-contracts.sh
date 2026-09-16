#!/usr/bin/env bash
set -euo pipefail

reeve_source=${1:-../reeve}
script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
work_dir=$(mktemp -d "${TMPDIR:-/tmp}/reeve-blob-contracts.XXXXXX")
s3_log=$work_dir/s3.log
gcs_log=$work_dir/gcs.log
s3_pid=
gcs_pid=

# shellcheck disable=SC2329 # Invoked by the EXIT trap.
cleanup() {
  status=$?
  trap - EXIT
  for pid in "$s3_pid" "$gcs_pid"; do
    if [ -n "$pid" ]; then
      kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
  find "$work_dir" -depth -delete
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

bash "$script_dir/run-local-s3-contract.sh" "$reeve_source" >"$s3_log" 2>&1 &
s3_pid=$!
bash "$script_dir/run-local-gcs-contract.sh" "$reeve_source" >"$gcs_log" 2>&1 &
gcs_pid=$!

status=0
if ! wait "$s3_pid"; then
  status=1
fi
s3_pid=
if ! wait "$gcs_pid"; then
  status=1
fi
gcs_pid=

printf 'S3 contract:\n'
cat "$s3_log"
printf 'GCS contract:\n'
cat "$gcs_log"
exit "$status"
