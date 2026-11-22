#!/bin/bash
set -euo pipefail
umask 077
if [[ $# != 1 ]]; then
  printf 'Usage: %s CORPUS_SCENARIO_DIRECTORY\n' "$0" >&2
  exit 2
fi
root=${BENCH_ROOT:?Set BENCH_ROOT to a writable test data directory}
root=$(realpath "$root")
input=$(realpath "$1")
test -d "$input"
count=$(find "$input" -maxdepth 1 -type f | wc -l)
((count > 0))
set -- "$count" "$input"
mkdir -p "$root"
work=$(mktemp -d "$root/magellanica.XXXXXXXX")
cd "$work"
project="bench-$(basename "$work" | tr '[:upper:].' '[:lower:]-')"
docker() {
  if [[ ${BENCH_DOCKER_SUDO:-0} == 1 ]]; then
    sudo -n "$(command -v docker)" "$@"
  else
    command docker "$@"
  fi
}
compose() { docker compose -p "$project" "$@"; }
trap 'status=$?; if [[ -n ${sampler:-} ]]; then kill "$sampler" 2>/dev/null || true; fi; if ((status)); then compose logs --tail 35 >&2; fi; compose down -v >&2' EXIT
trap 'exit 143' TERM HUP INT
matched=0
if [[ ${2:-} == */jpeg-4x3 ]]; then
  matched=1
  printf '%s\n' 'profile=matched-jpeg-1600x1200'
fi
ln -s "$2" inputs
find -L inputs -maxdepth 1 -type f -print0 | sort -z | xargs -0 -r cat > /dev/null
mkdir library cache
chmod 777 library cache
image=${MAGELLANICA_IMAGE:-ghcr.io/magellanica-app/magellanica-photos:latest}
export BENCH_IMAGE="$image"
export BENCH_PASSWORD="$(openssl rand -hex 24)"
printf "BENCH_IMAGE=%s\nBENCH_PASSWORD=%s\n" "$BENCH_IMAGE" "$BENCH_PASSWORD" > .env
cat > compose.yaml <<'YAML'
services:
  postgres:
    image: pgvector/pgvector:pg18-trixie
    environment:
      POSTGRES_USER: photos
      POSTGRES_PASSWORD: ${BENCH_PASSWORD}
    healthcheck:
      test: ["CMD", "pg_isready", "-h", "127.0.0.1", "-U", "photos"]
      interval: 1s
      timeout: 2s
      retries: 60
  app:
    image: ${BENCH_IMAGE}
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      DATABASE_URL: postgres://photos:${BENCH_PASSWORD}@postgres:5432/photos
      IMPORT_STABLE_SECONDS: "0"
      WORKERS: "3"
    ports:
      - "127.0.0.1::8080"
    mem_limit: 4g
    volumes:
      - ./library:/data/library
      - ./cache:/data/cache
      - ./inputs:/data/import:ro
YAML
compose up -d --wait --wait-timeout 180
port=$(compose port app 8080)
base="http://$port/api/v1"
for i in {1..30}; do
  if curl --max-time 2 -fsS "$base/health" >/dev/null 2>&1; then break; fi
  sleep 1
done
printf '%s\n' "$BENCH_PASSWORD" | compose exec -T app magellanica-photos create-admin benchmark
curl -fsS -c cookies -H 'Content-Type: application/json' -d "{\"login\":\"benchmark\",\"password\":\"$BENCH_PASSWORD\"}" "$base/login" >/dev/null
owner=$(curl -fsS -b cookies "$base/me" | python3 -c 'import sys,json;print(json.load(sys.stdin)["id"])')

docker image inspect "$BENCH_IMAGE" --format '{{.Id}} {{json .RepoDigests}}'
if ((matched)); then
  curl -fsS -b cookies -X PUT -H 'Content-Type: application/json' \
    -d '{"format":"jpeg","webpQuality":75,"webpEffort":2,"jpegQuality":80,"gridEdge":375,"viewerEdge":1600}' \
    "$base/settings/previews" >/dev/null
fi
curl -fsS -b cookies "$base/settings/previews"
printf '\n'
for service in app postgres; do
  id=$(compose ps -q "$service")
  pid=$(docker inspect "$id" --format '{{.State.Pid}}')
  awk -F: '$1 == 0 {print "/sys/fs/cgroup" $3}' "/proc/$pid/cgroup" >> cgroups
done
python3 -u -c '
import pathlib, time
groups = [pathlib.Path(s) for s in pathlib.Path("cgroups").read_text().splitlines()]
while True:
    current = 0
    working = 0
    for group in groups:
        usage = int((group / "memory.current").read_text())
        stats = dict(line.split() for line in (group / "memory.stat").read_text().splitlines())
        current += usage
        working += max(0, usage - int(stats["inactive_file"]))
    print(time.time_ns(), current, working)
    time.sleep(0.25)
' > memory.tsv &
sampler=$!
sleep 3
kill -0 "$sampler"
start=$(date +%s%N)
curl -fsS -b cookies -H 'Content-Type: application/json' -d "{\"ownerId\":$owner,\"folder\":\"\"}" "$base/imports" > start.json
run=$(python3 -c 'import json; print(json.load(open("start.json"))["id"])')
for i in {1..1800}; do
  curl -fsS -b cookies "$base/imports/$run" > scan.json
  read -r status added < <(python3 -c 'import json; data=json.load(open("scan.json")); run=data.get("run", data); print(run["status"],run["added"])')
  if [[ $status == finished ]]; then
    read -r ready failed < <(compose exec -T postgres psql -U photos -AtF ' ' -c "select count(*) filter(where status='ready'), count(*) filter(where status='failed') from photos")
    if (( ready + failed == added )); then
      end=$(date +%s%N)
      kill "$sampler"
      wait "$sampler" || true
      sampler=
      printf 'directory=%s inputs=%s ready=%s failed=%s\n' "$work" "$1" "$ready" "$failed"
      cat scan.json
      python3 - "$start" "$end" "$ready" <<'STATS'
import pathlib, statistics, sys
start, end, ready = map(int, sys.argv[1:])
samples = [tuple(map(int, row.split())) for row in pathlib.Path('memory.tsv').read_text().splitlines()]
seconds = (end - start) / 1e9
print(f'elapsed_s={seconds:.3f} photos_s={ready / seconds:.3f} idle_working_MiB={statistics.median(ws for ts, _, ws in samples if ts < start)/2**20:.1f} peak_working_MiB={max(ws for ts, _, ws in samples if start <= ts <= end)/2**20:.1f} peak_total_MiB={max(total for ts, total, _ in samples if start <= ts <= end)/2**20:.1f}')
STATS
      find cache/previews -type f -printf "%s\n" | awk '{n++; bytes+=$1} END {print "preview_files=" n, "preview_bytes=" bytes}'
      if [[ ${2:-} == */raw ]]; then
        compose exec -T postgres psql -U photos -AtF $'\t' -c 'select id, original_name, status, width, height, preview_format from photos order by original_name' > assets.tsv
      fi
      break
    fi
  elif [[ $status == failed || $status == stopped ]]; then
    cat scan.json
    exit 1
  fi
  sleep 0.5
done
test -n "${end:-}"

if ((matched)); then
  find cache/previews -name "*-viewer.jpeg" -o -name "*-viewer.jpg" | xargs -r file -b | grep -oE ', [0-9]+x[0-9]+, components' | sort | uniq -c > preview-dimensions.tsv
  cat preview-dimensions.tsv
  awk -v expected="$1" '$3 == "1600x1200," {correct+=$1} END {exit correct!=expected}' preview-dimensions.tsv
fi
