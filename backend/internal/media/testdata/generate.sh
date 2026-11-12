#!/bin/sh
# Regenerates the test fixtures. Requires libvips-tools, libimage-exiftool-perl,
# libheif-plugin-x265 and ffmpeg. Run through `make fixtures`.
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

# Stored as landscape, shown as portrait, carries GPS that must not leak, and
# names a camera and a lens for the filters.
cp landscape.jpg rotated.jpg
exiftool -q -overwrite_original -n -Orientation=6 -GPSLatitude=59.93 -GPSLongitude=30.33 \
    -Make=Canon '-Model=Canon EOS R5' '-LensModel=RF24-70mm F2.8 L IS USM' rotated.jpg

# Two seconds of 320x240 video.
clip() {
    ffmpeg -hide_banner -loglevel error -y -f lavfi -i testsrc=size=320x240:rate=10 -t 2 \
        -c:v libx264 -pix_fmt yuv420p "$@"
}

# Stored as landscape and played as portrait, with a location, a camera, and
# QuickTime's CreateDate, which is UTC.
clip "$tmp/plain.mp4"
ffmpeg -hide_banner -loglevel error -y -display_rotation:v:0 90 -i "$tmp/plain.mp4" -c copy \
    -metadata creation_time=2024-06-01T08:00:00Z -metadata location=+59.9300+030.3300/ video.mp4
exiftool -q -overwrite_original -Keys:Make=Apple '-Keys:Model=iPhone 15' video.mp4

# An iPhone writes the local time with its offset in CreationDate.
clip -f mov video.mov
exiftool -q -overwrite_original '-Keys:CreationDate=2024:05:01 12:00:00+02:00' video.mov

# Tagged as HLG, as an iPhone records by default, so the poster is tone mapped.
clip -pix_fmt yuv420p10le -vf setparams=color_primaries=bt2020:color_trc=arib-std-b67:colorspace=bt2020nc hdr.mp4

# A motion photo in Google's format: the video follows the JPEG, and XMP
# gives its length.
ffmpeg -hide_banner -loglevel error -y -f lavfi -i testsrc=size=160x120:rate=10 -t 1 \
    -c:v libx264 -pix_fmt yuv420p "$tmp/motion.mp4"
length=$(wc -c < "$tmp/motion.mp4")
cat > "$tmp/motion.xmp" <<EOF
<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
<rdf:Description rdf:about="" xmlns:GCamera="http://ns.google.com/photos/1.0/camera/"
    xmlns:Container="http://ns.google.com/photos/1.0/container/"
    xmlns:Item="http://ns.google.com/photos/1.0/container/item/"
    GCamera:MotionPhoto="1" GCamera:MotionPhotoVersion="1" GCamera:MotionPhotoPresentationTimestampUs="0">
<Container:Directory><rdf:Seq>
<rdf:li rdf:parseType="Resource"><Container:Item Item:Mime="image/jpeg" Item:Semantic="Primary" Item:Length="0" Item:Padding="0"/></rdf:li>
<rdf:li rdf:parseType="Resource"><Container:Item Item:Mime="video/mp4" Item:Semantic="MotionPhoto" Item:Length="$length" Item:Padding="0"/></rdf:li>
</rdf:Seq></Container:Directory>
</rdf:Description></rdf:RDF></x:xmpmeta>
EOF
cp landscape.jpg "$tmp/still.jpg"
exiftool -q -overwrite_original "-xmp<=$tmp/motion.xmp" "$tmp/still.jpg"
cat "$tmp/still.jpg" "$tmp/motion.mp4" > motion.jpg

# A motion photo in Google's older MicroVideo format, as a Pixel 2 or 3 made
# it: the same video follows the JPEG, and XMP gives only its length, which
# exiftool does not extract.
cp landscape.jpg "$tmp/micro.jpg"
exiftool -q -overwrite_original -XMP-GCamera:MicroVideo=1 -XMP-GCamera:MicroVideoVersion=1 \
    "-XMP-GCamera:MicroVideoOffset=$length" -XMP-GCamera:MicroVideoPresentationTimestampUs=0 "$tmp/micro.jpg"
cat "$tmp/micro.jpg" "$tmp/motion.mp4" > microvideo.jpg
