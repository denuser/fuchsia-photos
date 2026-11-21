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
work=$(mktemp -d "$root/immich.XXXXXXXX")
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
BENCH_PASSWORD=$(openssl rand -hex 24)
sed -e '/container_name:/d' -e '/^  immich-server:$/a\    mem_limit: 6g' -e '/^  database:$/a\    mem_limit: 4g' -e 's/restart: always/restart: "no"/' \
    -e "s|'2283:2283'|'127.0.0.1::2283'|" \
    -e '/- \/etc\/localtime/a\      - ./inputs:/samples:ro' \
    "${IMMICH_TEMPLATE:?}/docker-compose.yml" > compose.yaml
sed -e "s/^DB_PASSWORD=.*/DB_PASSWORD=$BENCH_PASSWORD/" -e "s/^IMMICH_VERSION=.*/IMMICH_VERSION=$IMMICH_VERSION/" "$IMMICH_TEMPLATE/example.env" > .env
COMPOSE_PROGRESS=quiet compose up -d --wait --wait-timeout 180 immich-server redis database
port=$(compose port immich-server 2283)
base="http://$port/api"
request() { curl --max-time 15 -fsS -H 'Content-Type: application/json' "$@"; }
request "$base/server/version"
printf '\n'
owner=$(request -d "{\"email\":\"benchmark@example.test\",\"name\":\"Benchmark\",\"password\":\"$BENCH_PASSWORD\"}" "$base/auth/admin-sign-up" | python3 -c 'import json,sys;print(json.load(sys.stdin)["id"])')
token=$(request -d "{\"email\":\"benchmark@example.test\",\"password\":\"$BENCH_PASSWORD\"}" "$base/auth/login" | python3 -c 'import json,sys;print(json.load(sys.stdin)["accessToken"])')
api() { request -H "Authorization: Bearer $token" "$@"; }
if api "$base/admin/config" > defaults.json; then config=admin/config; else config=system-config; api "$base/$config" > defaults.json; fi
python3 - "$matched" <<'CFG'
import sys
import json
with open("defaults.json") as f:
    config = json.load(f)
config["machineLearning"]["enabled"] = False
if sys.argv[1] == "1":
    config["image"]["preview"].update(size=1200, format="jpeg", quality=80)
    config["image"]["thumbnail"].update(size=375, format="jpeg", quality=80)
for key in ("thumbnailGeneration", "metadataExtraction"):
    config["job"][key]["concurrency"] = 3
for key in ("reverseGeocoding", "newVersionCheck"):
    if key in config:
        config[key]["enabled"] = False
print(json.dumps(config["image"]))
with open("config.json", "w") as f:
    json.dump(config, f)
CFG
api -X PUT --data-binary @config.json "$base/$config" > /dev/null
library=$(api -d "{\"name\":\"Benchmark\",\"ownerId\":\"$owner\",\"importPaths\":[\"/samples\"]}" "$base/libraries" | python3 -c 'import json,sys;print(json.load(sys.stdin)["id"])')
for service in immich-server database redis; do
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
api -X POST "$base/libraries/$library/scan" > /dev/null
seen=0
for i in {1..1800}; do
  api "$base/queues" > queues.json
  api "$base/libraries/$library/statistics" > scan.json
  read -r busy errors ready < <(python3 -c '
import json
queues = [queue["statistics"] for queue in json.load(open("queues.json"))]
print(sum(q[k] for q in queues for k in ("active","waiting","delayed")), sum(q["failed"] for q in queues), json.load(open("scan.json"))["photos"])
')
  if (( busy > 0 || ready > 0 || errors > 0 )); then seen=1; fi
  if (( seen && !busy )); then
    end=$(date +%s%N)
    kill "$sampler"
    wait "$sampler" || true
    sampler=
    indexed=$ready
    ready=$(find library/thumbs -type f -name '*_preview.*' | wc -l)
    printf 'directory=%s inputs=%s indexed=%s ready=%s failed_jobs=%s\n' "$work" "$1" "$indexed" "$ready" "$errors"
    cat scan.json
    printf '\n'
    find library/thumbs -type f | awk -F/ '{print $NF}' | sed -E 's/^.{36}//' | sort | uniq -c
    find library/thumbs -type f ! -name .immich -printf "%s\n" | awk '{n++; bytes+=$1} END {print "preview_files=" n, "preview_bytes=" bytes}'
    if [[ ${2:-} == */raw ]]; then
      api -d '{"size":100}' "$base/search/metadata" > assets.json || true
      compose logs --no-color immich-server > service.log
    fi
    break
  fi
  sleep 0.5
done
test -n "${end:-}"
python3 - "$start" "$end" "$ready" <<'STATS'
import pathlib, statistics, sys
start, end, ready = map(int, sys.argv[1:])
samples = [tuple(map(int, row.split())) for row in pathlib.Path('memory.tsv').read_text().splitlines()]
seconds = (end - start) / 1e9
print(f'elapsed_s={seconds:.3f} photos_s={ready / seconds:.3f} idle_working_MiB={statistics.median(ws for ts, _, ws in samples if ts < start)/2**20:.1f} peak_working_MiB={max(ws for ts, _, ws in samples if start <= ts <= end)/2**20:.1f} peak_total_MiB={max(total for ts, total, _ in samples if start <= ts <= end)/2**20:.1f}')
STATS

if ((matched)); then
  find library/thumbs -name "*_preview.jpeg" | xargs -r file -b | grep -oE ', [0-9]+x[0-9]+, components' | sort | uniq -c > preview-dimensions.tsv
  cat preview-dimensions.tsv
  awk -v expected="$1" '$3 == "1600x1200," {correct+=$1} END {exit correct!=expected}' preview-dimensions.tsv
fi
