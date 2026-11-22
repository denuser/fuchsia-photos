#!/usr/bin/env bash
set -euo pipefail
umask 077
if [[ $# != 1 || $1 == --help ]]; then
  cat <<'HELP'
Usage: bash run.sh CORPUS
CORPUS contains jpeg/, jpeg-4x3/, raw/ and their SHA-256 manifests.
Linux cgroup v2 and Docker Compose are required. Store CORPUS on the SSD being tested.
Round 1 uses default preview settings. Rounds 2 and 3 use matched JPEG previews.
Round 1 also tests the RAW corpus. Containers run one stack at a time.
Options (environment):
  BENCH_ROOT           test data directory (default: CORPUS/runs)
  BENCH_DOCKER_SUDO    set to 1 for noninteractive sudo (default: 0)
  BENCH_ROUNDS         number of rounds, 1 to 3 (default: 3)
  BENCH_REPEAT_RAW     set to 1 to run RAW in every round (default: 0)
  MAGELLANICA_IMAGE    Magellanica image name, tag, or digest
  IMMICH_VERSION       Immich release tag (default: v3.4.2)
  PHOTOPRISM_IMAGE     PhotoPrism image name, tag, or digest
HELP
  [[ ${1:-} == --help ]]
  exit
fi
dir=$(cd "$(dirname "$0")" && pwd)
corpus=$(cd "$1" && pwd)
export BENCH_ROOT="${BENCH_ROOT:-$corpus/runs}"
mkdir -p "$BENCH_ROOT"
export BENCH_ROOT=$(realpath "$BENCH_ROOT")
test -r /sys/fs/cgroup/cgroup.controllers
export IMMICH_VERSION="${IMMICH_VERSION:-v3.4.2}"
[[ $IMMICH_VERSION =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]
rounds=${BENCH_ROUNDS:-3}
[[ $rounds =~ ^[123]$ ]]
template="$BENCH_ROOT/immich-template/$IMMICH_VERSION"
mkdir -p "$template"
for file in docker-compose.yml example.env; do
  if [[ ! -s $template/$file ]]; then
    curl -fLsS --max-time 60 "https://github.com/immich-app/immich/releases/download/$IMMICH_VERSION/$file" -o "$template/$file.part"
    mv "$template/$file.part" "$template/$file"
  fi
done
export IMMICH_TEMPLATE="$template"
cd "$corpus"
for scenario in jpeg jpeg-4x3 raw; do
  sha256sum --quiet -c "$scenario.sha256"
done
results=$(mktemp -d "$BENCH_ROOT/results.XXXXXXXX")
printf 'logs=%s\n' "$results"
lscpu > "$results/host.txt"
failed=0
for round in $(seq "$rounds"); do
  case $round in
    1) apps=(magellanica immich photoprism); jpeg=jpeg;;
    2) apps=(immich photoprism magellanica); jpeg=jpeg-4x3;;
    3) apps=(photoprism magellanica immich); jpeg=jpeg-4x3;;
  esac
  scenarios=("$jpeg")
  if [[ $round == 1 || ${BENCH_REPEAT_RAW:-0} == 1 ]]; then
    scenarios+=(raw)
  fi
  for scenario in "${scenarios[@]}"; do
    for app in "${apps[@]}"; do
      log="$results/${round}-${scenario}-${app}.log"
      printf '%s round=%s %s %s start\n' "$(date -Is)" "$round" "$scenario" "$app"
      if timeout --signal=TERM --kill-after=80s 25m bash "$dir/$app.sh" "$corpus/$scenario" > "$log" 2>&1; then
        grep -E '^(directory=|elapsed_s=|preview_files=)' "$log"
      else
        failed=1
        printf 'ERROR %s\n' "$log" >&2
        tail -20 "$log" >&2
      fi
    done
  done
done
python3 "$dir/report.py" "$results"
exit "$failed"
