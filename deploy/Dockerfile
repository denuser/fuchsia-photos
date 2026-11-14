# Build context is the repository root.

FROM node:24-slim AS frontend
RUN npm install --global pnpm@12
WORKDIR /src/frontend
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm build

# ffmpeg source and the configure options both builds share: reading MP4
# and MOV, decoding a frame, tone mapping HDR with zscale and writing a PNG.
# Debian's ffmpeg package would add about 360 MB of libraries the
# application never loads. The version follows Debian trixie's.
FROM debian:trixie-slim AS ffmpeg-source
ARG FFMPEG_VERSION=7.1.5
ARG FFMPEG_SHA256=de668509caf9e35e3cd162473441fdb29538c6d96ed080292b3cf9e6fc5d558f
RUN apt-get update \
    && apt-get install --yes --no-install-recommends \
        build-essential ca-certificates curl nasm pkg-config xz-utils \
        libdav1d-dev libzimg-dev zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
RUN curl -fsSLo ffmpeg.tar.xz "https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz" \
    && echo "${FFMPEG_SHA256}  ffmpeg.tar.xz" | sha256sum -c - \
    && tar -xJf ffmpeg.tar.xz --strip-components=1 \
    && rm ffmpeg.tar.xz
ENV FFMPEG_OPTIONS="--prefix=/opt/ffmpeg \
    --disable-everything --disable-autodetect --disable-doc --disable-debug \
    --disable-ffplay --disable-avdevice --disable-network \
    --enable-zlib --enable-libzimg --enable-libdav1d \
    --enable-protocol=file,pipe \
    --enable-demuxer=mov \
    --enable-parser=h264,hevc,vp9,av1,mpeg4video,mjpeg,aac \
    --enable-decoder=h264,hevc,vp9,libdav1d,mpeg4,mjpeg,prores,aac \
    --enable-filter=scale,format,null,transpose,hflip,vflip,zscale,tonemap \
    --enable-encoder=png \
    --enable-muxer=image2"

FROM ffmpeg-source AS ffmpeg
RUN ./configure $FFMPEG_OPTIONS && make -j"$(nproc)" && make install

