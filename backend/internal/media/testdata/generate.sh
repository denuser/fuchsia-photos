#!/bin/sh
# Regenerates the test fixtures. Requires libvips-tools, libimage-exiftool-perl
# and libheif-plugin-x265. Run through `make fixtures`.
set -eu

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

image() {
    vips gaussnoise "$tmp/noise.v" "$1" "$2" --sigma 40 --mean 128
    vips gaussblur "$tmp/noise.v" "$tmp/blur.v" 8
    vips cast "$tmp/blur.v" "$tmp/gray.v" uchar
    vips bandjoin "$tmp/gray.v $tmp/gray.v $tmp/gray.v" "$tmp/rgb.v"
    vips copy "$tmp/rgb.v" "$3" --interpretation srgb
}

image 800 600 "$tmp/landscape.v"
vips copy "$tmp/landscape.v" landscape.jpg
vips copy "$tmp/landscape.v" landscape.png
vips copy "$tmp/landscape.v" landscape.webp
vips copy "$tmp/landscape.v" 'landscape.heic[compression=hevc,Q=50]'

image 2400 1800 "$tmp/large.v"
vips copy "$tmp/large.v" 'large.jpg[Q=40]'

# Stored as landscape, shown as portrait, and carries GPS that must not leak.
cp landscape.jpg rotated.jpg
exiftool -q -overwrite_original -n -Orientation=6 -GPSLatitude=59.93 -GPSLongitude=30.33 rotated.jpg
