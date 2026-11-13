#!/usr/bin/env bash
# Measures how fast the NAS turns videos into H.264 copies that every browser
# plays: hardware encoding through VAAPI against libx264 on the CPU. It uses
# the ffmpeg build of the Jellyfin image, which has VAAPI, Intel's iHD driver
# and libx264, so the application image stays as it is. See docs/benchmark.md.
#
# Usage: scripts/video-bench.sh <folder with videos> [folder for the copies]
#
# Environment:
#   MODES    modes to run, default "vaapi vaapi-1080"; add x264-1080 to
#            compare with the CPU, which takes minutes for each video on a NAS
#   LIMIT    maximum number of videos, default 0 (all)
#   DEVICE   render node, default /dev/dri/renderD128
#   IMAGE    image with jellyfin-ffmpeg, default jellyfin/jellyfin:10.11.11
#   KEEP     1 keeps the copies instead of deleting each one after measuring it
#   HDR_TONEMAP  how VAAPI modes tone map HDR: opencl (default), which takes
#            HDR10 and HLG, or vaapi, Intel's video processing for HDR10 only
#
# The table goes to stdout, progress and errors to stderr. The log of a failed
# run is kept in <folder for the copies>/logs.
set -euo pipefail
export LC_ALL=C

in=${1:?usage: video-bench.sh <folder with videos> [folder for the copies]}
out=${2:-$(mktemp -d)}
modes=${MODES:-vaapi vaapi-1080}
limit=${LIMIT:-0}
device=${DEVICE:-/dev/dri/renderD128}
image=${IMAGE:-jellyfin/jellyfin:10.11.11}
keep=${KEEP:-0}
hdr_tonemap=${HDR_TONEMAP:-opencl}

in=$(cd "$in" && pwd)
mkdir -p "$out/logs"
out=$(cd "$out" && pwd)

log() { printf '%s %s\n' "$(date '+%H:%M:%S')" "$*" >&2; }

needs_vaapi=0
for m in $modes; do [[ $m == vaapi* ]] && needs_vaapi=1; done

# The render node goes into the containers only when a mode uses it, so the
# CPU mode also runs on a machine without one.
gpu=()
((needs_vaapi)) && gpu=(--device "$device" -e LIBVA_DRIVER_NAME=iHD)

# run <tool> <args...> runs a jellyfin-ffmpeg tool with the videos read-only.
run() {
  local tool=$1
  shift
  docker run --rm ${gpu[@]+"${gpu[@]}"} \
    -v "$in:/in:ro" -v "$out:/out" \
    --entrypoint "/usr/lib/jellyfin-ffmpeg/$tool" "$image" "$@"
}

lp=()
if ((needs_vaapi)); then
  [[ -e $device ]] || { echo "no $device on this machine; set MODES=x264-1080 to measure the CPU alone" >&2; exit 1; }
  vainfo=$(run vainfo --display drm --device "$device" 2>&1) || {
    echo "$vainfo" >&2
    echo "vainfo failed: VAAPI does not work with $device" >&2
    exit 1
  }
  log "$(grep -m1 'Driver version' <<<"$vainfo" || echo 'driver version unknown')"
  # Newer Intel graphics, the N150 among them, encode H.264 only in the
  # low-power mode (EncSliceLP), which h264_vaapi has to be told to use.
  if grep -Eq 'VAProfileH264High[[:space:]]*:[[:space:]]*VAEntrypointEncSlice$' <<<"$vainfo"; then
    log "H.264 encoding: VAEntrypointEncSlice"
  elif grep -Eq 'VAProfileH264High[[:space:]]*:[[:space:]]*VAEntrypointEncSliceLP' <<<"$vainfo"; then
    log "H.264 encoding: VAEntrypointEncSliceLP, low-power mode"
    lp=(-low_power 1)
  else
    echo "$vainfo" >&2
    echo "the driver offers no H.264 encoding" >&2
    exit 1
  fi
fi

# The same tone mapping as the posters in backend/internal/media/video.go.
sw_tonemap="zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709,tonemap=tonemap=hable:desat=0,zscale=t=bt709:m=bt709:r=tv,format=yuv420p"

# fit <width> <height> prints the size with the shorter side at most 1080,
# both even.
fit() {
  local w=$1 h=$2 short=$(($1 < $2 ? $1 : $2))
  if ((short <= 1080)); then
    echo "$((w / 2 * 2)) $((h / 2 * 2))"
  else
    echo "$((w * 1080 / short / 2 * 2)) $((h * 1080 / short / 2 * 2))"
  fi
}

human() { awk -v b="$1" 'BEGIN { split("B KB MB GB", u); i = 1; while (b >= 1024 && i < 4) { b /= 1024; i++ } printf (i == 1 ? "%d %s" : "%.1f %s"), b, u[i] }'; }

echo "| Video | Source | Mode | Output | Time | Speed | CPU | Size | Source size |"
echo "|---|---|---|---|---:|---:|---:|---:|---:|"

