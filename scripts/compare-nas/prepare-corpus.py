#!/usr/bin/env python3
"""Stage identical read-only inputs and record SHA-256 manifests."""
from argparse import ArgumentParser
from hashlib import file_digest
from pathlib import Path
import os
import re
from shutil import copyfile
from subprocess import check_output

parser = ArgumentParser(description=__doc__)
parser.add_argument("originals", type=Path, help="Folder containing original JPEGs")
parser.add_argument("raw_samples", type=Path, help="Shared raw-samples folder")
parser.add_argument("output", type=Path, help="New corpus directory")
parser.add_argument("--jpeg-count", type=int, default=475)
parser.add_argument("--matched-count", type=int, default=100)
args = parser.parse_args()
jpeg = sorted(p for p in args.originals.rglob("*") if p.is_file() and p.suffix.lower() in {".jpg", ".jpeg"})
raw = sorted(p for p in (args.raw_samples / "problem").iterdir() if p.is_file()) + [
    args.raw_samples / "Panasonic-DC-S1RM2-177MP.RW2",
    args.raw_samples / "Samsung-Galaxy-S23-Ultra-50MP.dng",
]
if not 0 < args.jpeg_count <= len(jpeg) or not 0 < args.matched_count <= args.jpeg_count:
    parser.error("Counts must be positive and fit the JPEG library")
if len(raw) != 20 or not all(p.is_file() for p in raw):
    parser.error("Expected the shared set of 20 difficult RAWs")

root = args.output.resolve()
root.mkdir(parents=True, exist_ok=False)
matched = root / "jpeg-4x3"
matched.mkdir(mode=0o755)
matched.chmod(0o755)
seen = set()
matched_entries = []
for scenario, inputs in (("jpeg", jpeg[:args.jpeg_count]), ("raw", raw)):
    destination = root / scenario
    destination.mkdir(mode=0o755)
    destination.chmod(0o755)
    entries = []
    for i, photo in enumerate(inputs):
        target = destination / (f"{i:04}.jpg" if scenario == "jpeg" else photo.name)
        copyfile(photo, target)
        target.chmod(0o444)
        with target.open("rb") as file:
            digest = file_digest(file, "sha256").hexdigest()
        entries.append(f"{digest}  {target.relative_to(root)}")
        if scenario == "jpeg" and digest not in seen and len(matched_entries) < args.matched_count:
            header = check_output(["file", "-b", str(target)], text=True)
            dimensions = re.search(r", (\d+)x(\d+), components", header)
            if dimensions and not re.search(r"orientation=(?!upper-left)([^,;\]]+)", header, re.I):
                width, height = map(int, dimensions.groups())
                if width >= 1600 and width * 3 == height * 4:
                    link = matched / target.name
                    os.link(target, link)
                    matched_entries.append(f"{digest}  {link.relative_to(root)}")
        seen.add(digest)
    (root / f"{scenario}.sha256").write_text("\n".join(entries) + "\n")
    print(f"{scenario}: {len(entries)} files")
if not matched_entries:
    parser.error("No landscape 4:3 JPEGs at least 1600 px wide were found")
(root / "jpeg-4x3.sha256").write_text("\n".join(matched_entries) + "\n")
print(f"jpeg-4x3: {len(matched_entries)} unique files")
