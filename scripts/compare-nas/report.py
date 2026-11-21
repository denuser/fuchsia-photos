from collections import defaultdict
import json
from pathlib import Path
from statistics import median
import sys

logs = Path(sys.argv[1])
groups = defaultdict(list)
for log in sorted(logs.glob("[123]-*-*.log")):
    repeat, scenario = log.stem.split("-", 1)
    scenario, app = scenario.rsplit("-", 1)
    record = {}
    scan = {}
    matched_count = 0
    immich_preview_count = None
    for line in log.read_text().splitlines():
        if line.startswith(("directory=", "elapsed_s=", "preview_files=", "profile=")):
            record.update(field.split("=", 1) for field in line.split() if "=" in field)
        if line.strip().endswith(", 1600x1200, components"):
            matched_count += int(line.split()[0])
        parts = line.split()
        if app == "immich" and len(parts) == 2 and parts[1].startswith("_preview."):
            immich_preview_count = (immich_preview_count or 0) + int(parts[0])
        if line.startswith("{"):
            try:
                data = json.loads(line)
            except ValueError:
                continue
            if isinstance(data, dict) and "run" in data:
                scan = data["run"]
    if record.get("status") == "skipped_raw_repeat":
        continue
    if "elapsed_s" not in record:
        print(f"{repeat} {scenario:4} {app:10} incomplete")
        continue
    record["processed"] = int(record.get("ready", record.get("indexed", 0)))
    if "ready" not in record and immich_preview_count is not None:
        record["processed"] = immich_preview_count
    record["photos_s"] = record["processed"] / float(record["elapsed_s"])
    record["failures"] = int(record.get("failed", record.get("failed_jobs", 0)))
    record["scan_failures"] = int(scan.get("failed", 0))
    record["unsupported"] = int(scan.get("unsupported", 0))
    record["duplicates"] = int(scan.get("duplicates", 0))
    if record.get("profile") == "matched-jpeg-1600x1200":
        if matched_count != int(record["inputs"]):
            print(f"{repeat} {app}: matched preview check failed ({matched_count}/{record['inputs']})")
            continue
        scenario = "1600x1200"
    groups[scenario, app].append(record)
    print(f"{repeat} {scenario:4} {app:10} files {record['processed']:3}/{record['inputs']} "
          f"fail {record['failures'] + record['scan_failures']} unsupported {record['unsupported']} "
          f"duplicates {record['duplicates']} time {record['elapsed_s']} "
          f"peak {record['peak_working_MiB']} previews {record.get('preview_files', '?')}")
for (scenario, app), records in sorted(groups.items()):
    values = [median(float(r[field]) for r in records) for field in
              ["elapsed_s", "photos_s", "idle_working_MiB", "peak_working_MiB", "peak_total_MiB"]]
    seconds, rate, idle, peak, total = values
    print(f"{scenario:4} {app:10} n={len(records)} "
          f"{seconds:.1f}s {rate:.2f}/s RAM {idle:.0f}/{peak:.0f} MiB (raw {total:.0f}) "
          f"range {min(float(r['elapsed_s']) for r in records):.1f}-{max(float(r['elapsed_s']) for r in records):.1f}s")