total=0
failed=0
n=0
while IFS= read -r -d '' path; do
  ((limit > 0 && n >= limit)) && break
  n=$((n + 1))
  rel=${path#"$in"/}
  # One line: codec, width, height, transfer, duration, pixel format.
  probe=$(run ffprobe -v error -select_streams v:0 \
    -show_entries stream=codec_name,width,height,color_transfer,pix_fmt:format=duration \
    -of default=noprint_wrappers=1 "/in/$rel" 2>/dev/null) || { log "skipped $rel: ffprobe failed"; continue; }
  get() { sed -n "s/^$1=//p" <<<"$probe" | head -n1; }
  codec=$(get codec_name)
  w=$(get width)
  h=$(get height)
  transfer=$(get color_transfer)
  pix=$(get pix_fmt)
  duration=$(get duration)
  [[ -n $codec && -n $w && -n $duration && $duration != N/A ]] || { log "skipped $rel: no video stream"; continue; }
  hdr=0
  [[ $transfer == smpte2084 || $transfer == arib-std-b67 ]] && hdr=1
  src="$codec ${w}x${h}"
  ((hdr)) && src+=" HDR ($transfer)"
  [[ $pix == *10* ]] && ((hdr == 0)) && src+=" 10-bit"
  src+=", $(printf '%.0f' "$duration")s"
  read -r fw fh < <(fit "$w" "$h")
  size_in=$(stat -c %s "$path" 2>/dev/null || stat -f %z "$path")

  for mode in $modes; do
    case $mode in
      vaapi) tw=$((w / 2 * 2)) th=$((h / 2 * 2)) ;;
      vaapi-1080 | x264-1080)
        tw=$fw th=$fh
        # Nothing to scale: the run would repeat the full-size one.
        if [[ $mode == vaapi-1080 && $tw -eq $((w / 2 * 2)) && $th -eq $((h / 2 * 2)) && " $modes " == *" vaapi "* ]]; then continue; fi
        ;;
      *) echo "unknown mode $mode" >&2; exit 1 ;;
    esac

    name="bench-$n-$mode.mp4"
    args=(-hide_banner -nostdin -y -benchmark -noautorotate)
    case $mode in
      vaapi*)
        args+=(-init_hw_device "vaapi=va:$device")
        vf="scale_vaapi=w=$tw:h=$th:format=nv12"
        if ((hdr)) && [[ $hdr_tonemap == vaapi ]]; then
          # Intel's video processing, which takes HDR10 (PQ) only.
          vf="tonemap_vaapi=format=nv12:t=bt709:m=bt709:p=bt709,$vf"
        elif ((hdr)); then
          # OpenCL on the same graphics, which takes HLG too, as iPhones
          # write it. Frames are shared with VAAPI, not copied.
          args+=(-init_hw_device opencl=ocl@va -filter_hw_device ocl)
          vf="scale_vaapi=w=$tw:h=$th:format=p010,hwmap=derive_device=opencl:mode=read"
          vf+=",tonemap_opencl=format=nv12:t=bt709:m=bt709:p=bt709:tonemap=bt2390:desat=0"
          vf+=",hwmap=derive_device=vaapi:mode=write:reverse=1,format=vaapi"
        fi
        args+=(-hwaccel vaapi -hwaccel_device va -hwaccel_output_format vaapi
          -i "/in/$rel" -vf "$vf" -c:v h264_vaapi ${lp[@]+"${lp[@]}"} -profile:v high -rc_mode CQP -qp 24)
        ;;
      x264*)
        vf="scale=$tw:$th"
        ((hdr)) && vf="$sw_tonemap,$vf" || vf="$vf,format=yuv420p"
        args+=(-i "/in/$rel" -vf "$vf" -c:v libx264 -preset veryfast -crf 23 -profile:v high)
        ;;
    esac
    args+=(-map 0:v:0 -map '0:a:0?' -c:a aac -b:a 128k -movflags +faststart "/out/$name")

    log "$n $rel: $mode"
    logfile="$out/logs/$n-$mode.log"
    if ! run ffmpeg "${args[@]}" >"$logfile" 2>&1; then
      failed=$((failed + 1))
      log "failed: $rel, $mode, see $logfile"
      tail -n 5 "$logfile" >&2
      echo "| $rel | $src | $mode | ${tw}x${th} | failed | | | | $(human "$size_in") |"
      rm -f "$out/$name"
      continue
    fi
    # -benchmark ends with: bench: utime=12.345s stime=0.678s rtime=4.321s
    if ! read -r utime stime rtime < <(sed -n 's/.*bench: utime=\([0-9.]*\)s stime=\([0-9.]*\)s rtime=\([0-9.]*\)s.*/\1 \2 \3/p' "$logfile" | tail -n1); then
      failed=$((failed + 1))
      log "no timing in $logfile"
      echo "| $rel | $src | $mode | ${tw}x${th} | no timing | | | | $(human "$size_in") |"
      continue
    fi
    size_out=$(stat -c %s "$out/$name" 2>/dev/null || stat -f %z "$out/$name")
    # Speed in multiples of real time; CPU in cores kept busy on average.
    speed=$(awk -v d="$duration" -v r="$rtime" 'BEGIN { printf "%.1fx", d / r }')
    cpu=$(awk -v u="$utime" -v s="$stime" -v r="$rtime" 'BEGIN { printf "%.1f", (u + s) / r }')
    echo "| $rel | $src | $mode | ${tw}x${th} | ${rtime}s | $speed | $cpu | $(human "$size_out") | $(human "$size_in") |"
    total=$((total + 1))
    rm -f "$logfile"
    ((keep)) || rm -f "$out/$name"
  done
done < <(find "$in" -type f \( -iname '*.mp4' -o -iname '*.mov' -o -iname '*.m4v' \) -print0 | sort -z)

log "done: $total runs, $failed failed, copies in $out"
((failed == 0))