# The same build for the transcoder on Intel graphics, with what the copies of
# videos need:
# decoding, scaling and H.264 encoding through VAAPI, tone mapping HDR with
# OpenCL on frames shared with VAAPI, AAC sound and the MP4 muxer.
FROM ffmpeg-source AS ffmpeg-intel
RUN apt-get update \
    && apt-get install --yes --no-install-recommends libva-dev ocl-icd-opencl-dev opencl-c-headers \
    && rm -rf /var/lib/apt/lists/*
RUN ./configure $FFMPEG_OPTIONS \
        --enable-vaapi --enable-opencl \
        --enable-hwaccel=h264_vaapi,hevc_vaapi,vp9_vaapi,av1_vaapi \
        --enable-filter=scale_vaapi,tonemap_opencl,hwmap,hwupload,hwdownload,aformat,aresample,anull \
        --enable-decoder=alac,pcm_s16le,pcm_s24le \
        --enable-encoder=h264_vaapi,aac \
        --enable-muxer=mp4 \
    && grep -q '^#define HAVE_VAAPI_DRM 1' config.h \
    && grep -q '^#define HAVE_OPENCL_VAAPI_INTEL_MEDIA 1' config.h \
    && make -j"$(nproc)" && make install

# The same build for the transcoder on AMD graphics, without OpenCL: decoding, scaling and H.264
# encoding through VAAPI, AAC sound and the MP4 muxer. It makes no copies of
# HDR videos yet.
FROM ffmpeg-source AS ffmpeg-amd
RUN apt-get update \
    && apt-get install --yes --no-install-recommends libva-dev \
    && rm -rf /var/lib/apt/lists/*
RUN ./configure $FFMPEG_OPTIONS \
        --enable-vaapi \
        --enable-hwaccel=h264_vaapi,hevc_vaapi,vp9_vaapi,av1_vaapi \
        --enable-filter=scale_vaapi,hwmap,hwupload,hwdownload,aformat,aresample,anull \
        --enable-decoder=alac,pcm_s16le,pcm_s24le \
        --enable-encoder=h264_vaapi,aac \
        --enable-muxer=mp4 \
    && grep -q '^#define HAVE_VAAPI_DRM 1' config.h \
    && make -j"$(nproc)" && make install

FROM golang:1.27-trixie AS backend
RUN apt-get update \
    && apt-get install --yes --no-install-recommends \
        libvips-dev libheif-plugin-libde265 libimage-exiftool-perl libdav1d7 libzimg2 \
    && rm -rf /var/lib/apt/lists/*
COPY --from=ffmpeg /opt/ffmpeg/bin/ffmpeg /opt/ffmpeg/bin/ffprobe /usr/local/bin/
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./

# Runs the tests against libvips from the same Debian release as production.
FROM backend AS test
CMD ["sh", "-c", "go vet ./... && go test ./..."]

# Regenerates the fixtures of internal/media. Making the sample videos needs
# Debian's full ffmpeg, with lavfi and libx264.
FROM backend AS fixtures
RUN rm /usr/local/bin/ffmpeg /usr/local/bin/ffprobe \
    && apt-get update \
    && apt-get install --yes --no-install-recommends libvips-tools libheif-plugin-x265 ffmpeg \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src/backend/internal/media/testdata
CMD ["sh", "generate.sh"]

FROM backend AS build
COPY --from=frontend /src/frontend/dist/ internal/web/dist/
RUN mkdir /out && go build -trimpath -ldflags='-s -w' -o /out/ ./cmd/fuchsia-photos ./cmd/imgbench

# The transcoder needs no cgo, and so neither libvips nor the frontend.
FROM golang:1.27-trixie AS transcoder-build
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/fuchsia-photos-transcoder ./cmd/fuchsia-photos-transcoder

FROM debian:trixie-slim AS runtime
RUN apt-get update \
    && apt-get install --yes --no-install-recommends \
        libvips42t64 libheif-plugin-libde265 libimage-exiftool-perl libdav1d7 libzimg2 \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/ /usr/local/bin/
EXPOSE 8080
CMD ["fuchsia-photos", "serve"]

# The base of the transcoder's images: the service, and ffmpeg's libraries
# from Debian, without libvips. It runs as the application does, so that it
# can write the copies into the cache.
FROM debian:trixie-slim AS transcoder
RUN apt-get update \
    && apt-get install --yes --no-install-recommends libdav1d7 libzimg2 libva2 libva-drm2 vainfo \
    && rm -rf /var/lib/apt/lists/*
COPY --from=transcoder-build /out/fuchsia-photos-transcoder /usr/local/bin/
EXPOSE 8091
CMD ["fuchsia-photos-transcoder"]

# The transcoder for Intel graphics, amd64 only, published as
# fuchsia-photos-transcoder:latest-intel: Debian's non-free iHD driver, whose
# prebuilt GPU kernels encode faster than the free build, and Intel's OpenCL
# runtime, which Debian trixie does not package, from Intel's releases.
FROM transcoder AS transcoder-intel
ARG COMPUTE_RUNTIME=26.40.40196.8
ARG GMMLIB=22.10.2
ARG IGC=2.42.6+23070
RUN sed -i 's/^Components: main$/Components: main non-free/' /etc/apt/sources.list.d/debian.sources \
    && apt-get update \
    && apt-get install --yes --no-install-recommends ca-certificates curl \
    && mkdir /tmp/intel && cd /tmp/intel \
    && cr="https://github.com/intel/compute-runtime/releases/download/${COMPUTE_RUNTIME}" \
    && igc="https://github.com/intel/intel-graphics-compiler/releases/download/v${IGC%+*}" \
    && curl -fsSLO "$cr/intel-opencl-icd_${COMPUTE_RUNTIME}-0_amd64.deb" \
    && curl -fsSLO "$cr/libigdgmm12_${GMMLIB}_amd64.deb" \
    && curl -fsSLO "$igc/intel-igc-core-2_${IGC}_amd64.deb" \
    && curl -fsSLO "$igc/intel-igc-opencl-2_${IGC}_amd64.deb" \
    && printf '%s  %s\n' \
        96046069e94680b761df6e83ddc64244721178b826e3e1f8af3ca104cfd0c7c2 "intel-opencl-icd_${COMPUTE_RUNTIME}-0_amd64.deb" \
        053fceadb84a1d2f9a30a0d1bc9b16424b3f1a90cc49900fcd80aadcf25c8a4d "libigdgmm12_${GMMLIB}_amd64.deb" \
        e20539632f2d99875d8f8b225d5da58b31f98baa54bb47c6328aa98ec473ab9d "intel-igc-core-2_${IGC}_amd64.deb" \
        7f5593cf828503ffb8450967768bde9209e22753bc576296e846b5a751011fa3 "intel-igc-opencl-2_${IGC}_amd64.deb" \
        | sha256sum -c - \
    && apt-get install --yes --no-install-recommends \
        intel-media-va-driver-non-free ocl-icd-libopencl1 ./*.deb \
    && apt-get purge --yes --auto-remove curl \
    && cd / && rm -rf /tmp/intel /var/lib/apt/lists/*
COPY --from=ffmpeg-intel /opt/ffmpeg/bin/ffmpeg /opt/ffmpeg/bin/ffprobe /usr/local/bin/
ENV LIBVA_DRIVER_NAME=iHD
USER 1000:1000

# The transcoder for AMD graphics, amd64 only, published as
# fuchsia-photos-transcoder:latest-amd: Mesa's radeonsi driver. It makes no
# copies of HDR videos yet.
FROM transcoder AS transcoder-amd
RUN apt-get update \
    && apt-get install --yes --no-install-recommends mesa-va-drivers \
    && rm -rf /var/lib/apt/lists/*
COPY --from=ffmpeg-amd /opt/ffmpeg/bin/ffmpeg /opt/ffmpeg/bin/ffprobe /usr/local/bin/
ENV LIBVA_DRIVER_NAME=radeonsi
USER 1000:1000

# The default image, for every platform.
FROM runtime
COPY --from=ffmpeg /opt/ffmpeg/bin/ffmpeg /opt/ffmpeg/bin/ffprobe /usr/local/bin/
USER 1000:1000
