# Benchmark

`imgbench` measures how fast a machine processes a folder of photos. Its results decide the preview format and the worker count, and replace the estimates in the README.

It reads the originals without changing them. For each photo it computes the SHA-256 hash, generates the previews and writes them to `/out`. It deletes the previews at the end of each run.

EXIF extraction and database writes are not included, so the real pipeline will be somewhat slower.

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

The output is a Markdown table with one row per file type, plus a total row for each combination of format and worker count.

| Column | Meaning |
|---|---|
| Failed | Photos that could not be processed. They are listed in stderr, and the command exits with status 1 |
| Median, p95 | Time per photo: hashing, generating previews and writing them |
| Viewer avg, Grid avg | Average preview size, which determines how much disk space previews take |
| **photos/s** | Overall throughput, the main number to compare |

Peak memory is printed at the end, and covers all the runs in the command.

Throughput usually stops growing once the disk or the CPU cores are saturated. Choose the lowest worker count close to the best result, so that the UI keeps a core to itself.
