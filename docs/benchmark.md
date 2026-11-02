# Benchmark

`imgbench` measures how fast a machine processes a folder of photos. Its results decide the preview format and the worker count, and replace the estimates in the README.

It reads the originals without changing them. For each photo it computes the SHA-256 hash, generates the previews and writes them to `/out`. It deletes the previews at the end of each run.

EXIF extraction and database writes are not included, so the real pipeline will be somewhat slower. It uses the default preview settings, not the ones saved on the administration page.

## Quick run on the sample photos

The repository includes [real photos](../samples/README.md) from several cameras and phones. On a machine with the repository checked out, this builds the image and benchmarks them, writing previews to `.bench`:

```sh
make bench
```

To test a specific disk, point `BENCH_OUT` at a folder on it:

```sh
make bench BENCH_OUT=/path/on/disk/under/test
```

Without `make`:

```sh
docker build -f deploy/Dockerfile -t fuchsia-photos:latest .
docker run --rm --user "$(id -u):$(id -g)" \
  -v "$PWD/samples:/in:ro" \
  -v /path/on/disk/under/test:/out \
  fuchsia-photos:latest imgbench -in /in -out /out
```

There are only 9 photos, so this is a quick check that is easy to repeat. For the final decision, also run it on your own library, as described below.

## Benchmarking your own photos

### Getting the image onto the NAS

Build it on the NAS:

```sh
docker build -f deploy/Dockerfile -t fuchsia-photos:latest .
```

Or build it on another machine and copy it over:

```sh
make image
docker save fuchsia-photos:latest | ssh <nas> docker load
```

### Running

```sh
docker run --rm --user "$(id -u):$(id -g)" \
  -v /path/to/photos:/in:ro \
  -v /path/on/disk/under/test:/out \
  fuchsia-photos:latest imgbench -in /in -out /out -limit 500
```

Use a mix of JPEG and HEIC photos from real cameras and phones, since small or synthetic images say little about real speed. The user running the command needs write access to the `/out` folder.

| Flag | Default | Purpose |
|---|---|---|
| `-in` | required | Folder with photos, read recursively |
| `-out` | required | Folder for the previews, on the disk being tested |
| `-workers` | `1,2,3` | Comma-separated worker counts |
| `-formats` | `webp,jpeg` | Comma-separated preview formats |
| `-limit` | `0` | Maximum number of photos, `0` means all |

The command runs every combination of format and worker count, so the total time is several times that of one pass.

## What to measure

1. `/out` on the HDD
2. `/out` on the SSD, if the NAS has one

To measure a cold disk, clear the page cache on the NAS before a run. Otherwise, originals may be read from memory after the first pass:

```sh
sync && echo 3 | sudo tee /proc/sys/vm/drop_caches
```

### Full run

This script compares every format and worker count on a cold disk, clearing the page cache before each combination so that all the rows are measured under the same conditions. Run it once for each disk, with a mix of JPEG and HEIC photos in the source folder:

```sh
for f in webp jpeg; do for w in 2 3; do
  sync && echo 3 | sudo tee /proc/sys/vm/drop_caches >/dev/null
  docker run --rm --user "$(id -u):$(id -g)" \
    -v /path/to/photos:/in:ro \
    -v /path/on/disk/under/test:/out \
    fuchsia-photos:latest imgbench -in /in -out /out -limit 500 -formats "$f" -workers "$w"
done; done
```

Each combination prints its own table and peak memory.

## Reading the results

The output is a Markdown table with one row per file type, plus a total row for each combination of format and worker count. The rows of a combination appear when it finishes.

While a combination runs, a progress line goes to stderr every 5 seconds:

```text
2026/11/02 12:00:00 run 1/6, webp, workers 1: 350/1800 photos, 0 failed, 4.2 photos/s, 5m45s left
```

The console shows these lines mixed with the table. To get the table alone, redirect stdout with `> results.md`.

| Column | Meaning |
|---|---|
| Failed | Photos that could not be processed. They are listed in stderr, and the command exits with status 1 |
| Median, p95 | Time per photo: hashing, generating previews and writing them |
| Viewer avg, Grid avg | Average preview size, which determines how much disk space previews take |
| **photos/s** | Overall throughput, the main number to compare |

Peak memory is printed at the end, and covers all the runs in the command.

A warning like `VipsJpeg: premature end of JPEG image` means that a file is truncated or damaged. libvips reports it without the file name and decodes what it can, so the photo is not counted as failed. Each new warning also repeats the earlier ones, so the number of these lines says little about the number of damaged files.

Throughput usually stops growing once the disk or the CPU cores are saturated. Choose the lowest worker count close to the best result, so that the UI keeps a core to itself.

## Results

Measured on 2026-11-02 on the NAS (Intel N150) with 475 JPEG photos and `/out` on the SSD. No photo failed, and peak memory was 372.6 MB. Only the first row may have read the originals from a cold disk, since the page cache was not cleared between runs.

| Format | Workers | Photos/s | Total | Median | p95 | Viewer avg | Grid avg |
|---|---:|---:|---:|---:|---:|---:|---:|
| WebP, quality 75 | 1 | 1.1 | 7m31s | 934ms | 1.185s | 229 KB | 14 KB |
| WebP, quality 75 | 2 | 2.1 | 3m48s | 945ms | 1.2s | 229 KB | 14 KB |
| WebP, quality 75 | 3 | 3.0 | 2m38s | 984ms | 1.253s | 229 KB | 14 KB |
| JPEG, quality 80 | 1 | 2.4 | 3m15s | 408ms | 504ms | 416 KB | 23 KB |
| JPEG, quality 80 | 2 | 4.8 | 1m39s | 413ms | 505ms | 416 KB | 23 KB |
| JPEG, quality 80 | 3 | 6.6 | 1m12s | 447ms | 555ms | 416 KB | 23 KB |

**Format: WebP.** JPEG is 2.2 times faster, but WebP previews take 243 KB per photo instead of 439 KB. For 100,000 photos, that is about 9.3 hours instead of 4.2 hours of one-time background work, and about 24 GB instead of 44 GB of permanent space on the SSD. The two quality settings are not directly comparable, and this benchmark does not measure visual quality.

**Workers: 3.** Throughput grows almost linearly up to 3 workers, which is about 40% faster than 2, so the limit is the CPU rather than the disk. This still leaves a core for the UI.

HEIC photos were left out, because they are not currently relevant, and originals on the HDD were not compared with the SSD. Previews are kept on the SSD in any case.
