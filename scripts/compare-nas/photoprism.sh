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
work=$(mktemp -d "$root/photoprism.XXXXXXXX")
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
export BENCH_PASSWORD="$(openssl rand -hex 24)"
printf 'BENCH_PASSWORD=%s\n' "$BENCH_PASSWORD" > .env
if ((matched)); then
  printf 'BENCH_JPEG_QUALITY=80\n' >> .env
fi
printf 'BENCH_PHOTOPRISM_IMAGE=%s\n' "${PHOTOPRISM_IMAGE:-photoprism/photoprism:latest}" >> .env
cat > compose.yaml <<'YAML'
services:
  mariadb:
    image: mariadb:11.8
    mem_limit: 4g
    environment:
      MARIADB_ROOT_PASSWORD: ${BENCH_PASSWORD}
      MARIADB_DATABASE: photoprism
      MARIADB_USER: photoprism
      MARIADB_PASSWORD: ${BENCH_PASSWORD}
    healthcheck:
      test: ["CMD", "healthcheck.sh", "--connect", "--innodb_initialized"]
      interval: 2s
      timeout: 3s
      retries: 30
  app:
    image: ${BENCH_PHOTOPRISM_IMAGE}
    mem_limit: 6g
    depends_on:
      mariadb:
        condition: service_healthy
    ports:
      - "127.0.0.1::2342"
    environment:
      PHOTOPRISM_ADMIN_PASSWORD: ${BENCH_PASSWORD}
      PHOTOPRISM_DATABASE_DRIVER: mysql
      PHOTOPRISM_DATABASE_SERVER: mariadb:3306
      PHOTOPRISM_DATABASE_NAME: photoprism
      PHOTOPRISM_DATABASE_USER: photoprism
      PHOTOPRISM_DATABASE_PASSWORD: ${BENCH_PASSWORD}
      PHOTOPRISM_DISABLE_TLS: "true"
      PHOTOPRISM_READONLY: "true"
      PHOTOPRISM_DISABLE_FACES: "true"
      PHOTOPRISM_DISABLE_CLASSIFICATION: "true"
      PHOTOPRISM_LABELS_MODEL: none
      PHOTOPRISM_FACE_RUN: never
      PHOTOPRISM_DISABLE_PLACES: "true"
      PHOTOPRISM_JPEG_QUALITY: ${BENCH_JPEG_QUALITY:-83}
      PHOTOPRISM_WORKERS: "3"
    volumes:
      - ./inputs:/photoprism/originals:ro
      - ./storage:/photoprism/storage
YAML
compose up -d --wait --wait-timeout 180
port=$(compose port app 2342)
healthy=
for i in {1..60}; do
  if curl --max-time 2 -fsS "http://$port/api/v1/status" 2>/dev/null; then healthy=yes; break; fi
  sleep 1
done
test -n "$healthy"
printf '\n'
compose exec -T app photoprism version
compose exec -T app photoprism config | grep -Ei 'thumb|workers|labels-model|face-run' || true
for service in app mariadb; do
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
compose exec -T app sh -ec 'printf "bench_start=%s\n" "$(date +%s%N)"; photoprism index; printf "bench_end=%s\n" "$(date +%s%N)"' > index.log 2>&1
start=$(sed -n 's/^bench_start=//p' index.log)
end=$(sed -n 's/^bench_end=//p' index.log)
kill "$sampler"
wait "$sampler" || true
sampler=
tail -12 index.log
ready=$(compose exec -T mariadb mariadb -uphotoprism "-p$BENCH_PASSWORD" -N photoprism -e "SELECT COUNT(*) FROM files WHERE file_missing=0 AND file_error='' AND file_primary=1;")
printf 'directory=%s inputs=%s ready=%s\n' "$work" "$1" "$ready"
python3 - "$start" "$end" "$ready" <<'STATS'
import pathlib, statistics, sys
start, end, ready = map(int, sys.argv[1:])
samples = [tuple(map(int, row.split())) for row in pathlib.Path('memory.tsv').read_text().splitlines()]
seconds = (end - start) / 1e9
print(f'elapsed_s={seconds:.3f} photos_s={ready / seconds:.3f} idle_working_MiB={statistics.median(ws for ts, _, ws in samples if ts < start)/2**20:.1f} peak_working_MiB={max(ws for ts, _, ws in samples if start <= ts <= end)/2**20:.1f} peak_total_MiB={max(total for ts, total, _ in samples if start <= ts <= end)/2**20:.1f}')
STATS
find storage/cache/thumbnails -type f -printf "%s\n" | awk '{n++; bytes+=$1} END {print "preview_files=" n, "preview_bytes=" bytes}'
if [[ ${2:-} == */raw ]]; then
  compose exec -T mariadb mariadb -uphotoprism "-p$BENCH_PASSWORD" -N photoprism -e 'SELECT file_name,file_hash,file_width,file_height,file_error FROM files ORDER BY file_name;' > assets.tsv || true
fi

if ((matched)); then
  find storage/cache/thumbnails -name "*1920x1200_fit.jpg" | xargs -r file -b | grep -oE ', [0-9]+x[0-9]+, components' | sort | uniq -c > preview-dimensions.tsv
  cat preview-dimensions.tsv
  awk -v expected="$1" '$3 == "1600x1200," {correct+=$1} END {exit correct!=expected}' preview-dimensions.tsv
fi
