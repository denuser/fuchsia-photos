# Benchmark

`imgbench` measures how fast a machine processes a folder of photos. Its results decide the preview format and the worker count, and give the performance figures in the README.

Video transcoding has a script of its own, described under [Video transcoding](#video-transcoding).

It reads the originals without changing them. For each photo it computes the SHA-256 hash, generates the previews and writes them to `/out`. It deletes the previews at the end of each run.

EXIF extraction and database writes are not included, so the real pipeline will be somewhat slower. It uses the default preview settings, not the ones saved on the administration page.

## Quick run on sample photos

Put a few photos from different cameras and phones into `samples/` at the repository root. The folder is not part of the repository; public domain photos from [Wikimedia Commons](https://commons.wikimedia.org/) work well, and HEIC files can be made from them with libvips. This builds the image and benchmarks them, writing previews to `.bench`:

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

A handful of photos makes a quick check that is easy to repeat. For the final decision, also run it on your own library, as described below.

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

## Video transcoding

`scripts/video-bench.sh` measures how fast the NAS makes H.264 copies of videos that some browsers cannot play, such as HEVC from phones. Its results decide step 2 of stage 8 in the [roadmap](roadmap.md). The plan is hardware encoding with `h264_vaapi`, turned on in the administration page where the graphics allow it; libx264 on the CPU is measured for comparison.

The script runs the ffmpeg build of the Jellyfin image (`jellyfin/jellyfin:10.11.11`), which has VAAPI, Intel's iHD driver, `tonemap_vaapi` and libx264, so the application image stays as it is until the results are in. It needs bash and Docker on the NAS, and the render node `/dev/dri/renderD128`. It reads the originals without changing them, and deletes each copy after measuring it.

### Running videos

Copy the script to the NAS, or clone the repository there, and point it at a folder with videos and a folder for the copies on the SSD:

```sh
scripts/video-bench.sh /path/to/videos /path/on/ssd/video-bench > video-results.md
```

Use real videos from phones: 1080p and 4K HEVC from an iPhone, HDR included, and H.264 from Android. A dozen files of 10 to 60 seconds is enough. Each video runs in every mode, one after another, so the total time is several times their length on a slow mode.

| Variable | Default | Purpose |
|---|---|---|
| `MODES` | `vaapi vaapi-1080` | Modes to run, separated by spaces. Add `x264-1080` to compare with the CPU, which takes minutes for each video on the N150 |
| `LIMIT` | `0` | Maximum number of videos, `0` means all |
| `DEVICE` | `/dev/dri/renderD128` | Render node |
| `IMAGE` | `jellyfin/jellyfin:10.11.11` | Image with jellyfin-ffmpeg |
| `KEEP` | `0` | `1` keeps the copies, to compare them by eye |
| `TOOLS` | `/usr/lib/jellyfin-ffmpeg` | Where `IMAGE` keeps ffmpeg, ffprobe and vainfo |
| `HDR_TONEMAP` | `opencl` | How the VAAPI modes tone map HDR: `opencl`, or `vaapi` for Intel's video processing |

| Mode | What it does |
|---|---|
| `vaapi` | Decoding, scaling and H.264 encoding on the graphics, at the size of the original |
| `vaapi-1080` | The same, with the shorter side brought down to 1080. Skipped for videos that are 1080p or smaller already |
| `x264-1080` | Decoding and libx264 (`veryfast`, CRF 23) on the CPU, with the shorter side at 1080 |

HDR videos are tone mapped to SDR. The VAAPI modes use `tonemap_opencl` on the same graphics by default, as Jellyfin does on Intel. `tonemap_vaapi`, Intel's video processing, needs no OpenCL runtime but takes HDR10 (PQ) only, while iPhones record HDR as HLG with Dolby Vision; run once more with `HDR_TONEMAP=vaapi` to see how it does with them. The CPU mode uses the same `zscale` and `tonemap` chain as the posters.

HDR from an iPhone is the case to watch. If the graphics cannot tone map it, hardware transcoding without a CPU fallback leaves those videos as they are in step 1, and the plan for step 2 has to change. The application image would also need the OpenCL runtime for Intel graphics, which is large, next to libva and the iHD driver.

The application's image for Intel graphics, described in [Running from an image](deployment.md#running-from-an-image), runs the VAAPI modes too, which checks its own ffmpeg build and drivers:

```sh
IMAGE=ghcr.io/denuser/fuchsia-photos:latest-intel TOOLS=/usr/local/bin \
  scripts/video-bench.sh /path/to/videos /path/on/ssd/video-bench > video-results.md
```

Before the first video, the script runs `vainfo` and stops if VAAPI does not work or offers no H.264 encoding. Newer Intel graphics, the N150 among them, encode H.264 only in the low-power mode, which the script turns on when `vainfo` shows only `VAEntrypointEncSliceLP`.

### Reading the video results

The table has one row per video and mode. Progress goes to stderr; the log of a failed run stays in `logs/` in the folder for the copies, and its last lines are printed.

| Column | Meaning |
|---|---|
| Source | Codec, size, HDR transfer and duration of the original |
| Output | Width and height of the copy |
| Time | Wall time of the encoding, without starting the container |
| **Speed** | Multiples of real time: `4.0x` makes a minute of video in 15 seconds. The main number to compare |
| CPU | Cores kept busy on average. With VAAPI it shows what is left for the application and the other services |
| Size | Size of the copy, against the size of the original in the last column |

To see whether the graphics are the limit during a run, run `intel_gpu_top` on the NAS if it is installed.

### Video results

Measured on 2026-11-13 on the NAS (Intel N150, iHD driver 25.4.6) with four 30-second videos from [Jellyfin's test set](https://repo.jellyfin.org/test-videos/). The two `DV P8.4` files are HEVC Main 10 in HLG with Dolby Vision profile 8.4, the format an iPhone records. The driver offers the full H.264 encoder (`VAEntrypointEncSlice`), so the low-power mode was not needed.

HDR tone mapped with `tonemap_opencl`:

| Video | Source | Mode | Output | Time | Speed | CPU | Size | Source size |
|---|---|---|---|---:|---:|---:|---:|---:|
| 1080p DV P8.4 | HEVC 1920x1080, HLG | vaapi | 1920x1080 | 9.8s | 3.1x | 1.3 | 26.9 MB | 50.3 MB |
| 4K DV P8.4 | HEVC 3840x2160, HLG | vaapi | 3840x2160 | 26.5s | 1.1x | 1.0 | 57.0 MB | 183.1 MB |
| 4K DV P8.4 | HEVC 3840x2160, HLG | vaapi-1080 | 1920x1080 | 16.0s | 1.9x | 1.4 | 28.2 MB | 183.1 MB |
| 4K HEVC 8bit 50M | HEVC 3840x2160, SDR | vaapi | 3840x2160 | 23.4s | 1.3x | 0.3 | 44.6 MB | 175.7 MB |
| 4K HEVC 8bit 50M | HEVC 3840x2160, SDR | vaapi-1080 | 1920x1080 | 10.5s | 2.9x | 0.7 | 22.8 MB | 175.7 MB |
| 4K HEVC HDR10 50M | HEVC 3840x2160, PQ | vaapi | 3840x2160 | 26.3s | 1.1x | 1.0 | 37.4 MB | 175.7 MB |
| 4K HEVC HDR10 50M | HEVC 3840x2160, PQ | vaapi-1080 | 1920x1080 | 15.9s | 1.9x | 1.3 | 19.6 MB | 175.7 MB |

With `HDR_TONEMAP=vaapi`, `tonemap_vaapi` failed on both HLG files with `Failed to start picture processing`. On the PQ file it worked at 0.8x for 4K and 0.9x for 1080p, slower than OpenCL, with 0.3 of a core. SDR rows matched the run above.

`x264-1080` on the CPU was stopped after the first two files: the 1080p HLG file took about 3.5 minutes by the progress timestamps, about 0.14x real time, with every core busy.

**Encoder: `h264_vaapi`.** Hardware encoding is about 20 times faster than libx264 on HDR and leaves the CPU almost free, so a CPU fallback is not worth having on this NAS.

**HDR: `tonemap_opencl`.** It is the only path that handles HLG, which is what iPhones record, and it is faster than `tonemap_vaapi` on PQ too. The application image would need the OpenCL runtime for Intel graphics next to libva and the iHD driver.

**Size: the shorter side at 1080.** A copy at 1080p is made 1.5 to 2.2 times faster than one at the original 4K size, takes about half the space, and is enough for watching in a browser; the original stays available for download. A minute of 4K HDR takes about 32 seconds, a minute of 4K SDR about 21, and the copy takes 11 to 16 percent of the original's size.

**Driver.** Debian trixie's free `intel-media-va-driver` (25.2.3) encodes H.264 on the N150 only in the low-power mode (`VAEntrypointEncSliceLP`); the non-free one also offers `VAEntrypointEncSlice`. With Debian's ffmpeg, scaling 4K to 1080p and encoding to H.264 with no tone mapping and no audio took 12.9s for the SDR file and 12.8s for the HLG file with the free driver, and 10.6s and 11.5s with the non-free one: 10 to 20 percent slower. Measured on 2026-11-13. The Intel image takes the non-free driver: it differs only in prebuilt GPU kernels without published source, which Debian keeps out of main but which may be redistributed, and its 23 MB are small next to the OpenCL runtime.

**Image size.** On amd64, libva with the free iHD driver adds 18 MB to the application image, with the non-free one 41 MB. Intel's OpenCL runtime is not in Debian trixie: the packages from Intel's compute-runtime and graphics compiler releases add 412 MB, almost all of it the compiler. libplacebo with Mesa's Vulkan drivers would add 240 MB, untested on the N150. Measured on 2026-11-13 by installing the packages over the runtime layer.

The tone mapped copies, kept with `KEEP=1`, looked right next to the originals. Not checked yet: phone videos with rotation and sound.
