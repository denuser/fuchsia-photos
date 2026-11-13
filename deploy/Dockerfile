# Build context is the repository root.

FROM node:24-slim AS frontend
RUN npm install --global pnpm@12
WORKDIR /src/frontend
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm build

# ffmpeg and ffprobe with only what the application uses: reading MP4 and MOV,
# decoding a frame, tone mapping HDR with zscale and writing a PNG. Debian's
# ffmpeg package would add about 360 MB of libraries the application never
# loads. The version follows Debian trixie's.
FROM debian:trixie-slim AS ffmpeg
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
    && ./configure --prefix=/opt/ffmpeg \
        --disable-everything --disable-autodetect --disable-doc --disable-debug \
        --disable-ffplay --disable-avdevice --disable-network \
        --enable-zlib --enable-libzimg --enable-libdav1d \
        --enable-protocol=file,pipe \
        --enable-demuxer=mov \
        --enable-parser=h264,hevc,vp9,av1,mpeg4video,mjpeg,aac \
        --enable-decoder=h264,hevc,vp9,libdav1d,mpeg4,mjpeg,prores,aac \
        --enable-filter=scale,format,null,transpose,hflip,vflip,zscale,tonemap \
        --enable-encoder=png \
        --enable-muxer=image2 \
    && make -j"$(nproc)" \
    && make install

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
RUN mkdir /out && go build -trimpath -ldflags='-s -w' -o /out/ ./cmd/...

FROM debian:trixie-slim
RUN apt-get update \
    && apt-get install --yes --no-install-recommends \
        libvips42t64 libheif-plugin-libde265 libimage-exiftool-perl libdav1d7 libzimg2 \
    && rm -rf /var/lib/apt/lists/*
COPY --from=ffmpeg /opt/ffmpeg/bin/ffmpeg /opt/ffmpeg/bin/ffprobe /usr/local/bin/
COPY --from=build /out/ /usr/local/bin/
USER 1000:1000
EXPOSE 8080
CMD ["fuchsia-photos", "serve"]
