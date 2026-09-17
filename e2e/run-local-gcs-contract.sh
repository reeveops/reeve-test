#!/usr/bin/env bash
set -euo pipefail

reeve_source=${1:-../reeve}
reeve_source=$(cd "$reeve_source" && pwd -P)

for command in curl fake-gcs-server go; do
  if ! command -v "$command" >/dev/null 2>&1; then
    printf 'required command is missing: %s\n' "$command" >&2
    exit 1
  fi
done

port=${REEVE_FAKE_GCS_PORT:-}
if [ -z "$port" ]; then
  for candidate in $(seq 19100 19199); do
    if ! (exec 3<>"/dev/tcp/127.0.0.1/$candidate") 2>/dev/null; then
      port=$candidate
      break
    fi
  done
fi
if [ -z "$port" ]; then
  printf 'no free local fake GCS port found\n' >&2
  exit 1
fi

work_dir=$(mktemp -d "${TMPDIR:-/tmp}/reeve-gcs.XXXXXX")
data_dir=$work_dir/data
log_file=$work_dir/fake-gcs-server.log
bucket=reeve-e2e
mkdir -p "$data_dir/$bucket"

fake-gcs-server \
  -scheme http \
  -host 127.0.0.1 \
  -port "$port" \
  -external-url "http://127.0.0.1:$port" \
  -public-host "127.0.0.1:$port" \
  -data "$data_dir" \
  -backend filesystem \
  -filesystem-root "$work_dir/storage" \
  -log-level error >"$log_file" 2>&1 &
server_pid=$!

cleanup() {
  status=$?
  trap - EXIT
  kill "$server_pid" 2>/dev/null || true
  wait "$server_pid" 2>/dev/null || true
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
  if curl -fsS "http://127.0.0.1:$port/storage/v1/b" >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 0.1
done
if [ "$ready" != true ]; then
  printf 'fake GCS server did not become ready\n' >&2
  exit 1
fi

unset GOOGLE_APPLICATION_CREDENTIALS CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE
export STORAGE_EMULATOR_HOST="http://127.0.0.1:$port"
export REEVE_GCS_CONTRACT_BUCKET=$bucket
export REEVE_GCS_CONTRACT_PREFIX="local-${RANDOM}-${RANDOM}"

go -C "$reeve_source" test -race -count=1 -v ./internal/blob/gcs \
  -run '^TestContract$/^(MissingObject|PutGetOverwrite|ConditionalCreate|ConditionalUpdate|ConcurrentCreate|RecursiveList|ListMetadata|Delete)$'

# fake-gcs-server 1.56.1 ignores the generation-match precondition on DELETE.
# Require Reeve's strict contract to reject that behavior instead of treating
# an unconditional delete as supported.
if conditional_output=$(go -C "$reeve_source" test -race -count=1 -v ./internal/blob/gcs \
  -run '^TestContract$/ConditionalDelete$' 2>&1); then
  printf 'fake-gcs-server unexpectedly passed the conditional-delete contract\n' >&2
  exit 1
fi
printf '%s\n' "$conditional_output"
if ! grep -Fq 'stale DeleteIfMatch = <nil>, want ErrPreconditionFailed' <<<"$conditional_output"; then
  printf 'conditional-delete failure did not identify the ignored generation precondition\n' >&2
  exit 1
fi
printf 'fake-gcs-server conditional-delete limitation confirmed\n'
